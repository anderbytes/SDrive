package platform

import (
	"fmt"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// ComponentVersions reports installed components, not an assumed installer version.
func ComponentVersions() (osVersion, winFsp, webView string) {
	v := windows.RtlGetVersion()
	osVersion = fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\WinFsp`, registry.QUERY_VALUE|registry.WOW64_32KEY); err == nil {
		dir, _, _ := key.GetStringValue("InstallDir")
		key.Close()
		if dir != "" {
			winFsp = fileVersion(filepath.Join(dir, "bin", "winfsp-x64.dll"))
		}
	}
	const path = `Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		for _, view := range []uint32{registry.WOW64_32KEY, registry.WOW64_64KEY} {
			key, err := registry.OpenKey(root, path, registry.QUERY_VALUE|view)
			if err != nil {
				continue
			}
			version, _, _ := key.GetStringValue("pv")
			key.Close()
			if version != "" && version != "0.0.0.0" {
				webView = version
				return
			}
		}
	}
	return
}

func fileVersion(path string) string {
	var zero windows.Handle
	size, err := windows.GetFileVersionInfoSize(path, &zero)
	if err != nil || size == 0 || size > 1<<20 {
		return ""
	}
	buffer := make([]byte, size)
	if windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buffer[0])) != nil {
		return ""
	}
	var info *windows.VS_FIXEDFILEINFO
	length := uint32(unsafe.Sizeof(*info))
	if windows.VerQueryValue(unsafe.Pointer(&buffer[0]), `\`, unsafe.Pointer(&info), &length) != nil || info == nil || length < uint32(unsafe.Sizeof(*info)) {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", info.FileVersionMS>>16, info.FileVersionMS&0xffff, info.FileVersionLS>>16, info.FileVersionLS&0xffff)
}
