package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func writeFixture(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func readFixture(t *testing.T, dir, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestIncompleteSettingsNeverAuthorizeCleanupOrWrites(t *testing.T) {
	for _, data := range []string{
		`null`, `{}`, `[]`, `broken`,
		`{"connections":null,"settings":{"autoStart":false,"cacheGiB":2}}`,
		`{"connections":[],"settings":null}`,
		`{"connections":[],"settings":{}}`,
		`{"settings":{"autoStart":false,"cacheGiB":2}}`,
		`{"connections":[],"settings":{"autoStart":null,"cacheGiB":2}}`,
		`{"connections":[],"settings":{"autoStart":false,"cacheGiB":null}}`,
		`{"connections":[],"settings":{"autoStart":false}}`,
	} {
		t.Run(data, func(t *testing.T) {
			dir := t.TempDir()
			writeFixture(t, dir, "settings.json", []byte(data))
			rpc := &fakeRPC{}
			s, err := New(dir, rpc, "")
			if err != nil || s.Snapshot().SettingsError == "" || len(rpc.calls) != 0 {
				t.Fatalf("invalid settings did not enter protected recovery: %v, %+v", err, s)
			}
			started, registry := false, false
			s.restart = func() (RPC, error) { started = true; return rpc, nil }
			s.autoStart = func(bool) error { registry = true; return nil }
			StartEngine(s)
			AutoMount(s)
			if _, err = s.BeginConfig(ConfigInput{}); err == nil {
				t.Fatal("configuration allowed without valid settings")
			}
			if s.SaveSettings(Settings{CacheGiB: 2}) == nil || s.Remove("id") == nil || s.UpdateConnection(testConnection()) == nil || s.Mount("id") == nil || s.RetryCleanup() == nil {
				t.Fatal("mutation allowed without valid settings")
			}
			if started || registry || len(rpc.calls) != 0 || string(readFixture(t, dir, "settings.json")) != data {
				t.Fatal("invalid settings changed existing state")
			}
		})
	}
}

func TestSettingsVersionsAndHistoricalCompatibility(t *testing.T) {
	legacy := `{"connections":[],"settings":{"autoStart":true,"cacheGiB":3}}`
	for _, prefix := range []string{"", `"version":1,`, `"version":2,`} {
		store, err := decodeStore([]byte("{" + prefix + legacy[1:]))
		if err != nil || store.Settings.Language != "en" || store.Settings.CacheGiB != 3 || !store.Settings.AutoStart {
			t.Fatalf("supported settings rejected: %+v, %v", store, err)
		}
	}
	for _, version := range []string{"null", "0", "3", "-1", `"1"`, "1.5"} {
		dir := t.TempDir()
		writeFixture(t, dir, "settings.json", []byte(`{"version":`+version+","+legacy[1:]))
		writeFixture(t, dir, "settings.backup.json", []byte(legacy))
		s, _ := New(dir, &fakeRPC{}, "")
		if !s.incompatibleSettings || s.Snapshot().BackupAvailable || s.RestoreSettings() == nil {
			t.Fatalf("incompatible version %s allowed restoration", version)
		}
	}
}

func TestBackupPreservesPreviousSettingsAndFailedWrites(t *testing.T) {
	dir := t.TempDir()
	old := defaultStore()
	if err := saveStore(dir, old); err != nil {
		t.Fatal(err)
	}
	first := readFixture(t, dir, "settings.json")
	var shape map[string]any
	if json.Unmarshal(first, &shape) != nil || shape["version"] != float64(2) {
		t.Fatal("new settings are not versioned")
	}
	next := old
	next.Settings.CacheGiB = 8
	if err := saveStore(dir, next); err != nil {
		t.Fatal(err)
	}
	if string(readFixture(t, dir, "settings.backup.json")) != string(first) {
		t.Fatal("previous valid settings not retained")
	}
	current := readFixture(t, dir, "settings.json")
	path, _ := windows.UTF16PtrFromString(filepath.Join(dir, "settings.json"))
	h, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = saveStore(dir, old); err == nil {
		windows.CloseHandle(h)
		t.Fatal("replacement of locked settings succeeded")
	}
	windows.CloseHandle(h)
	if string(readFixture(t, dir, "settings.json")) != string(current) {
		t.Fatal("failed write changed settings")
	}
	partials, _ := filepath.Glob(filepath.Join(dir, ".sdrive-*"))
	if len(partials) != 0 {
		t.Fatalf("temporary files leaked: %v", partials)
	}
	if err = os.Remove(filepath.Join(dir, "settings.backup.json")); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(dir, "settings.backup.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if saveStore(dir, old) == nil || string(readFixture(t, dir, "settings.json")) != string(current) {
		t.Fatal("failed backup changed current settings")
	}
}

func TestCleanupFailureIsRecoverableAndRetryClearsWarning(t *testing.T) {
	for _, failure := range []string{"config/listremotes", "config/delete"} {
		t.Run(failure, func(t *testing.T) {
			dir := t.TempDir()
			if err := saveStore(dir, defaultStore()); err != nil {
				t.Fatal(err)
			}
			fail := true
			rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
				if fail && method == failure {
					return nil, errors.New("private provider detail")
				}
				return map[string]any{"remotes": []string{"sd_" + strings.Repeat("a", 32)}}, nil
			}}
			s, err := New(dir, rpc, "")
			if err != nil || s.maintenanceError == "" || s.Snapshot().EngineError != "" || strings.Contains(s.maintenanceError, "private") {
				t.Fatalf("maintenance failure prevented use or exposed details: %v", err)
			}
			fail = false
			if err = s.RetryCleanup(); err != nil || s.maintenanceError != "" {
				t.Fatalf("retry failed: %v", err)
			}
		})
	}
}

