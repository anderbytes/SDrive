package platform

import "testing"

func TestCleanupDriveAppearanceRejectsInvalidExecutable(t *testing.T) {
	for _, path := range []string{"", ".", "SDrive.exe", `relative\SDrive.exe`, `C:\Other\Other.exe`} {
		if CleanupDriveAppearance(path) == nil {
			t.Fatalf("accepted invalid installation executable %q", path)
		}
	}
}
