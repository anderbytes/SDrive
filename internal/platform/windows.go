//go:build windows

package platform

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func DataDir() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "SDrive"), nil
}
func Alert(message string) {
	text, _ := windows.UTF16PtrFromString(message)
	title, _ := windows.UTF16PtrFromString("SDrive")
	_, _, _ = windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}
func Replace(src, dst string) error {
	s, e := windows.UTF16PtrFromString(src)
	if e != nil {
		return e
	}
	d, e := windows.UTF16PtrFromString(dst)
	if e != nil {
		return e
	}
	return windows.MoveFileEx(s, d, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
func blob(b []byte) windows.DataBlob { return windows.DataBlob{Size: uint32(len(b)), Data: &b[0]} }
func Key(dir string) (string, error) {
	path := filepath.Join(dir, "key.dpapi")
	encrypted, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		config, readErr := os.ReadFile(filepath.Join(dir, "rclone.conf"))
		if readErr != nil && !os.IsNotExist(readErr) {
			return "", fmt.Errorf("could not verify encrypted configuration; preserve the data directory")
		}
		if len(config) > 0 {
			return "", fmt.Errorf("DPAPI key missing; preserve configuration and cache for recovery")
		}
		key := make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return "", err
		}
		text := []byte(base64.RawStdEncoding.EncodeToString(key))
		in := blob(text)
		var out windows.DataBlob
		if err = windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
			return "", err
		}
		defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
		if err = atomicWrite(path, unsafe.Slice(out.Data, out.Size), createFile); err != nil {
			if errors.Is(err, windows.ERROR_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_FILE_EXISTS) {
				return Key(dir)
			}
			return "", err
		}
		return string(text), nil
	}
	if err != nil {
		return "", err
	}
	if len(encrypted) == 0 {
		return "", fmt.Errorf("empty DPAPI key; do not replace the key")
	}
	in := blob(encrypted)
	var out windows.DataBlob
	if err = windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return "", fmt.Errorf("could not unlock credentials with this Windows user")
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	text := string(unsafe.Slice(out.Data, out.Size))
	decoded, err := base64.RawStdEncoding.DecodeString(text)
	if err != nil || len(decoded) != 32 {
		return "", fmt.Errorf("invalid DPAPI key; do not replace the key")
	}
	return text, nil
}
func DriveUsed(letter string) bool {
	mask, err := windows.GetLogicalDrives()
	return err != nil || mask&(1<<uint(letter[0]-'A')) != 0
}
func FreeBytes(path string) (uint64, error) {
	p, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return 0, e
	}
	var free uint64
	e = windows.GetDiskFreeSpaceEx(p, &free, nil, nil)
	return free, e
}
func AutoStart(enabled bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !enabled {
		err = k.DeleteValue("SDrive")
		if err == syscall.ERROR_FILE_NOT_FOUND {
			return nil
		}
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return k.SetStringValue("SDrive", `"`+exe+`" --background`)
}

func AutoStartMatches(enabled bool) (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE)
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return !enabled, nil
	}
	if err != nil {
		return false, err
	}
	defer k.Close()
	value, _, err := k.GetStringValue("SDrive")
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return !enabled, nil
	}
	if err != nil {
		return false, err
	}
	if !enabled {
		return false, nil
	}
	exe, err := os.Executable()
	return value == `"`+exe+`" --background`, err
}
func SingleInstance() (func(), error) {
	name, _ := windows.UTF16PtrFromString(`Local\SDrive-` + os.Getenv("USERNAME"))
	h, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		if h != 0 {
			windows.CloseHandle(h)
		}
		return nil, fmt.Errorf("SDrive is already running; look for the tray icon")
	}
	return func() { windows.CloseHandle(h) }, nil
}
func ProtectDir(path string) error {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;FA;;;SY)")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}
func WinFspInstalled() bool {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\WinFsp`, registry.QUERY_VALUE|registry.WOW64_32KEY)
	if err != nil {
		return false
	}
	defer key.Close()
	dir, _, err := key.GetStringValue("InstallDir")
	if err != nil {
		return false
	}
	return compatibleWinFsp(filepath.Join(dir, "bin", "winfsp-x64.dll"))
}
