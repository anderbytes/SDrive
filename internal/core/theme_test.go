package core

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"
)

func TestThemePersistenceAndCompatibility(t *testing.T) {
	s := testService(t, nil)
	SetAutoStart(s, func(bool) error { return nil })
	for _, theme := range []string{"system", "light", "dark", "retro", "outline", "terminal", "editorial", "atelier", ""} {
		settings := s.store.Settings
		settings.Theme = theme
		if err := s.SaveSettings(settings); err != nil {
			t.Fatal(err)
		}
		want := theme
		if want == "" {
			want = "system"
		}
		loaded, err := loadStore(s.dir)
		if err != nil || loaded.Settings.Theme != want || s.AccessStatus().Theme != want {
			t.Fatalf("theme %q: got %+v, %v", theme, loaded.Settings, err)
		}
	}
	settings := s.store.Settings
	settings.Theme = "unsupported"
	if err := s.SaveSettings(settings); err == nil || s.store.Settings.Theme != "system" {
		t.Fatal("invalid theme changed saved preferences")
	}
	for _, data := range []string{
		`{"connections":[],"settings":{"autoStart":false,"cacheGiB":3}}`,
		`{"connections":[],"settings":{"autoStart":false,"cacheGiB":3,"theme":"unsupported"}}`,
	} {
		loaded, err := decodeStore([]byte(data))
		if err != nil || loaded.Settings.Theme != "system" || loaded.Settings.CacheGiB != 3 {
			t.Fatalf("legacy/invalid theme: %+v, %v", loaded.Settings, err)
		}
	}
	s.access.locked = true
	for _, theme := range []string{"dark", "retro", "outline", "terminal", "editorial", "atelier"} {
		s.store.Settings.Theme = theme
		if s.AccessStatus().Theme != theme || s.lockedView().Settings.Theme != theme {
			t.Fatal("lock screen lost the saved theme")
		}
	}
}

func TestPortableThemeValidationAndSelectiveImport(t *testing.T) {
	for _, theme := range []string{"system", "light", "dark", "retro", "outline", "terminal", "editorial", "atelier", ""} {
		document := portableFixture()
		document.Preferences.Theme = theme
		data, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		if theme == "" {
			data = bytes.Replace(data, []byte(`"theme":"",`), nil, 1)
		}
		decoded, err := decodePortable(data)
		want := theme
		if want == "" {
			want = "system"
		}
		if err != nil || decoded.Preferences.Theme != want {
			t.Fatalf("portable theme %q: %+v, %v", theme, decoded.Preferences, err)
		}
	}
	document := portableFixture()
	document.Preferences.Theme = "unsupported"
	if validatePortableDocument(document) == nil {
		t.Fatal("invalid portable theme accepted")
	}
	if runtime.GOOS == "linux" {
		document.Connections[0].MountFolder = filepath.Join(t.TempDir(), "Mount")
		document.Connections[0].Letter = ""
		document.Connections[0].MountMode = "local"
	}
	for _, apply := range []bool{false, true} {
		s := testService(t, nil)
		s.store.Settings.Theme = "dark"
		document.Preferences.Theme = "atelier"
		preview := previewFixture(t, s, document)
		if err := s.CommitImport(preview.ID, preview.Document, apply); err != nil {
			t.Fatal(err)
		}
		want := "dark"
		if apply {
			want = "atelier"
		}
		loaded, err := loadStore(s.dir)
		if err != nil || loaded.Settings.Theme != want || portablePreferences(loaded.Settings).Theme != want {
			t.Fatalf("apply=%v: %+v, %v", apply, loaded.Settings, err)
		}
	}
}
