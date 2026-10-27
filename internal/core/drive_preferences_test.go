package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLegacyDrivePreferencesPreserveExistingBehavior(t *testing.T) {
	for _, visibility := range []string{"", `,"showHiddenFiles":true`, `,"showHiddenFiles":false`} {
		for _, open := range []bool{false, true} {
			c := testConnection()
			raw, _ := json.Marshal(c)
			raw = append(raw[:len(raw)-1], []byte(visibility+"}")...)
			settings := `{"autoStart":false,"cacheGiB":2,"openExplorerAfterConnect":false}`
			if open {
				settings = strings.Replace(settings, ":false}", ":true}", 1)
			}
			for _, version := range []string{"", `"version":5,`} {
				data := []byte("{" + version + `"connections":[` + string(raw) + `],"settings":` + settings + "}")
				store, err := decodeStore(data)
				if err != nil {
					t.Fatal(err)
				}
				mode := "show"
				if strings.Contains(visibility, "false") {
					mode = "system"
				}
				if store.Connections[0].HiddenFileVisibility != mode || store.Connections[0].OpenMountAfterConnect != open {
					t.Fatalf("lost legacy preference: %+v", store.Connections[0])
				}
				encoded, err := encodeStore(store)
				if err != nil || strings.Contains(string(encoded), "showHiddenFiles") {
					t.Fatal("legacy fields were persisted", err)
				}
				again, err := decodeStore(encoded)
				if err != nil || !reflect.DeepEqual(store, again) {
					t.Fatal("migration was not stable", err)
				}
			}
		}
	}
	document := portableFixture()
	raw, _ := json.Marshal(document)
	var legacy map[string]any
	_ = json.Unmarshal(raw, &legacy)
	legacy["version"] = 1
	legacy["preferences"].(map[string]any)["openExplorerAfterConnect"] = true
	entry := legacy["connections"].([]any)[0].(map[string]any)
	delete(entry, "hiddenFileVisibility")
	delete(entry, "openExplorerAfterConnect")
	entry["showHiddenFiles"] = false
	raw, _ = json.Marshal(legacy)
	migrated, err := decodePortable(raw)
	if err != nil || migrated.Version != 2 || migrated.Connections[0].HiddenFileVisibility != "system" || !migrated.Connections[0].OpenMountAfterConnect {
		t.Fatal("legacy portable preferences lost", err, migrated)
	}
}

func TestReorderConnectionsPersistsWithoutChangingMountedDrives(t *testing.T) {
	s := testService(t, nil)
	first, second := testConnection(), testConnection()
	second.ID, second.Remote, second.Letter = newID(), newID(), "T"
	s.store.Connections = []Connection{first, second}
	s.mounted[first.ID] = true
	for _, invalid := range [][]string{{first.ID}, {first.ID, first.ID}, {first.ID, "missing"}} {
		if s.ReorderConnections(invalid) == nil {
			t.Fatal("invalid order accepted")
		}
	}
	if err := s.ReorderConnections([]string{second.ID, first.ID}); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadStore(s.dir)
	if err != nil || !reflect.DeepEqual(loaded.Connections, []Connection{second, first}) || !s.mounted[first.ID] {
		t.Fatal("order not saved or mount changed", err)
	}
	before := append([]Connection{}, s.store.Connections...)
	s.dir = filepath.Join(s.dir, "settings.json", "directory")
	if s.ReorderConnections([]string{first.ID, second.ID}) == nil || !reflect.DeepEqual(s.store.Connections, before) {
		t.Fatal("failed save did not restore order")
	}
	s.access.locked = true
	if s.ReorderConnections([]string{second.ID, first.ID}) == nil {
		t.Fatal("locked order changed")
	}
}

func TestForcedHiddenFilesUseMountLocalFilterOnly(t *testing.T) {
	c := testConnection()
	c.HiddenFileVisibility = "hide"
	options := mountOptions(c, defaultStore().Settings)
	if !reflect.DeepEqual(options["_filter"], map[string]any{"ExcludeRule": []string{".*", ".*/**"}}) {
		t.Fatal("hidden filter missing")
	}
	for _, mode := range []string{"system", "show", ""} {
		c.HiddenFileVisibility = mode
		if _, ok := mountOptions(c, defaultStore().Settings)["_filter"]; ok {
			t.Fatal("filter leaked to another mount mode")
		}
	}
	c.HiddenFileVisibility = "invalid"
	if validateConnection(&c) == nil {
		t.Fatal("unknown visibility accepted")
	}
}

func TestConnectionDetailsSanitizeURLsAndExcludeCredentials(t *testing.T) {
	calls := []string{}
	s := testService(t, &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
		calls = append(calls, method)
		return map[string]string{"type": "webdav", "url": "https://private:password@example.test/dav?secret=token#private", "user": "synthetic-user", "pass": "password-sentinel", "token": "token-sentinel"}, nil
	}})
	c := testConnection()
	c.Provider = "webdav"
	s.store.Connections = []Connection{c}
	s.mounted[c.ID] = true
	details, err := s.ConnectionDetails(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(details)
	text := string(raw)
	if !strings.Contains(text, "https://example.test/dav") || !strings.Contains(text, "synthetic-user") {
		t.Fatal("public details missing", text)
	}
	for _, secret := range []string{"password", "token", "private", "secret="} {
		if strings.Contains(text, secret) {
			t.Fatal("sensitive details exposed", text)
		}
	}
	for _, call := range calls {
		if call != "config/get" {
			t.Fatal("detail reader performed a connection operation", call)
		}
	}
	s.access.locked = true
	if _, err := s.ConnectionDetails(c.ID); err == nil {
		t.Fatal("locked details exposed")
	}
}

func TestDiagnosticReportExportPreservesReviewedTextAndHandlesCancellation(t *testing.T) {
	s := testService(t, nil)
	report := "SDrive diagnostic report v1\nReviewed report\n"
	path := filepath.Join(t.TempDir(), "report.txt")
	SetDiagnosticReportPicker(s, func() (string, error) { return path, nil })
	saved, err := s.ExportDiagnosticReport(report)
	raw, readErr := os.ReadFile(path)
	if err != nil || readErr != nil || !saved || string(raw) != report {
		t.Fatal("export changed reviewed text", err, readErr)
	}
	SetDiagnosticReportPicker(s, func() (string, error) { return "", nil })
	if saved, err := s.ExportDiagnosticReport(report); saved || err != nil {
		t.Fatal("cancel treated as an error", err)
	}
	SetDiagnosticReportPicker(s, func() (string, error) { s.mu.Lock(); s.access.locked = true; s.mu.Unlock(); return path, nil })
	if saved, err := s.ExportDiagnosticReport(report + "late"); saved || err == nil {
		t.Fatal("export survived application lock")
	}
	raw, _ = os.ReadFile(path)
	if string(raw) != report {
		t.Fatal("lock did not preserve previous file")
	}
}

func TestConnectionDetailsIncludePublicProviderDefaults(t *testing.T) {
	s := testService(t, &fakeRPC{handle: func(context.Context, string, any) (any, error) {
		return map[string]string{"type": "pcloud", "token": "private-token"}, nil
	}})
	c := testConnection()
	c.Provider = "pcloud"
	s.store.Connections = []Connection{c}
	details, err := s.ConnectionDetails(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(details, []ConnectionDetail{{Label: "Region", Value: "api.pcloud.com"}}) {
		t.Fatalf("public provider defaults missing or secrets exposed: %+v", details)
	}
}
