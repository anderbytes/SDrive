package platform

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/w32"
	"golang.org/x/sys/windows"
)

func InstanceID(dir string) (string, error) {
	path, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return "", err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("sdrive-%x", sha256.Sum256([]byte(user.User.Sid.String()+"|"+strings.ToLower(filepath.Clean(path))))), nil
}

// Wails beta.34's sender uses FindWindowW, which cannot find its message-only receiver.
func ActivateInstance(id string) error {
	user32 := windows.NewLazySystemDLL("user32.dll")
	class, _ := windows.UTF16PtrFromString("wails-app-" + id + "-sic")
	name, _ := windows.UTF16PtrFromString("wails-app-" + id + "-siw")
	deadline := time.Now().Add(3 * time.Second)
	var hwnd uintptr
	for {
		hwnd, _, _ = user32.NewProc("FindWindowExW").Call(^uintptr(2), 0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(name)))
		if hwnd != 0 {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("SDrive owns this profile but cannot be activated in this Windows session; use its tray icon")
		}
		time.Sleep(25 * time.Millisecond)
	}
	var pid uint32
	user32.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid != 0 {
		user32.NewProc("AllowSetForegroundWindow").Call(uintptr(pid))
	}
	data, _ := windows.UTF16FromString(`{"args":[],"workingDir":""}`)
	message := w32.COPYDATASTRUCT{DwData: w32.WMCOPYDATA_SINGLE_INSTANCE_DATA, CbData: uint32(len(data) * 2), LpData: uintptr(unsafe.Pointer(&data[0]))}
	var result uintptr
	ok, _, _ := user32.NewProc("SendMessageTimeoutW").Call(hwnd, w32.WM_COPYDATA, 0, uintptr(unsafe.Pointer(&message)), 0x2, 2000, uintptr(unsafe.Pointer(&result)))
	if ok == 0 {
		return fmt.Errorf("SDrive did not respond to window activation; use its tray icon")
	}
	return nil
}
