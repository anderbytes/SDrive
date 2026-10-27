//go:build windows

package platform

import (
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func ProtectData(_ string, data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty protected data")
	}
	in, out := blob(data), windows.DataBlob{}
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

func UnprotectData(_ string, data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty protected data")
	}
	in, out := blob(data), windows.DataBlob{}
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

var getLastInputInfo = windows.NewLazySystemDLL("user32.dll").NewProc("GetLastInputInfo")
var getTickCount = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetTickCount")

func InputIdle() (time.Duration, error) {
	info := struct{ Size, Tick uint32 }{Size: 8}
	ok, _, err := getLastInputInfo.Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return 0, err
	}
	now, _, _ := getTickCount.Call()
	// Both counters are DWORDs; subtraction preserves the Windows wraparound.
	return time.Duration(uint32(now)-info.Tick) * time.Millisecond, nil
}
