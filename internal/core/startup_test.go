package core

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreStartupPreservesProfileAcrossReinstallation(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			dir := t.TempDir()
			store := defaultStore()
			store.Settings.AutoStart = enabled
			store.AccessProtected = true
			connection := testConnection()
			connection.Recovery = false
			store.Connections = []Connection{connection}
			if err := saveStore(dir, store); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(dir, "settings.json"))
			if err != nil {
				t.Fatal(err)
			}
			entry, executable, writes := "old-path", "new-path", 0
			matches := func(want bool) (bool, error) {
				return (!want && entry == "") || (want && entry == executable), nil
			}
			update := func(want bool) error {
				writes++
				entry = ""
				if want {
					entry = executable
				}
				return nil
			}
			for _, previousEntry := range []string{"old-path", ""} {
				entry = previousEntry // Uninstallation removes only the Run entry.
				if err := restoreStartup(dir, matches, update); err != nil {
					t.Fatal(err)
				}
				count := writes
				if err := restoreStartup(dir, matches, update); err != nil || writes != count {
					t.Fatalf("matching startup was rewritten: %v", err)
				}
				if ok, _ := matches(enabled); !ok {
					t.Fatal("saved preference was not restored")
				}
			}
			after, err := os.ReadFile(filepath.Join(dir, "settings.json"))
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("profile changed during restoration")
			}
		})
	}
}

func TestRestoreStartupRejectsUntrustedSettings(t *testing.T) {
	for _, scenario := range []string{"missing", "invalid", "incompatible", "marker", "connection", "unreadable"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			store := defaultStore()
			store.Settings.AutoStart = true
			if scenario == "connection" {
				store.Connections = []Connection{testConnection()}
			}
			if scenario != "missing" && scenario != "unreadable" {
				if err := saveStore(dir, store); err != nil {
					t.Fatal(err)
				}
			}
			switch scenario {
			case "invalid":
				writeFixture(t, dir, "settings.json", []byte("invalid"))
			case "incompatible":
				writeFixture(t, dir, "settings.json", []byte(`{"version":999}`))
			case "marker":
				writeFixture(t, dir, recoveryMarker, []byte("recovery"))
			case "unreadable":
				if err := os.Mkdir(filepath.Join(dir, "settings.json"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			err := restoreStartup(dir, func(bool) (bool, error) { t.Fatal("untrusted profile reached registry"); return false, nil }, func(bool) error { t.Fatal("untrusted profile changed registry"); return nil })
			if (err == nil) != (scenario == "missing") {
				t.Fatalf("unexpected restoration result: %v", err)
			}
		})
	}
}

func TestRestoreStartupReportsRegistryFailures(t *testing.T) {
	dir := t.TempDir()
	if err := saveStore(dir, defaultStore()); err != nil {
		t.Fatal(err)
	}
	denied := errors.New("registry denied")
	if err := restoreStartup(dir, func(bool) (bool, error) { return false, denied }, func(bool) error { t.Fatal("write after failed read"); return nil }); !errors.Is(err, denied) {
		t.Fatal(err)
	}
	if err := restoreStartup(dir, func(bool) (bool, error) { return false, nil }, func(bool) error { return denied }); !errors.Is(err, denied) {
		t.Fatal(err)
	}
	if err := restoreStartup(dir, func(bool) (bool, error) { return false, nil }, func(bool) error { return nil }); err == nil {
		t.Fatal("unconfirmed write reported success")
	}
}

func TestReconcileStartupReportsRecoveryWithoutRegistryWrites(t *testing.T) {
	s := testService(t, nil)
	s.recoveryHold = true
	SetAutoStart(s, func(bool) error { t.Fatal("recovery changed registry"); return nil })
	if err := ReconcileStartup(s); err == nil || s.startupWarning == "" {
		t.Fatal("recovery failure hidden")
	}
	s.recoveryHold = false
	if err := ReconcileStartup(s); err != nil || s.startupWarning != "" {
		t.Fatal("missing profile reconciliation failed")
	}
}

func TestReconcileStartupUsesDesktopCallbackWhileLocked(t *testing.T) {
	s := testService(t, nil)
	s.store.Settings.AutoStart = true
	if err := saveStore(s.dir, s.store); err != nil {
		t.Fatal(err)
	}
	s.access.locked = true
	enabled, writes := false, 0
	SetAutoStart(s, func(want bool) error { enabled = want; writes++; return nil })
	if err := reconcileStartup(s, func(want bool) (bool, error) { return enabled == want, nil }); err != nil || writes != 1 || s.startupWarning != "" || !s.access.locked {
		t.Fatalf("locked startup reconciliation: writes=%d, err=%v", writes, err)
	}
	SetAutoStart(s, func(bool) error { return errors.New("registry denied") })
	if err := reconcileStartup(s, func(bool) (bool, error) { return false, nil }); err == nil || s.startupWarning == "" {
		t.Fatal("registry failure warning missing")
	}
}