func TestCleanupProtectsBackupAndCacheReferences(t *testing.T) {
	dir := t.TempDir()
	c := testConnection()
	c.Recovery = false
	if err := saveStore(dir, Store{Settings: defaultStore().Settings, Connections: []Connection{c}}); err != nil {
		t.Fatal(err)
	}
	if err := saveStore(dir, defaultStore()); err != nil {
		t.Fatal(err)
	}
	cached := "sd_" + strings.Repeat("a", 32)
	if err := os.MkdirAll(filepath.Join(dir, "cache", "vfs", cached), 0700); err != nil {
		t.Fatal(err)
	}
	orphan := "sd_" + strings.Repeat("b", 32)
	var deleted []string
	rpc := &fakeRPC{handle: func(_ context.Context, method string, in any) (any, error) {
		if method == "config/delete" {
			deleted = append(deleted, in.(map[string]string)["name"])
		}
		return map[string]any{"remotes": []string{"sd_" + c.Remote, cached, orphan, "unrelated", "sd_invalid"}}, nil
	}}
	s, _ := New(dir, rpc, "")
	if !reflect.DeepEqual(deleted, []string{orphan}) {
		t.Fatalf("unsafe cleanup: %v", deleted)
	}
	writeFixture(t, dir, "settings.backup.json", []byte(`null`))
	deleted = nil
	if s.RetryCleanup() == nil || len(deleted) != 0 {
		t.Fatal("unreadable backup authorized cleanup")
	}
	writeFixture(t, dir, "settings.json", []byte(`{}`))
	if s.RetryCleanup() == nil || len(deleted) != 0 {
		t.Fatal("externally damaged settings authorized cleanup")
	}
}

