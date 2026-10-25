package platform

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const driveLabelRoot = `Software\Microsoft\Windows\CurrentVersion\Explorer\MountPoints2`

var driveLabelMu sync.Mutex
var networkConnection = windows.NewLazySystemDLL("mpr.dll").NewProc("WNetGetConnectionW")

type DriveLabel struct{ Letter, Name string }

func SyncDriveLabels(profile string, drives []DriveLabel) error {
	driveLabelMu.Lock()
	defer driveLabelMu.Unlock()
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	wanted := map[string]string{}
	for _, drive := range drives {
		if len(drive.Letter) != 1 || drive.Letter[0] < 'D' || drive.Letter[0] > 'Z' || drive.Name == "" || strings.ContainsRune(drive.Name, 0) {
			return fmt.Errorf("invalid drive label")
		}
		letter, _ := windows.UTF16PtrFromString(drive.Letter + ":")
		buffer := make([]uint16, 32768)
		size := uint32(len(buffer))
		result, _, _ := networkConnection.Call(uintptr(unsafe.Pointer(letter)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)))
		if result != 0 {
			return fmt.Errorf("resolve network drive label: %w", syscall.Errno(result))
		}
		// Use the actual UNC path, including rclone's volume-name normalization.
		remote := windows.UTF16ToString(buffer)
		if !strings.HasPrefix(remote, `\\`) {
			return fmt.Errorf("invalid network drive path")
		}
		wanted[strings.ReplaceAll(remote, `\`, "#")] = drive.Name
	}
	changed, err := reconcileDriveLabels(driveLabelRoot, profile, executable, wanted)
	if changed {
		for _, drive := range drives {
			notifyDriveChange(drive.Letter)
		}
	}
	return err
}

func CleanupDriveLabels() error {
	driveLabelMu.Lock()
	defer driveLabelMu.Unlock()
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	_, err = reconcileDriveLabels(driveLabelRoot, "", executable, nil)
	return err
}

func reconcileDriveLabels(root, profile, executable string, wanted map[string]string) (changed bool, err error) {
	parent, _, err := registry.CreateKey(registry.CURRENT_USER, root, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return false, err
	}
	names, err := parent.ReadSubKeyNames(-1)
	parent.Close()
	if err != nil {
		return false, err
	}
	desired := map[string]string{}
	for name, label := range wanted {
		desired[strings.ToLower(name)] = label
	}
	paths := map[string]bool{}
	for _, name := range names {
		key, openErr := registry.OpenKey(registry.CURRENT_USER, root+`\`+name, registry.QUERY_VALUE)
		if openErr != nil {
			return false, openErr
		}
		install, _, _ := key.GetStringValue("SDriveLabelInstall")
		key.Close()
		if strings.EqualFold(install, executable) {
			paths[strings.ToLower(name)] = true
		}
	}
	for name := range desired {
		paths[name] = true
	}
	for name := range paths {
		key, _, openErr := registry.CreateKey(registry.CURRENT_USER, root+`\`+name, registry.QUERY_VALUE|registry.SET_VALUE)
		if openErr != nil {
			return changed, openErr
		}
		owner, _, _ := key.GetStringValue("SDriveLabelOwner")
		install, _, _ := key.GetStringValue("SDriveLabelInstall")
		prior, _, _ := key.GetStringValue("SDriveLabel")
		current, _, readErr := key.GetStringValue("_LabelFromReg")
		owned := strings.EqualFold(install, executable) && (profile == "" || strings.EqualFold(owner, profile))
		expected := desired[name]
		if !owned && (owner != "" || install != "" || readErr != registry.ErrNotExist) {
			key.Close()
			continue
		}
		if owned && expected == prior && readErr == nil && current == prior {
			key.Close()
			continue
		}
		if owned {
			if readErr == nil && current == prior {
				err = key.DeleteValue("_LabelFromReg")
				changed = true
			}
			for _, value := range []string{"SDriveLabelOwner", "SDriveLabelInstall", "SDriveLabel"} {
				if err == nil {
					err = key.DeleteValue(value)
					if err == registry.ErrNotExist {
						err = nil
					}
				}
			}
		}
		_, _, readErr = key.GetStringValue("_LabelFromReg")
		if err == nil && expected != "" && readErr == registry.ErrNotExist {
			for _, value := range []struct{ name, value string }{{"SDriveLabelOwner", profile}, {"SDriveLabelInstall", executable}, {"SDriveLabel", expected}, {"_LabelFromReg", expected}} {
				if err = key.SetStringValue(value.name, value.value); err != nil {
					break
				}
			}
			changed = true
		}
		key.Close()
		if err != nil {
			return changed, err
		}
	}
	return changed, nil
}
