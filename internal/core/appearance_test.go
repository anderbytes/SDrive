package core

import (
	"reflect"
	"testing"
)

func TestAppearancePersistenceAndPortableRoundTrip(t *testing.T) {
	s, err := New(t.TempDir(), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	SetAutoStart(s, func(bool) error { return nil })
	p := appearanceDefaults()
	p.HiddenCardDetails = 15
	p.DisconnectDelayMS = 0
	p.Sidebar = "auto"
	p.TextSize = "large"
	p.CapacityParts = 1
	settings := s.store.Settings
	settings.Appearance = &p
	if err := s.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	stored, err := loadStore(s.dir)
	if err != nil || !reflect.DeepEqual(stored.Settings.Appearance, &p) {
		t.Fatalf("appearance lost: %v", err)
	}
	doc := portableFixture()
	doc.Preferences = portablePreferences(settings)
	if err := validatePortableDocument(doc); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(doc.Preferences.settings(defaultStore().Settings).Appearance, &p) {
		t.Fatal("portable appearance lost")
	}
	legacy, err := decodeStore([]byte(`{"connections":[],"settings":{"autoStart":false,"cacheGiB":2}}`))
	if err != nil || legacy.Settings.Appearance != nil {
		t.Fatal("legacy appearance", err)
	}
	for _, mutate := range []func(*AppearancePreferences){func(p *AppearancePreferences) { p.DisconnectDelayMS = -1 }, func(p *AppearancePreferences) { p.DisconnectDelayMS = 60001 }, func(p *AppearancePreferences) { p.CapacityParts = 0 }, func(p *AppearancePreferences) { p.CapacityParts = 8 }, func(p *AppearancePreferences) { p.HiddenCardDetails = 16 }, func(p *AppearancePreferences) { p.TextSize = "invalid" }, func(p *AppearancePreferences) { p.Motion = "invalid" }, func(p *AppearancePreferences) { p.Sidebar = "invalid" }, func(p *AppearancePreferences) { p.DisconnectSize = "invalid" }} {
		bad := appearanceDefaults()
		mutate(&bad)
		settings.Appearance = &bad
		if s.SaveSettings(settings) == nil {
			t.Fatal("invalid preference accepted")
		}
		doc.Preferences.Appearance = &bad
		if validatePortableDocument(doc) == nil {
			t.Fatal("invalid imported preference accepted")
		}
	}
}
