package core

import (
	"context"
	"fmt"
	"testing"
)

func TestExplorerOpensOncePerSuccessfulMountAndNeverFromSnapshots(t *testing.T) {
	for _, tc := range []struct {
		name             string
		preference, fail bool
		opens            int
	}{{"default", false, false, 0}, {"enabled", true, false, 1}, {"failure", true, true, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			mounted := false
			rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
				if method == "config/get" {
					return map[string]string{"type": "ftp", "host": "example.test"}, nil
				}
				if method == "mount/mount" {
					if tc.fail {
						return nil, fmt.Errorf("mount failed")
					}
					mounted = true
				}
				if method == "mount/listmounts" {
					mounts := []engineMount{}
					if mounted {
						mounts = append(mounts, engineMount{Fs: remote(testConnection()), MountPoint: "S:"})
					}
					return map[string]any{"mountPoints": mounts}, nil
				}
				if method == "vfs/stats" {
					return map[string]any{"diskCache": cleanCacheStats()}, nil
				}
				return map[string]any{}, nil
			}}
			s := testService(t, rpc)
			c := testConnection()
			c.Recovery = false
			s.store.Connections = []Connection{c}
			s.store.Settings.OpenExplorerAfterConnect = tc.preference
			opens := 0
			s.open = func(target string) error {
				if target != `S:\` {
					t.Error(target)
				}
				_ = Language(s)
				opens++
				return nil
			}
			err := s.Mount(c.ID)
			if (err != nil) != tc.fail || opens != tc.opens {
				t.Fatal("incorrect mount action", err, opens)
			}
			s.Snapshot()
			s.Snapshot()
			_ = s.Mount(c.ID)
			if opens != tc.opens {
				t.Fatal("reconciliation or repeated mount reopened Explorer")
			}
		})
	}
}

func TestDesktopFailuresDoNotChangeMountedRecovery(t *testing.T) {
	s := testService(t, &fakeRPC{})
	c := testConnection()
	s.store.Connections = []Connection{c}
	s.mounted[c.ID] = true
	s.store.Settings.OpenExplorerAfterConnect = true
	s.store.Settings.ShowServiceIcons = true
	s.open = func(string) error { return fmt.Errorf("desktop failed") }
	SetDriveIcons(s, func(drives []Connection) error {
		_ = Language(s)
		if len(drives) != 1 {
			t.Error("connected letter missing")
		}
		return fmt.Errorf("registry denied")
	})
	s.mountDesktopActions(c.ID)
	if !s.mounted[c.ID] || s.store.Connections[0] != c || s.desktopWarning == "" || s.explorerWarning == "" || s.errors[c.ID] != "" {
		t.Fatal("cosmetic failure affected mounting or recovery")
	}
	SetDriveIcons(s, func(drives []Connection) error {
		if len(drives) != 0 {
			t.Error("disabled icons retained")
		}
		return nil
	})
	s.store.Settings.ShowServiceIcons = false
	s.reconcileDesktopIcons()
	if s.desktopWarning != "" {
		t.Fatal("icon warning not cleared")
	}
}

func TestDesktopPreferencesPersistWithoutOpeningExistingMounts(t *testing.T) {
	s := testService(t, &fakeRPC{})
	SetAutoStart(s, func(bool) error { return nil })
	c := testConnection()
	s.store.Connections = []Connection{c}
	s.mounted[c.ID] = true
	opens := 0
	s.open = func(string) error { opens++; return nil }
	icons := 0
	SetDriveIcons(s, func(drives []Connection) error { icons = len(drives); return nil })
	settings := s.store.Settings
	settings.OpenExplorerAfterConnect = true
	settings.ShowServiceIcons = true
	if err := s.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadStore(s.dir)
	if err != nil || !loaded.Settings.OpenExplorerAfterConnect || !loaded.Settings.ShowServiceIcons || opens != 0 || icons != 1 {
		t.Fatal("desktop preferences were not preserved independently of mount actions", err, opens, icons)
	}
}
