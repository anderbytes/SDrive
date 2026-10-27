package platform

import (
	"os"
	"path/filepath"
)

const OS = "windows"
const AccessFile = "application-lock.dpapi"
const GlobalIdleSupported = true
const DefaultIdleMinutes = 15
const MountType = "cmount"
const CredentialStore = "DPAPI"

func MountAvailable() bool      { return WinFspInstalled() }
func CheckKey(dir string) error { _, err := os.Stat(filepath.Join(dir, "key.dpapi")); return err }
func CheckMountFolder(_ string, _ string, path string) error {
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		return os.ErrExist
	}
	return nil
}
func PrepareMountFolder(string, string, string) error    { return nil }
func ReleaseMountFolder(string, string, string) error    { return nil }
func PortableValidationTarget() (string, string, string) { return "S", "", "network" }
func TrayAvailable() bool                                { return true }

func CleanupProbeKey(string) error { return nil }

func FolderMounted(path string) bool { _, err := os.Lstat(path); return !os.IsNotExist(err) }
