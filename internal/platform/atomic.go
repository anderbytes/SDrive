package platform

import (
	"os"
	"path/filepath"
)

// AtomicWrite replaces a file only after its complete contents have been synced.
func AtomicWrite(path string, data []byte) error {
	return atomicWrite(path, data, Replace)
}

func atomicWrite(path string, data []byte, promote func(string, string) error) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".sdrive-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return promote(temp, path)
}
