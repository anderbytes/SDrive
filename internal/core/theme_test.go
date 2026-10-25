package core

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestThemePersistenceAndCompatibility(t *testing.T) {
	s := testService(t, nil)
	SetAutoStart(s, func(bool) error { return nil })
	for _, theme := range []string{"system", "light", "dark", ""} {
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
	s.store.Settings.Theme = "dark"
	s.access.locked = true
	if s.AccessStatus().Theme != "dark" || s.lockedView().Settings.Theme != "dark" {
		t.Fatal("lock screen lost the saved theme")
	}
}

func TestPortableThemeValidationAndSelectiveImport(t *testing.T) {
	for _, theme := range []string{"system", "light", "dark", ""} {
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
	for _, apply := range []bool{false, true} {
		s := testService(t, nil)
		s.store.Settings.Theme = "dark"
		document.Preferences.Theme = "light"
		preview := previewFixture(t, s, document)
		if err := s.CommitImport(preview.ID, preview.Document, apply); err != nil {
			t.Fatal(err)
		}
		want := "dark"
		if apply {
			want = "light"
		}
		loaded, err := loadStore(s.dir)
		if err != nil || loaded.Settings.Theme != want || portablePreferences(loaded.Settings).Theme != want {
			t.Fatalf("apply=%v: %+v, %v", apply, loaded.Settings, err)
		}
	}
}
