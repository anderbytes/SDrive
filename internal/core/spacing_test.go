package core

import "testing"

func TestPortableSpacingDefaultsAndValidation(t *testing.T) {
	for _, density := range []string{"", "compact", "balanced", "comfortable", "invalid"} {
		document := portableFixture()
		document.Preferences.Density = density
		err := validatePortableDocument(document)
		if (err != nil) != (density == "invalid") {
			t.Fatalf("density %q: %v", density, err)
		}
		settings := document.Preferences.settings(defaultStore().Settings)
		if density == "" && settings.Density != "balanced" {
			t.Fatal("old exports must default to balanced")
		}
		if density != "" && portablePreferences(settings).Density != density {
			t.Fatal("spacing lost in export")
		}
	}
}

func TestSpacingSaveReloadAndLegacyDefault(t *testing.T) {
	s, err := New(t.TempDir(), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	SetAutoStart(s, func(bool) error { return nil })
	for _, density := range []string{"compact", "balanced", "comfortable"} {
		settings := s.store.Settings
		settings.Density = density
		if err := s.SaveSettings(settings); err != nil {
			t.Fatal(err)
		}
		stored, err := loadStore(s.dir)
		if err != nil || stored.Settings.Density != density {
			t.Fatalf("spacing lost after reload: %v", err)
		}
	}
	settings := s.store.Settings
	settings.Density = "invalid"
	if s.SaveSettings(settings) == nil || s.store.Settings.Density != "comfortable" {
		t.Fatal("invalid spacing changed saved preferences")
	}
	legacy, err := decodeStore([]byte(`{"connections":[],"settings":{"autoStart":false,"cacheGiB":2}}`))
	if err != nil || legacy.Settings.Density != "balanced" {
		t.Fatalf("legacy spacing: %v", err)
	}
}
