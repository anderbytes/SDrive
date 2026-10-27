package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func portableFixture() PortableFile {
	c := testConnection()
	c.Provider, c.Name, c.Letter = "webdav", "Imported storage", "T"
	c.Recovery, c.AutoMount = false, true
	d := portableDefinition(c)
	d.Values["url"] = "https://storage.example.test/dav"
	return PortableFile{Format: "sdrive-portable", Version: 2, Connections: []PortableConnection{d}, Preferences: portablePreferences(defaultStore().Settings)}
}

func previewFixture(t *testing.T, s *Service, document PortableFile) *ImportPreview {
	t.Helper()
	b, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "portable.json")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	SetPortableFilePicker(s, func(bool) (string, error) { return path, nil })
	p, err := s.PreviewImport()
	if err != nil {
		t.Fatal(err)
	}
	return p
}

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
