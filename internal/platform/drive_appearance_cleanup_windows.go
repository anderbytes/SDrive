package platform

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// CleanupDriveAppearance removes unchanged values owned by the named installation,
// even when its application executable has been removed or quarantined.
func CleanupDriveAppearance(executable string) error {
	if !filepath.IsAbs(executable) || !strings.EqualFold(filepath.Base(executable), "SDrive.exe") {
		return fmt.Errorf("invalid installation executable")
	}
	driveIconMu.Lock()
	defer driveIconMu.Unlock()
	driveLabelMu.Lock()
	defer driveLabelMu.Unlock()
	iconErr := reconcileDriveIcons(registry.CURRENT_USER, driveIconRoot, "", executable, nil, true)
	_, labelErr := reconcileDriveLabels(driveLabelRoot, "", executable, nil)
	return errors.Join(iconErr, labelErr)
}
