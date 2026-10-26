package platform

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const driveIconRoot = `Software\Classes\Applications\Explorer.exe\Drives`

var driveIconMu sync.Mutex
var shellNotify = windows.NewLazySystemDLL("shell32.dll").NewProc("SHChangeNotify")

type DriveIcon struct{ Letter, Provider string }

var serviceBadges = map[string]struct {
	label string
	tint  color.RGBA
}{
	"drive": {"GD", color.RGBA{38, 132, 70, 255}}, "onedrive": {"OD", color.RGBA{0, 105, 185, 255}},
	"sharepoint": {"SP", color.RGBA{0, 115, 120, 255}}, "dropbox": {"DB", color.RGBA{0, 97, 255, 255}},
	"box": {"BX", color.RGBA{0, 97, 170, 255}}, "pcloud": {"PC", color.RGBA{0, 125, 135, 255}},
	"yandex": {"YD", color.RGBA{180, 65, 25, 255}},
	"mega":   {"ME", color.RGBA{190, 35, 40, 255}}, "protondrive": {"PD", color.RGBA{105, 65, 185, 255}},
	"nextcloud": {"NC", color.RGBA{0, 110, 190, 255}}, "owncloud": {"OC", color.RGBA{30, 60, 100, 255}},
	"webdav": {"WD", color.RGBA{70, 100, 135, 255}}, "sftp": {"SF", color.RGBA{65, 100, 70, 255}},
	"ftp": {"FT", color.RGBA{120, 75, 20, 255}}, "smb": {"SM", color.RGBA{75, 80, 135, 255}},
	"s3": {"S3", color.RGBA{155, 75, 15, 255}}, "b2": {"B2", color.RGBA{180, 45, 45, 255}},
	"azureblob": {"AZ", color.RGBA{0, 105, 170, 255}}, "gcs": {"GC", color.RGBA{35, 105, 155, 255}},
}

// Original service monograms avoid downloading or redistributing third-party logos.
var badgeGlyphs = map[rune][7]byte{
	'A': {14, 17, 17, 31, 17, 17, 17}, 'B': {30, 17, 17, 30, 17, 17, 30}, 'C': {14, 17, 16, 16, 16, 17, 14},
	'D': {30, 17, 17, 17, 17, 17, 30}, 'E': {31, 16, 16, 30, 16, 16, 31}, 'F': {31, 16, 16, 30, 16, 16, 16},
	'G': {14, 17, 16, 23, 17, 17, 14}, 'M': {17, 27, 21, 21, 17, 17, 17}, 'N': {17, 25, 25, 21, 19, 19, 17},
	'O': {14, 17, 17, 17, 17, 17, 14}, 'P': {30, 17, 17, 30, 16, 16, 16}, 'S': {15, 16, 16, 14, 1, 1, 30},
	'T': {31, 4, 4, 4, 4, 4, 4}, 'W': {17, 17, 17, 21, 21, 27, 17}, 'X': {17, 17, 10, 4, 10, 17, 17},
	'Y': {17, 17, 10, 4, 4, 4, 4}, 'Z': {31, 1, 2, 4, 8, 16, 31}, '2': {14, 17, 1, 2, 4, 8, 31}, '3': {30, 1, 1, 14, 1, 1, 30},
}

