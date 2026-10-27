//go:build windows

package integration

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"sdrive/internal/core"
	"sdrive/internal/platform"
)

func TestWindowsServiceIconsResolveForMountedDriveLetters(t *testing.T) {
	requireNativeMount(t)
	for _, mode := range []string{"network", "local"} {
		t.Run(mode, func(t *testing.T) {
			f := newIntegrationFixture(t, true)
			// Use only a free letter without an existing user or machine customization.
			f.letter = ""
			for letter := 'Z'; letter >= 'D'; letter-- {
				if platform.DriveUsed(string(letter)) {
					continue
				}
				custom := false
				for _, entry := range []struct {
					hive registry.Key
					root string
				}{{registry.CURRENT_USER, `Software\Classes\Applications\Explorer.exe\Drives`}, {registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Explorer\DriveIcons`}, {registry.LOCAL_MACHINE, `Software\Microsoft\Windows\CurrentVersion\Explorer\DriveIcons`}} {
					key, err := registry.OpenKey(entry.hive, entry.root+`\`+string(letter)+`\DefaultIcon`, registry.QUERY_VALUE)
					if err == nil {
						custom = true
						key.Close()
					} else if err != registry.ErrNotExist {
						custom = true
					}
				}
				if !custom {
					f.letter = string(letter)
					break
				}
			}
			if f.letter == "" {
				t.Fatal("no free letter without customization for native icon validation")
			}
			s, _ := openIntegrationService(t, f.profile, nil)
			c := configureIntegration(t, s, f.url, f.letter)
			isolatedConnectionWorkers(t, s, f.profile)
			c.MountMode = mode
			if err := s.UpdateConnection(c); err != nil {
				t.Fatal(err)
			}
			core.SetDriveIcons(s, func(connections []core.Connection) error {
				drives := []platform.DriveIcon{}
				for _, c := range connections {
					drives = append(drives, platform.DriveIcon{Letter: c.Letter, Provider: c.Provider})
				}
				return platform.SyncDriveIcons(f.profile, drives)
			})
			t.Cleanup(func() {
				view := s.Snapshot()
				if len(view.Connections) > 0 && view.Connections[0].Mounted {
					if err := s.Unmount(c.ID); err != nil {
						t.Error(err)
					}
				}
				if err := platform.SyncDriveIcons(f.profile, nil); err != nil {
					t.Error(err)
				}
			})
			settings := s.Snapshot().Settings
			settings.ShowServiceIcons = true
			if err := s.SaveSettings(settings); err != nil {
				t.Fatal(err)
			}
			if err := s.Mount(c.ID); err != nil {
				t.Fatal(err)
			}
			view := s.Snapshot()
			if view.DesktopWarning != "" {
				t.Fatal(view.DesktopWarning)
			}
			expected := loadNativeIcon(t, filepath.Join(f.profile, "icons", "webdav.ico"))
			defer windows.NewLazySystemDLL("user32.dll").NewProc("DestroyIcon").Call(uintptr(expected))
			if !bytes.Equal(nativeIconPixels(t, expected), nativeDriveIconPixels(t, f.letter+`:\`)) {
				t.Fatal("Windows Shell did not resolve the service icon for the mounted drive")
			}
			settings.ShowServiceIcons = false
			if err := s.SaveSettings(settings); err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(nativeIconPixels(t, expected), nativeDriveIconPixels(t, f.letter+`:\`)) {
				t.Fatal("Windows Shell retained the service icon after disabling it")
			}
			if err := s.Unmount(c.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func loadNativeIcon(t *testing.T, path string) windows.Handle {
	t.Helper()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, _, err := windows.NewLazySystemDLL("user32.dll").NewProc("LoadImageW").Call(0, uintptr(unsafe.Pointer(p)), 1, 32, 32, 0x10)
	if h == 0 {
		t.Fatal("LoadImageW", err)
	}
	return windows.Handle(h)
}

func nativeDriveIconPixels(t *testing.T, path string) []byte {
	t.Helper()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		Icon       windows.Handle
		Index      int32
		Attributes uint32
		Display    [260]uint16
		Type       [80]uint16
	}
	result, _, err := windows.NewLazySystemDLL("shell32.dll").NewProc("SHGetFileInfoW").Call(uintptr(unsafe.Pointer(p)), 0, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info), 0x100)
	if result == 0 || info.Icon == 0 {
		t.Fatal("SHGetFileInfoW", err)
	}
	defer windows.NewLazySystemDLL("user32.dll").NewProc("DestroyIcon").Call(uintptr(info.Icon))
	return nativeIconPixels(t, info.Icon)
}

func nativeIconPixels(t *testing.T, icon windows.Handle) []byte {
	t.Helper()
	gdi := windows.NewLazySystemDLL("gdi32.dll")
	dc, _, err := gdi.NewProc("CreateCompatibleDC").Call(0)
	if dc == 0 {
		t.Fatal(err)
	}
	defer gdi.NewProc("DeleteDC").Call(dc)
	var bitmap struct {
		Size                         uint32
		Width, Height                int32
		Planes, BitCount             uint16
		Compression, SizeImage       uint32
		XPelsPerMeter, YPelsPerMeter int32
		ClrUsed, ClrImportant        uint32
		Colors                       uint32
	}
	bitmap.Size = 40
	bitmap.Width = 32
	bitmap.Height = -32
	bitmap.Planes = 1
	bitmap.BitCount = 32
	var pixels unsafe.Pointer
	h, _, err := gdi.NewProc("CreateDIBSection").Call(dc, uintptr(unsafe.Pointer(&bitmap)), 0, uintptr(unsafe.Pointer(&pixels)), 0, 0)
	if h == 0 || pixels == nil {
		t.Fatal("CreateDIBSection", err)
	}
	defer gdi.NewProc("DeleteObject").Call(h)
	previous, _, _ := gdi.NewProc("SelectObject").Call(dc, h)
	defer gdi.NewProc("SelectObject").Call(dc, previous)
	buffer := unsafe.Slice((*byte)(pixels), 32*32*4)
	for i := range buffer {
		buffer[i] = 255
	}
	result, _, err := windows.NewLazySystemDLL("user32.dll").NewProc("DrawIconEx").Call(dc, 0, 0, uintptr(icon), 32, 32, 0, 0, 3)
	if result == 0 {
		t.Fatal(fmt.Errorf("DrawIconEx: %w", err))
	}
	return bytes.Clone(buffer)
}
