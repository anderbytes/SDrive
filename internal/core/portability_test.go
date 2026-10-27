//go:build windows

package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPortableExportExcludesCredentialsAndRecovery(t *testing.T) {
	for _, p := range Catalog() {
		t.Run(p.ID, func(t *testing.T) {
			config := map[string]string{"type": p.Backend, "token": "sentinel-token", "client_secret": "sentinel-oauth", "pass": "sentinel-password", "key": "sentinel-key", "sas_url": "https://example.test/?sig=sentinel-sas", "key_file": "sentinel-private-path", "service_account_file": "sentinel-service-account", "access_key_id": "sentinel-access-key", "account": "synthetic-account"}
			for _, f := range p.Fields {
				if f.Secret || f.File || f.Key == "client_id" || f.Key == "access_key_id" {
					config[f.Key] = "sentinel-" + f.Key
				}
			}
			if p.ID == "b2" {
				config["account"] = "sentinel-b2-key-id"
			}
			if p.ID == "ftp" {
				config["explicit_tls"] = "true"
			}
			rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
				if method == "config/get" {
					return config, nil
				}
				return map[string]any{}, nil
			}}
			s := testService(t, rpc)
			c := testConnection()
			c.Provider = p.ID
			c.Binding = "sentinel-binding"
			c.TargetHash = "sentinel-target"
			s.store.Connections = []Connection{c}
			s.store.Settings.AutoStart = true
			path := filepath.Join(t.TempDir(), "export.json")
			SetPortableFilePicker(s, func(bool) (string, error) { return path, nil })
			if err := s.ExportConfiguration(); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(b), "sentinel") || strings.Contains(string(b), `"remote"`) || strings.Contains(string(b), `"autoStart"`) || strings.Contains(string(b), `"targetHash"`) {
				t.Fatalf("credential or local identity exported: %s", b)
			}
			if _, err := decodePortable(b); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPortableImportPreservesRecoveryAndRequiresAuthentication(t *testing.T) {
	rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
		if method == "operations/list" {
			return map[string]any{"list": []any{}}, nil
		}
		return ConfigResult{}, nil
	}}
	s := testService(t, rpc)
	old := testConnection()
	old.Recovery = true
	s.store.Connections = []Connection{old}
	if err := saveStore(s.dir, s.store); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(s.dataPath(), "vfsMeta", "sd_"+old.Remote)
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	metadata := filepath.Join(root, "pending")
	if err := os.WriteFile(metadata, []byte(`{"Dirty":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	d := portableFixture()
	d.Preferences.Language = "pt-BR"
	p := previewFixture(t, s, d)
	before := len(rpc.calls)
	if err := s.CommitImport(p.ID, p.Document, false); err != nil {
		t.Fatal(err)
	}
	if len(rpc.calls) != before || len(s.store.Connections) != 1 || s.store.Connections[0] != old || s.store.Settings.Language != "en" || len(s.store.ImportDrafts) != 1 || len(s.mounted) != 0 {
		t.Fatal("import touched operational state or mounted a draft")
	}
	loaded, err := loadStore(s.dir)
	if err != nil || len(loaded.ImportDrafts) != 1 || loaded.ImportDrafts[0].ID == old.ID {
		t.Fatal("draft did not persist independently", err)
	}
	if raw, err := os.ReadFile(metadata); err != nil || string(raw) != `{"Dirty":true}` {
		t.Fatal("pending recovery changed")
	}
	draftID := s.store.ImportDrafts[0].ID
	c := d.Connections[0].connection()
	id, err := s.BeginConfig(ConfigInput{Connection: c, Values: d.Connections[0].Values, Guided: true, ImportDraftID: draftID})
	if err != nil {
		t.Fatal(err)
	}
	ss := waitSession(t, s, id)
	if !ss.Authenticated || len(s.store.ImportDrafts) != 1 {
		t.Fatal("setup removed draft before confirmation")
	}
	if err := s.EndConfig(id); err != nil {
		t.Fatal(err)
	}
	if len(s.store.ImportDrafts) != 1 {
		t.Fatal("cancellation discarded draft")
	}
	// Confirmation commits the new connection and removes only its draft in the same write.
	id, err = s.BeginConfig(ConfigInput{Connection: c, Values: d.Connections[0].Values, Guided: true, ImportDraftID: draftID})
	if err != nil {
		t.Fatal(err)
	}
	ss = waitSession(t, s, id)
	if !ss.Authenticated {
		t.Fatal("reauthentication failed")
	}
	settingsPath, _ := windows.UTF16PtrFromString(filepath.Join(s.dir, "settings.json"))
	lock, lockErr := windows.CreateFile(settingsPath, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, 0, 0)
	if lockErr != nil {
		t.Fatal(lockErr)
	}
	_, failedCommit := s.ConfirmConfig(id, c.Path, mountTarget(c))
	_ = windows.CloseHandle(lock)
	if failedCommit == nil || len(s.store.ImportDrafts) != 1 || len(s.store.Connections) != 1 {
		t.Fatal("failed authentication commit discarded the draft or partially saved")
	}
	_, err = s.ConfirmConfig(id, c.Path, mountTarget(c))
	if err != nil || len(s.store.ImportDrafts) != 0 || len(s.store.Connections) != 2 || s.store.Connections[0] != old {
		t.Fatal("draft confirmation failed", err)
	}
}

func TestPortableImportRejectsInvalidFilesAndConflictsAtomically(t *testing.T) {
	d := portableFixture()
	optional := portableFixture()
	optional.Connections[0].Provider = "s3"
	optional.Connections[0].Values = map[string]string{"provider": "AWS", "endpoint": ""}
	if err := validatePortableDocument(optional); err != nil {
		t.Fatal("empty optional endpoint rejected", err)
	}
	optional.Connections[0].Values["provider"] = "unsupported"
	if validatePortableDocument(optional) == nil {
		t.Fatal("invalid provider option accepted")
	}
	b, _ := json.Marshal(d)
	for _, raw := range [][]byte{[]byte("null"), []byte("{}"), []byte(`{"format":"sdrive-portable","version":99}`), append(append([]byte{}, b...), []byte(" {}")...), []byte(strings.Replace(string(b), `"values":{`, `"password":"sentinel","values":{`, 1)), []byte(strings.Repeat(" ", portableLimit+1))} {
		if _, err := decodePortable(raw); err == nil {
			t.Fatal("invalid file accepted")
		}
	}
	for _, key := range []string{"token", "client_secret", "key_file", "service_account_file", "access_key_id", "unknown"} {
		d := portableFixture()
		d.Connections[0].Values[key] = "sentinel"
		if validatePortableDocument(d) == nil {
			t.Fatal("unsafe field accepted", key)
		}
	}
	for _, address := range []string{"https://user:password@example.test", "https://example.test/?token=secret", "https://example.test/#token"} {
		d := portableFixture()
		d.Connections[0].Values["url"] = address
		if validatePortableDocument(d) == nil {
			t.Fatal("unsafe URL accepted")
		}
	}
	for _, failure := range []string{"name", "letter", "occupied", "hold", "stale", "write"} {
		t.Run(failure, func(t *testing.T) {
			s := testService(t, &fakeRPC{})
			old := testConnection()
			s.store.Connections = []Connection{old}
			if err := saveStore(s.dir, s.store); err != nil {
				t.Fatal(err)
			}
			d := portableFixture()
			if failure == "name" {
				d.Connections[0].Name = old.Name
			}
			if failure == "letter" {
				d.Connections[0].Letter = old.Letter
			}
			p := previewFixture(t, s, d)
			id := p.ID
			if failure == "occupied" {
				s.driveUsed = func(string) bool { return true }
			}
			if failure == "hold" {
				s.recoveryHold = true
			}
			if failure == "stale" {
				id = "stale"
			}
			if failure == "write" {
				path, _ := windows.UTF16PtrFromString(filepath.Join(s.dir, "settings.json"))
				h, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, 0, 0)
				if err != nil {
					t.Fatal(err)
				}
				defer windows.CloseHandle(h)
			}
			before := readFixture(t, s.dir, "settings.json")
			original := s.store
			if s.CommitImport(id, p.Document, true) == nil {
				t.Fatal("invalid import succeeded")
			}
			if !reflect.DeepEqual(s.store, original) || string(readFixture(t, s.dir, "settings.json")) != string(before) {
				t.Fatal("failed import partially changed profile")
			}
		})
	}
}

func TestPortableExportCannotOverwriteProfileOrCache(t *testing.T) {
	s := testService(t, &fakeRPC{})
	if err := saveStore(s.dir, s.store); err != nil {
		t.Fatal(err)
	}
	before := readFixture(t, s.dir, "settings.json")
	SetPortableFilePicker(s, func(bool) (string, error) { return filepath.Join(s.dir, "settings.json"), nil })
	if s.ExportConfiguration() == nil || string(readFixture(t, s.dir, "settings.json")) != string(before) {
		t.Fatal("export overwrote local profile")
	}
}
