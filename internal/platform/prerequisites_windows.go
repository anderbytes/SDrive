//go:build windows

package platform

import (
	"debug/pe"
	"os/exec"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func compatibleWinFsp(path string) bool {
	file, err := pe.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	if file.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
		return false
	}
	var zero windows.Handle
	size, err := windows.GetFileVersionInfoSize(path, &zero)
	if err != nil || size == 0 || size > 1<<20 {
		return false
	}
	buffer := make([]byte, size)
	if windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buffer[0])) != nil {
		return false
	}
	var info *windows.VS_FIXEDFILEINFO
	length := uint32(unsafe.Sizeof(*info))
	if windows.VerQueryValue(unsafe.Pointer(&buffer[0]), `\`, unsafe.Pointer(&info), &length) != nil || info == nil {
		return false
	}
	return info.FileVersionMS>>16 == 2 && info.FileVersionMS&0xffff >= 1
}

func WebViewInstalled() bool {
	const path = `Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		for _, view := range []uint32{registry.WOW64_32KEY, registry.WOW64_64KEY} {
			key, err := registry.OpenKey(root, path, registry.QUERY_VALUE|view)
			if err != nil {
				continue
			}
			version, _, err := key.GetStringValue("pv")
			key.Close()
			if err == nil && version != "" && version != "0.0.0.0" {
				return true
			}
		}
	}
	return false
}

// Exit flags are consumed by the non-elevated installer before creating any user data.
func PrerequisiteStatus() int {
	status := 0
	if !WinFspInstalled() {
		status += 10
	}
	if !WebViewInstalled() {
		status += 20
	}
	return status
}

type shellExecuteInfo struct {
	Size, Mask                        uint32
	Window                            uintptr
	Verb, File, Parameters, Directory *uint16
	Show                              int32
	Instance, IDList                  uintptr
	Class                             *uint16
	ClassKey                          uintptr
	HotKey                            uint32
	Icon                              uintptr
	Process                           windows.Handle
}

func InstallPrerequisite(p Prerequisite, path string) int {
	file, err := verifiedPrerequisite(p, path)
	if err != nil {
		return 13
	}
	defer file.Close()
	if p.File == "webview2.exe" {
		command := exec.Command(path, "/silent", "/install")
		command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if err = command.Run(); err != nil {
			if exit, ok := err.(*exec.ExitError); ok {
				return exit.ExitCode()
			}
			return 1603
		}
		return 0
	}
	if p.File != "winfsp.msi" {
		return 13
	}
	systemDir, err := windows.GetSystemDirectory()
	if err != nil {
		return 2
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	exe, _ := windows.UTF16PtrFromString(systemDir + `\msiexec.exe`)
	params, err := windows.UTF16PtrFromString("/i " + syscall.EscapeArg(path) + " /qn /norestart")
	if err != nil {
		return 13
	}
	info := shellExecuteInfo{Mask: 0x40 | 0x100 | 0x400, Verb: verb, File: exe, Parameters: params, Show: 0}
	info.Size = uint32(unsafe.Sizeof(info))
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ole := windows.NewLazySystemDLL("ole32.dll")
	hr, _, _ := ole.NewProc("CoInitializeEx").Call(0, 2)
	if hr == 0 || hr == 1 {
		defer ole.NewProc("CoUninitialize").Call()
	}
	result, _, callErr := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW").Call(uintptr(unsafe.Pointer(&info)))
	if result == 0 {
		if callErr == windows.ERROR_CANCELLED {
			return 1223
		}
		return 1603
	}
	if info.Process == 0 {
		return 1603
	}
	defer windows.CloseHandle(info.Process)
	if _, err = windows.WaitForSingleObject(info.Process, windows.INFINITE); err != nil {
		return 1603
	}
	var code uint32
	if windows.GetExitCodeProcess(info.Process, &code) != nil {
		return 1603
	}
	return int(code)
}
