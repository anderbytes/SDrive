package desktop

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf16"

	"sdrive/internal/platform"
)

// nsExec expects UTF-16 output for Unicode download details.
type installerLog struct{ io.Writer }

func (w installerLog) Write(data []byte) (int, error) {
	units := utf16.Encode([]rune(string(data)))
	encoded := make([]byte, len(units)*2)
	for i, unit := range units {
		binary.LittleEndian.PutUint16(encoded[i*2:], unit)
	}
	n, err := w.Writer.Write(encoded)
	if err != nil {
		return 0, err
	}
	if n != len(encoded) {
		return 0, io.ErrShortWrite
	}
	return len(data), nil
}

func handleCommand(prerequisiteManifest []byte) bool {
	if len(os.Args) == 2 && os.Args[1] == "--restore-startup" {
		if err := restoreStartup(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return true
	}
	if len(os.Args) == 2 && os.Args[1] == "--cleanup-drive-icons" {
		if err := errors.Join(platform.CleanupDriveIcons(), platform.CleanupDriveLabels()); err != nil {
			os.Exit(1)
		}
		return true
	}
	if len(os.Args) == 4 && (os.Args[1] == "--download-prerequisite" || os.Args[1] == "--install-prerequisite") {
		component, err := platform.PrerequisiteFromManifest(prerequisiteManifest, os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(13)
		}
		if os.Args[1] == "--install-prerequisite" {
			os.Exit(platform.InstallPrerequisite(component, filepath.Join(os.Args[3], component.File)))
		}
		log := installerLog{os.Stdout}
		fmt.Fprint(log, "\ufeff")
		if err = platform.DownloadPrerequisite(context.Background(), component, os.Args[3], log); err != nil {
			fmt.Fprintln(log, err)
			os.Exit(1)
		}
		return true
	}
	if len(os.Args) == 2 && os.Args[1] == "--check-prerequisites" {
		os.Exit(platform.PrerequisiteStatus())
	}
	if len(os.Args) == 3 && os.Args[1] == "--config-key" {
		key, err := platform.Key(os.Args[2])
		if err != nil {
			os.Exit(1)
		}
		fmt.Print(key)
		return true
	}
	if len(os.Args) > 1 && os.Args[1] == "--self-check" {
		if err := selfCheck(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return true
	}
	return false
}
