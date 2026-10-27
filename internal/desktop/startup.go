package desktop

import (
	"os"

	"sdrive/internal/core"
	"sdrive/internal/platform"
)

func restoreStartup() error {
	dir, err := platform.DataDir()
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	unlock, err := platform.SingleInstance(dir)
	if err != nil {
		return err
	}
	defer unlock()
	return core.RestoreStartup(dir)
}