func TestRestorePreservesNewerCredentialsCacheAndDamagedSettingsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	c := testConnection()
	c.Recovery, c.AutoMount = false, true
	c.TargetHash = ""
	if err := saveStore(dir, Store{Settings: defaultStore().Settings, Connections: []Connection{c}}); err != nil {
		t.Fatal(err)
	}
	backup := readFixture(t, dir, "settings.backup.json")
	writeFixture(t, dir, "settings.json", []byte(`broken original`))
	newer := "sd_" + strings.Repeat("a", 32)
	for _, name := range []string{"sd_" + c.Remote, newer} {
		if err := os.MkdirAll(filepath.Join(dir, "cache", "vfs", name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	rpc := &fakeRPC{handle: func(context.Context, string, any) (any, error) {
		return map[string]any{"remotes": []string{"sd_" + c.Remote, newer}}, nil
	}}
	s, _ := New(dir, rpc, "")
	if !s.Snapshot().BackupAvailable || s.RestoreSettings() != nil {
		t.Fatal("valid backup not restorable")
	}
	if !s.recoveryHold || !s.store.Connections[0].Recovery || s.settingsError != "" || s.startupWarning == "" {
		t.Fatal("restore lost recovery guards")
	}
	archives, _ := filepath.Glob(filepath.Join(dir, "settings-damaged-*.json"))
	if len(archives) != 1 || string(readFixture(t, dir, filepath.Base(archives[0]))) != "broken original" {
		t.Fatal("original not archived")
	}
	if string(readFixture(t, dir, "settings.backup.json")) != string(backup) {
		t.Fatal("backup overwritten during restore")
	}
	reopened, _ := New(dir, rpc, "")
	AutoMount(reopened)
	if !reopened.recoveryHold || len(rpc.calls) != 0 {
		t.Fatalf("restart authorized automatic operations: %v", rpc.calls)
	}
	if err := reopened.Mount(c.ID); err == nil || !strings.Contains(err.Error(), "cannot prove") {
		t.Fatalf("unknown destination allowed: %v", err)
	}
	reopened.store.Connections[0].Recovery = false
	if err := reopened.VerifyRecovery(); err == nil || !strings.Contains(err.Error(), "manual recovery") || !reopened.recoveryHold {
		t.Fatalf("newer data not protected: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cache", "vfs", newer)); err != nil {
		t.Fatal("newer cache lost")
	}
	for _, call := range rpc.calls {
		if call == "config/delete" || call == "mount/mount" {
			t.Fatalf("destructive recovery operation: %s", call)
		}
	}
}

func TestRecoveryVerificationAndRestoreFailures(t *testing.T) {
	t.Run("verified recovery", func(t *testing.T) {
		dir := t.TempDir()
		if err := saveStore(dir, defaultStore()); err != nil {
			t.Fatal(err)
		}
		writeFixture(t, dir, "settings.json", []byte(`null`))
		s, _ := New(dir, &fakeRPC{}, "")
		if err := s.RestoreSettings(); err != nil {
			t.Fatal(err)
		}
		if err := s.VerifyRecovery(); err != nil || s.recoveryHold {
			t.Fatalf("verification failed: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, recoveryMarker)); !os.IsNotExist(err) {
			t.Fatal("recovery marker remains")
		}
	})
	for _, failure := range []string{"no backup", "bad backup", "marker write", "archive read", "primary replacement"} {
		t.Run(failure, func(t *testing.T) {
			dir := t.TempDir()
			if err := saveStore(dir, defaultStore()); err != nil {
				t.Fatal(err)
			}
			writeFixture(t, dir, "settings.json", []byte(`broken`))
			var h windows.Handle
			switch failure {
			case "no backup":
				if err := os.Remove(filepath.Join(dir, "settings.backup.json")); err != nil {
					t.Fatal(err)
				}
			case "bad backup":
				writeFixture(t, dir, "settings.backup.json", []byte(`{}`))
			case "marker write":
				if err := os.Mkdir(filepath.Join(dir, recoveryMarker), 0700); err != nil {
					t.Fatal(err)
				}
			case "archive read":
				if err := os.Remove(filepath.Join(dir, "settings.json")); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Join(dir, "settings.json"), 0700); err != nil {
					t.Fatal(err)
				}
			case "primary replacement":
				path, _ := windows.UTF16PtrFromString(filepath.Join(dir, "settings.json"))
				var err error
				h, err = windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
				if err != nil {
					t.Fatal(err)
				}
				defer windows.CloseHandle(h)
			}
			s, _ := New(dir, nil, "")
			if s.RestoreSettings() == nil || s.settingsError == "" {
				t.Fatal("failed restoration reported success")
			}
			if failure != "archive read" && string(readFixture(t, dir, "settings.json")) != "broken" {
				t.Fatal("original settings changed")
			}
		})
	}
}

func TestFirstRunIsCommittedBeforeEngineInitialization(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir, nil, "")
	s.restart = func() (RPC, error) {
		if _, err := loadStore(dir); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dir, "settings.json")); err != nil {
			t.Fatal("engine started before initial save")
		}
		return &fakeRPC{}, nil
	}
	StartEngine(s)
	if s.settingsError != "" || s.engineError != "" || s.rpc == nil {
		t.Fatal("first run failed")
	}
	writeFixture(t, dir, "rclone.conf", []byte("existing encrypted config"))
	reopened, _ := New(dir, nil, "")
	if reopened.settingsError != "" {
		t.Fatal("reopening first profile failed")
	}
	if err := os.Remove(filepath.Join(dir, "settings.json")); err != nil {
		t.Fatal(err)
	}
	missing, _ := New(dir, nil, "")
	if missing.settingsError == "" || missing.SaveSettings(defaultStore().Settings) == nil {
		t.Fatal("missing settings authorized an empty replacement")
	}
}

func TestSaveSettingsSurfacesRollbackFailureWithoutRegistryWrites(t *testing.T) {
	s := testService(t, &fakeRPC{})
	old := s.store.Settings
	if err := os.Mkdir(filepath.Join(s.dir, "settings.json"), 0700); err != nil {
		t.Fatal(err)
	}
	var calls []bool
	s.autoStart = func(enabled bool) error {
		calls = append(calls, enabled)
		if len(calls) == 2 {
			return errors.New("rollback denied")
		}
		return nil
	}
	err := s.SaveSettings(Settings{AutoStart: true, CacheGiB: 8, Language: "pt-BR"})
	if err == nil || !strings.Contains(err.Error(), "rollback denied") || !strings.Contains(err.Error(), "could not save preferences") || s.startupWarning == "" || s.store.Settings != old || !reflect.DeepEqual(calls, []bool{true, false}) {
		t.Fatalf("rollback failure hidden: %v, %+v, %v", err, s.store.Settings, calls)
	}
	if err = os.Remove(filepath.Join(s.dir, "settings.json")); err != nil {
		t.Fatal(err)
	}
	s.autoStart = func(bool) error { return nil }
	if err = s.SaveSettings(old); err != nil || s.startupWarning != "" {
		t.Fatalf("reconciliation failed: %v", err)
	}
}

