package integration

import (
	"os"
	"path/filepath"
	"sdrive/internal/core"
	"sdrive/internal/desktop"
	"sdrive/internal/platform"
	"testing"
)

func isolatedConnectionWorkers(t *testing.T, s *core.Service, profile string) {
	t.Helper()
	core.SetConnectionEngine(s, func(c core.Connection, proxy string) (core.RPC, error) {
		return desktop.StartConnectionEngine(profile, core.CachePath(s), c, proxy, nil)
	})
	t.Cleanup(func() { core.CloseConnectionEngines(s) })
}

func requireNativeMount(t *testing.T) {
	t.Helper()
	if os.Getenv("SDRIVE_TEST_MOUNT") != "1" {
		t.Skip("opt-in: SDRIVE_TEST_MOUNT=1")
	}
	if !platform.MountAvailable() {
		t.Fatal("native mount requested, but compatible WinFsp is unavailable")
	}
	available := platform.OS == "linux"
	for letter := 'D'; letter <= 'Z'; letter++ {
		if !platform.DriveUsed(string(letter)) {
			available = true
			break
		}
	}
	if !available {
		t.Fatal("native mount requested, but no disposable drive letter is available")
	}
	workspace, err := filepath.Abs(".tools")
	if err != nil {
		t.Fatal(err)
	}
	if free, err := platform.FreeBytes(workspace); err != nil || free < 6<<30 {
		t.Fatal("native mount requested, but the fixture needs 6 GiB free")
	}
}
