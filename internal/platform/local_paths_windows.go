package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// LocalPath rejects device paths, alternate streams and ambiguous Win32 names.
func LocalPath(path string) (string, error) {
	if !filepath.IsAbs(path) || len(filepath.VolumeName(path)) != 2 || strings.ContainsAny(path, "\x00\r\n*?\"<>|") {
		return "", fmt.Errorf("Choose an absolute path on a local disk.")
	}
	for _, part := range strings.FieldsFunc(path[3:], func(r rune) bool { return r == '/' || r == '\\' }) {
		name := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if part == "." || part == ".." || strings.TrimRight(part, " .") != part || strings.Contains(part, ":") || name == "CON" || name == "PRN" || name == "AUX" || name == "NUL" || (len(name) == 4 && (strings.HasPrefix(name, "COM") || strings.HasPrefix(name, "LPT")) && name[3] >= '1' && name[3] <= '9') {
			return "", fmt.Errorf("Choose an unambiguous local directory name.")
		}
	}
	path = filepath.Clean(path)
	return path, nil
}

// DirectoryIdentity checks the disk and every ancestor for reparse points before use.
func DirectoryIdentity(path string) (string, error) {
	var err error
	path, err = LocalPath(path)
	if err != nil {
		return "", err
	}
	root, err := windows.UTF16PtrFromString(filepath.VolumeName(path) + `\`)
	if err != nil {
		return "", err
	}
	kind := windows.GetDriveType(root)
	if kind != windows.DRIVE_FIXED && kind != windows.DRIVE_REMOVABLE {
		return "", fmt.Errorf("Choose a directory on an available local disk.")
	}
	for current := path; ; current = filepath.Dir(current) {
		p, err := windows.UTF16PtrFromString(current)
		if err != nil {
			return "", err
		}
		attributes, err := windows.GetFileAttributes(p)
		if err != nil || attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || attributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
			return "", fmt.Errorf("The local directory is unavailable or contains a link. Preserve its contents.")
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info); err != nil {
		return "", err
	}
	return fmt.Sprintf("%08x:%08x%08x", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow), nil
}