func TestRecoveryVerificationRefusesUnaccountedData(t *testing.T) {
	for _, scenario := range []string{"unknown credential", "unknown cache", "missing credential", "invalid inventory", "pending recovery", "active connection"} {
		t.Run(scenario, func(t *testing.T) {
			s := testService(t, &fakeRPC{})
			c := testConnection()
			c.Recovery = scenario == "pending recovery"
			s.store.Connections = []Connection{c}
			s.recoveryHold = true
			writeFixture(t, s.dir, recoveryMarker, []byte("protected"))
			remotes := []string{"sd_" + c.Remote}
			switch scenario {
			case "unknown credential":
				remotes = append(remotes, "sd_"+strings.Repeat("a", 32))
			case "unknown cache":
				if err := os.MkdirAll(filepath.Join(s.dataPath(), "vfsMeta", "sd_"+strings.Repeat("a", 32)), 0700); err != nil {
					t.Fatal(err)
				}
			case "missing credential":
				remotes = []string{}
			case "active connection":
				s.mounted[c.ID] = true
			}
			s.rpc = &fakeRPC{handle: func(context.Context, string, any) (any, error) {
				if scenario == "invalid inventory" {
					return nil, nil
				}
				return map[string]any{"remotes": remotes}, nil
			}}
			if s.VerifyRecovery() == nil || !s.recoveryHold {
				t.Fatal("unaccounted data released recovery protection")
			}
			if _, err := os.Stat(filepath.Join(s.dir, recoveryMarker)); err != nil {
				t.Fatal("durable guard lost")
			}
		})
	}
}

func TestRestoredCacheRetainsDestinationBinding(t *testing.T) {
	dir := t.TempDir()
	c := testConnection()
	c.Recovery = false
	config := map[string]string{"type": "ftp", "host": "original.example", "user": "original"}
	hash, err := checkDestination(c, config)
	if err != nil {
		t.Fatal(err)
	}
	c.TargetHash = hash
	if err = saveStore(dir, Store{Settings: defaultStore().Settings, Connections: []Connection{c}}); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, "settings.json", []byte(`{}`))
	if err = os.MkdirAll(filepath.Join(dir, "cache", "vfsMeta", "sd_"+c.Remote), 0700); err != nil {
		t.Fatal(err)
	}
	s, _ := New(dir, nil, "")
	if err = s.RestoreSettings(); err != nil {
		t.Fatal(err)
	}
	restored := s.store.Connections[0]
	if !restored.Recovery || restored.TargetHash != hash {
		t.Fatal("destination binding lost")
	}
	if _, err = checkDestination(restored, config); err != nil {
		t.Fatal(err)
	}
	config["host"] = "another.example"
	if _, err = checkDestination(restored, config); err == nil {
		t.Fatal("restored cache accepted another destination")
	}
}

func TestRestartCleanupFailureDoesNotInvalidateNewEngine(t *testing.T) {
	s := testService(t, &stoppedRPC{})
	if err := saveStore(s.dir, defaultStore()); err != nil {
		t.Fatal(err)
	}
	rpc := &fakeRPC{handle: func(context.Context, string, any) (any, error) { return nil, context.DeadlineExceeded }}
	s.restart = func() (RPC, error) { return rpc, nil }
	if err := s.RestartEngine(); err != nil || s.engineError != "" || s.maintenanceError == "" || s.rpc != rpc {
		t.Fatalf("cleanup failure invalidated engine restart: %v", err)
	}
}

func TestMalformedCacheTreesNeverAuthorizeCleanup(t *testing.T) {
	for _, tree := range []string{"cache", filepath.Join("cache", "vfs"), filepath.Join("cache", "vfsMeta")} {
		t.Run(tree, func(t *testing.T) {
			dir := t.TempDir()
			if err := saveStore(dir, defaultStore()); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, tree)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("unexpected data"), 0600); err != nil {
				t.Fatal(err)
			}
			rpc := &fakeRPC{}
			s, _ := New(dir, rpc, "")
			if s.maintenanceError == "" || len(rpc.calls) != 0 {
				t.Fatal("malformed cache authorized cleanup")
			}
			if string(readFixture(t, dir, tree)) != "unexpected data" {
				t.Fatal("cache changed")
			}
		})
	}
}
