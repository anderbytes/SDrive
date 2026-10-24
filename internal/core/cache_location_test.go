package core

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func cacheLocationService(t *testing.T) *Service {
	t.Helper()
	s := testService(t, nil)
	s.store.Connections = []Connection{testConnection()}
	s.store.Connections[0].Recovery = false
	SetConnectionEngine(s, func(Connection, string) (RPC, error) { return &fakeRPC{}, nil })
	if err := saveStore(s.dir, s.store); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCacheLocationPreservesOldDataAcrossRestartAndSettingsRestore(t *testing.T) {
	s := cacheLocationService(t)
	content := "preserved clean data"
	oldMetadata := cacheFixture(t, s, testConnection(), `{"Dirty":false}`, &content)
	oldRoot := s.dataPath()
	target := t.TempDir()
	if err := s.ChangeCacheLocation(target); err != nil {
		t.Fatal(err)
	}
	if s.dataPath() != target {
		t.Fatal("cache location not activated")
	}
	if b, err := os.ReadFile(oldMetadata); err != nil || string(b) != `{"Dirty":false}` {
		t.Fatal("previous metadata changed")
	}
	if b, err := os.ReadFile(filepath.Join(oldRoot, "vfs", "sd_"+testConnection().Remote, "nested", "file")); err != nil || string(b) != content {
		t.Fatal("previous data changed")
	}
	reopened, _ := New(s.dir, &fakeRPC{}, "")
	if reopened.dataPath() != target || reopened.settingsError != "" {
		t.Fatalf("cache binding lost: %s", reopened.settingsError)
	}
	if err := os.WriteFile(filepath.Join(s.dir, "settings.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	restored, _ := New(s.dir, &fakeRPC{}, "")
	if err := restored.RestoreSettings(); err != nil {
		t.Fatal(err)
	}
	if restored.dataPath() != target {
		t.Fatal("settings restoration redirected the cache")
	}
	if err := os.Rename(target, target+"-offline"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Rename(target+"-offline", target) })
	offline, _ := New(s.dir, &fakeRPC{}, "")
	if offline.settingsError == "" || offline.dataPath() != target || offline.cacheReady() == nil {
		t.Fatal("unavailable cache silently fell back")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("unavailable root recreated")
	}
}

func TestCacheLocationRejectsPendingUnknownAndActiveState(t *testing.T) {
	for _, state := range []string{"dirty", "corrupt", "unpaired", "recovery", "mounted", "busy", "unknown remote"} {
		t.Run(state, func(t *testing.T) {
			s := cacheLocationService(t)
			c := testConnection()
			old := s.dataPath()
			switch state {
			case "dirty":
				content := "pending"
				cacheFixture(t, s, c, `{"Dirty":true}`, &content)
			case "corrupt":
				cacheFixture(t, s, c, `broken`, nil)
			case "unpaired":
				content := "unknown"
				path := cacheFixture(t, s, c, `{"Dirty":false}`, &content)
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "recovery":
				s.store.Connections[0].Recovery = true
			case "mounted":
				s.mounted[c.ID] = true
			case "busy":
				s.busy[c.ID] = true
			case "unknown remote":
				if err := os.MkdirAll(filepath.Join(old, "vfs", "unrecognized"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.ChangeCacheLocation(t.TempDir()); err == nil || s.dataPath() != old {
				t.Fatal("unsafe cache transition accepted")
			}
		})
	}
}

func TestCacheLocationCommitFailureKeepsOriginalBinding(t *testing.T) {
	s := cacheLocationService(t)
	old := s.dataPath()
	name, _ := windows.UTF16PtrFromString(filepath.Join(s.dir, cacheLocationFile))
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	target := t.TempDir()
	if err := s.ChangeCacheLocation(target); err == nil || s.dataPath() != old {
		t.Fatal("failed commit changed active cache")
	}
	if _, err := os.Stat(filepath.Join(target, cacheOwnerFile)); err != nil {
		t.Fatal("test did not reach location commit")
	}
	location, err := readCacheLocation(s.dir)
	if err != nil || location.Path != old {
		t.Fatal("original disk binding changed")
	}
}

func TestMissingCacheBindingNeverFallsBackToLegacyCache(t *testing.T) {
	s := cacheLocationService(t)
	if err := os.Remove(filepath.Join(s.dir, cacheLocationFile)); err != nil {
		t.Fatal(err)
	}
	reopened, _ := New(s.dir, &fakeRPC{}, "")
	if reopened.settingsError == "" || reopened.SaveSettings(s.store.Settings) == nil || reopened.RestoreSettings() == nil {
		t.Fatal("missing binding authorized fallback or writes")
	}
}
