package core

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestHiddenFileMountOptionsPreserveSecurity(t *testing.T) {
	for _, mode := range []string{"network", "local", "folder"} {
		for _, readOnly := range []bool{false, true} {
			for _, visibility := range []string{"", "system", "show", "hide"} {
				c := testConnection()
				c.MountMode, c.ReadOnly = mode, readOnly
				if mode == "folder" {
					c.MountMode, c.Letter, c.MountFolder = "local", "", filepath.Join(t.TempDir(), "mount")
				}
				c.HiddenFileVisibility = "show"
				baseline := mountOptions(c, defaultStore().Settings)
				c.HiddenFileVisibility = visibility
				actual := mountOptions(c, defaultStore().Settings)
				mount := actual["mountOpt"].(map[string]any)
				extra, _ := mount["ExtraOptions"].([]string)
				if slices.Contains(extra, "dothidden") != (visibility == "" || visibility == "system") {
					t.Fatalf("mode=%s readOnly=%t: hidden attribute option differs: %v", mode, readOnly, extra)
				}
				if hiddenFileVisibility(c) == "system" {
					if len(extra) == 1 {
						delete(mount, "ExtraOptions")
					} else {
						mount["ExtraOptions"] = extra[:len(extra)-1]
					}
				}
				delete(actual, "_filter")
				delete(baseline, "_filter")
				if !reflect.DeepEqual(actual, baseline) {
					t.Fatal("file visibility changed mount permissions or VFS options")
				}
			}
		}
	}
}

func TestDriveDisplayOptionsPersistAndTravelWithPortableConfiguration(t *testing.T) {
	for _, visibility := range []string{"", "system", "show", "hide"} {
		store := defaultStore()
		c := testConnection()
		c.HiddenFileVisibility = visibility
		store.Connections = []Connection{c}
		store.Settings.DriveStripeColorSource = "drive"
		raw, err := encodeStore(store)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := decodeStore(raw)
		if err != nil || !reflect.DeepEqual(loaded, store) {
			t.Fatal("drive display options did not survive reload", err)
		}
		document := portableFixture()
		document.Connections[0].HiddenFileVisibility = visibility
		document.Preferences = portablePreferences(store.Settings)
		raw, _ = json.Marshal(document)
		portable, err := decodePortable(raw)
		if err != nil || hiddenFileVisibility(portable.Connections[0].connection()) != hiddenFileVisibility(c) || portable.Preferences.settings(defaultStore().Settings).DriveStripeColorSource != "drive" {
			t.Fatal("portable configuration lost drive display options", err)
		}
	}
	legacy, err := decodeStore([]byte(`{"connections":[],"settings":{"autoStart":false,"cacheGiB":2}}`))
	if err != nil || legacy.Settings.DriveStripeColorSource != "theme" || hiddenFileVisibility(Connection{}) != "system" {
		t.Fatal("legacy display defaults changed", err)
	}
}

func TestDriveStripeSourceValidatesSavedAndPortablePreferences(t *testing.T) {
	s := testService(t, nil)
	SetAutoStart(s, func(bool) error { return nil })
	for _, source := range []string{"", "theme", "drive", "unsupported"} {
		settings := defaultStore().Settings
		settings.DriveStripeColorSource = source
		err := s.SaveSettings(settings)
		if (err != nil) != (source == "unsupported") {
			t.Fatalf("source=%q: save validation differs: %v", source, err)
		}
		document := portableFixture()
		document.Preferences.DriveStripeColorSource = source
		if (validatePortableDocument(document) != nil) != (source == "unsupported") {
			t.Fatalf("source=%q: portable validation differs", source)
		}
	}
}

func TestHiddenFileVisibilityCannotChangeDuringRecovery(t *testing.T) {
	for _, shown := range []string{"system", "show", "hide"} {
		rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
			if method == "config/get" {
				return map[string]string{"type": "ftp", "host": "example.test", "user": "user", "explicit_tls": "true"}, nil
			}
			return ConfigResult{}, nil
		}}
		s := testService(t, rpc)
		c := testConnection()
		c.HiddenFileVisibility, c.Recovery = shown, false
		c.TargetHash, _ = checkDestination(c, map[string]string{"type": "ftp", "host": "example.test", "user": "user"})
		c.Recovery = true
		s.store.Connections = []Connection{c}
		input := ConfigInput{Connection: c, Values: map[string]string{"host": "example.test", "user": "user", "pass": "new-password"}, Transport: "explicit"}
		changed := "show"
		if shown == changed {
			changed = "hide"
		}
		input.Connection.HiddenFileVisibility = changed
		if _, err := s.BeginConfig(input); err == nil {
			t.Fatal("recovery allowed changed visibility")
		}
		input.Connection.HiddenFileVisibility = shown
		id, err := s.BeginConfig(input)
		if err != nil {
			t.Fatal(err)
		}
		if session := waitSession(t, s, id); !session.Complete || hiddenFileVisibility(s.store.Connections[0]) != shown {
			t.Fatal("recovery lost file visibility")
		}
	}
}
