package platform

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

type mountFolderRecord struct{ Path, Connection, Identity string }

func mountRecordPath(dir, path string) string {
	return filepath.Join(dir, fmt.Sprintf("mount-folder-%x.json", sha256.Sum256([]byte(path))))
}
func ownedMountFolder(dir, id, path string) error {
	b, err := os.ReadFile(mountRecordPath(dir, path))
	if err != nil {
		return err
	}
	var record mountFolderRecord
	if json.Unmarshal(b, &record) != nil || record.Path != path || record.Connection != id {
		return fmt.Errorf("Mount folder ownership is unknown. Preserve the folder and cache.")
	}
	identity, err := DirectoryIdentity(path)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) || identity != record.Identity {
		return fmt.Errorf("Mount folder identity changed. Preserve the folder and cache.")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	entries, err := f.Readdirnames(1)
	if len(entries) != 0 {
		return fmt.Errorf("Mount folder is not empty. Preserve its contents.")
	}
	if err != nil && err != io.EOF {
		return err
	}
	return nil
}
func CheckMountFolder(dir, id, path string) error {
	if _, err := DirectoryIdentity(filepath.Dir(path)); err != nil {
		return err
	}
	_, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return ownedMountFolder(dir, id, path)
}
func PrepareMountFolder(dir, id, path string) error {
	if err := CheckMountFolder(dir, id, path); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return nil
	}
	if err := os.Mkdir(path, 0700); err != nil {
		return err
	}
	identity, err := DirectoryIdentity(path)
	if err != nil {
		return err
	}
	data, err := json.Marshal(mountFolderRecord{Path: path, Connection: id, Identity: identity})
	if err != nil {
		return err
	}
	return AtomicWrite(mountRecordPath(dir, path), data)
}
func ReleaseMountFolder(dir, id, path string) error {
	if err := ownedMountFolder(dir, id, path); err != nil {
		return err
	}
	// Remove is deliberately non-recursive and refuses folders containing user files.
	if err := os.Remove(path); err != nil {
		return err
	}
	return os.Remove(mountRecordPath(dir, path))
}
