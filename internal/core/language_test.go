package core

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSmokeStartupCallbackIsolatesPreferenceWrites(t *testing.T) {
	s, err := New(t.TempDir(), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	settings := s.store.Settings
	settings.CacheGiB = 3
	calls := 0
	SetAutoStart(s, func(bool) error { calls++; return nil })
	if err := s.SaveSettings(settings); err != nil || calls != 1 {
		t.Fatalf("isolated preference save: calls=%d, err=%v", calls, err)
	}
	loaded, err := loadStore(s.dir)
	if err != nil || loaded.Settings.CacheGiB != 3 {
		t.Fatalf("preferences were not persisted: %v", err)
	}
	blocked := errors.New("startup callback rejected update")
	SetAutoStart(s, func(bool) error { return blocked })
	settings.CacheGiB = 4
	if err := s.SaveSettings(settings); !errors.Is(err, blocked) || s.store.Settings.CacheGiB != 3 {
		t.Fatal("startup callback error did not prevent preference changes")
	}
}

func TestLanguageSettingsPersistenceAndCompatibility(t *testing.T) {
	dir := t.TempDir()
	store, err := loadStore(dir)
	if err != nil || store.Settings.Language != "en" {
		t.Fatalf("new settings must default to English: %v", err)
	}
	for _, language := range []string{"en", "pt-BR", "", "unsupported"} {
		store.Settings.Language = language
		if err := saveStore(dir, store); err != nil {
			t.Fatal(err)
		}
		loaded, err := loadStore(dir)
		want := language
		if language == "" || language == "unsupported" {
			want = "en"
		}
		if err != nil || loaded.Settings.Language != want {
			t.Fatalf("language %q: got %q, %v", language, loaded.Settings.Language, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"connections":[],"settings":{"cacheGiB":3,"autoStart":true}}`), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadStore(dir)
	if err != nil || loaded.Settings.Language != "en" || loaded.Settings.CacheGiB != 3 || !loaded.Settings.AutoStart {
		t.Fatalf("older preferences were not preserved: %+v, %v", loaded.Settings, err)
	}
	service := &Service{}
	if err := service.SaveSettings(Settings{CacheGiB: 2, Language: "unsupported"}); err == nil {
		t.Fatal("unsupported language accepted")
	}
}