func serviceIcon(provider string) ([]byte, error) {
	badge, ok := serviceBadges[provider]
	if !ok {
		return nil, fmt.Errorf("unknown service icon")
	}
	im := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 2; y < 30; y++ {
		for x := 2; x < 30; x++ {
			im.Set(x, y, badge.tint)
		}
	}
	for i, r := range badge.label {
		for y, row := range badgeGlyphs[r] {
			for x := 0; x < 5; x++ {
				if row&(1<<uint(4-x)) != 0 {
					for dy := 0; dy < 2; dy++ {
						for dx := 0; dx < 2; dx++ {
							im.Set(5+i*12+x*2+dx, 9+y*2+dy, color.White)
						}
					}
				}
			}
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, im); err != nil {
		return nil, err
	}
	var icon bytes.Buffer
	for _, v := range []uint16{0, 1, 1} {
		_ = binary.Write(&icon, binary.LittleEndian, v)
	}
	icon.Write([]byte{32, 32, 0, 0})
	for _, v := range []uint16{1, 32} {
		_ = binary.Write(&icon, binary.LittleEndian, v)
	}
	_ = binary.Write(&icon, binary.LittleEndian, uint32(encoded.Len()))
	_ = binary.Write(&icon, binary.LittleEndian, uint32(22))
	icon.Write(encoded.Bytes())
	return icon.Bytes(), nil
}

func SyncDriveIcons(profile string, drives []DriveIcon) error {
	driveIconMu.Lock()
	defer driveIconMu.Unlock()
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	wanted := map[string]string{}
	for _, drive := range drives {
		if len(drive.Letter) != 1 || drive.Letter[0] < 'D' || drive.Letter[0] > 'Z' {
			return fmt.Errorf("invalid icon drive letter")
		}
		data, err := serviceIcon(drive.Provider)
		if err != nil {
			return err
		}
		path := filepath.Join(profile, "icons", drive.Provider+".ico")
		if _, err := DirectoryIdentity(profile); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		if _, err := DirectoryIdentity(filepath.Dir(path)); err != nil {
			return err
		}
		current, _ := os.ReadFile(path)
		if !bytes.Equal(current, data) {
			if err := AtomicWrite(path, data); err != nil {
				return err
			}
		}
		wanted[drive.Letter] = path
	}
	return reconcileDriveIcons(registry.CURRENT_USER, driveIconRoot, profile, executable, wanted, false)
}

// Cleanup is deliberately limited to this executable's owned, unchanged values.
func CleanupDriveIcons() error {
	driveIconMu.Lock()
	defer driveIconMu.Unlock()
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return reconcileDriveIcons(registry.CURRENT_USER, driveIconRoot, "", executable, nil, true)
}

func reconcileDriveIcons(hive registry.Key, root, profile, executable string, wanted map[string]string, uninstall bool) error {
	for letter := 'D'; letter <= 'Z'; letter++ {
		path := root + `\` + string(letter) + `\DefaultIcon`
		key, err := registry.OpenKey(hive, path, registry.QUERY_VALUE|registry.SET_VALUE)
		if err != nil && err != registry.ErrNotExist {
			return err
		}
		expected := wanted[string(letter)]
		if err == nil {
			owner, _, _ := key.GetStringValue("SDriveOwner")
			install, _, _ := key.GetStringValue("SDriveInstall")
			prior, _, _ := key.GetStringValue("SDriveIcon")
			current, _, readErr := key.GetStringValue("")
			owned := strings.EqualFold(install, executable) && (uninstall || strings.EqualFold(owner, profile))
			if !owned && (owner != "" || install != "") {
				key.Close()
				continue
			}
			if owned && expected == prior && readErr == nil && current == prior {
				key.Close()
				continue
			}
			if owned {
				if readErr == nil && current == prior {
					if err := key.DeleteValue(""); err != nil {
						key.Close()
						return err
					}
					notifyDriveChange(string(letter))
				}
				for _, name := range []string{"SDriveOwner", "SDriveInstall", "SDriveIcon"} {
					if err := key.DeleteValue(name); err != nil && err != registry.ErrNotExist {
						key.Close()
						return err
					}
				}
			}
			_, _, readErr = key.GetStringValue("")
			info, statErr := key.Stat()
			key.Close()
			if owned && statErr == nil && info.ValueCount == 0 && info.SubKeyCount == 0 {
				if err := registry.DeleteKey(hive, path); err != nil && err != registry.ErrNotExist {
					return err
				}
				parent := root + `\` + string(letter)
				if parentKey, err := registry.OpenKey(hive, parent, registry.QUERY_VALUE); err == nil {
					parentInfo, err := parentKey.Stat()
					parentKey.Close()
					if err == nil && parentInfo.ValueCount == 0 && parentInfo.SubKeyCount == 0 {
						if err := registry.DeleteKey(hive, parent); err != nil && err != registry.ErrNotExist {
							return err
						}
					}
				}
			}
			if readErr != registry.ErrNotExist {
				continue
			}
		}
		if expected == "" {
			continue
		}
		key, _, err = registry.CreateKey(hive, path, registry.QUERY_VALUE|registry.SET_VALUE)
		if err != nil {
			return err
		}
		// Recheck before writing: an unrelated existing customization always wins.
		if _, _, err = key.GetStringValue(""); err != registry.ErrNotExist {
			key.Close()
			continue
		}
		for _, value := range []struct{ name, value string }{{"SDriveOwner", profile}, {"SDriveInstall", executable}, {"SDriveIcon", expected}, {"", expected}} {
			if err = key.SetStringValue(value.name, value.value); err != nil {
				key.Close()
				return err
			}
		}
		key.Close()
		notifyDriveChange(string(letter))
	}
	return nil
}

func notifyDriveChange(letter string) {
	path, _ := windows.UTF16PtrFromString(letter + `:\`)
	// SHCNE_UPDATEITEM, SHCNF_PATHW: refresh this drive without restarting Explorer.
	shellNotify.Call(0x2000, 0x5, uintptr(unsafe.Pointer(path)), 0)
}
