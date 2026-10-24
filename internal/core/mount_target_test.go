package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRuntimeSettingsCompatibilityAndValidation(t *testing.T) {
	for _, version := range []string{"", `"version":1,`, `"version":2,`, `"version":3,`} {
		store, err := decodeStore([]byte(`{` + version + `"connections":[],"settings":{"autoStart":false,"cacheGiB":2}}`))
		age, delay := cacheDurations(store.Settings)
		if err != nil || age != 24*time.Hour || delay != 5*time.Second {
			t.Fatal("legacy cache timing changed")
		}
	}
	store := defaultStore()
	age, delay := 48, 0
	store.Settings.CacheMaxAgeHours, store.Settings.WriteBackSeconds = &age, &delay
	c := testConnection()
	c.Letter, c.MountFolder, c.MountMode, c.DirectoryCacheSeconds = "", filepath.Join(t.TempDir(), "mount"), "local", 30
	store.Connections = []Connection{c}
	raw, err := encodeStore(store)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := decodeStore(raw)
	if err != nil || loaded.Connections[0] != c || *loaded.Settings.WriteBackSeconds != 0 || *loaded.Settings.CacheMaxAgeHours != 48 {
		t.Fatal("runtime settings did not round trip")
	}
	for _, invalid := range []int{-1, 3601} {
		store.Settings.WriteBackSeconds = &invalid
		if _, err := encodeStore(store); err == nil {
			t.Fatal("invalid upload delay accepted")
		}
	}
}

func TestMountTargetsRejectExistingFoldersAndPreserveLetterDefaults(t *testing.T) {
	s := testService(t, nil)
	c := testConnection()
	if err := validateConnection(&c); err != nil || mountPoint(c) != "S:" || mountKey("s:\\") != mountKey("S:") {
		t.Fatal("letter compatibility changed")
	}
	c.Letter, c.MountMode, c.MountFolder = "", "local", filepath.Join(t.TempDir(), "new-mount")
	if err := validateConnection(&c); err != nil {
		t.Fatal(err)
	}
	if err := s.availableMount(c); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(c.MountFolder, 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.availableMount(c); err == nil {
		t.Fatal("existing empty mount folder accepted")
	}
	if err := os.WriteFile(filepath.Join(c.MountFolder, "keep.txt"), []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.availableMount(c); err == nil {
		t.Fatal("nonempty mount folder accepted")
	}
	if b, err := os.ReadFile(filepath.Join(c.MountFolder, "keep.txt")); err != nil || string(b) != "preserve" {
		t.Fatal("mount validation changed existing contents")
	}
	c.MountMode = "network"
	if validateConnection(&c) == nil {
		t.Fatal("network folder mount accepted")
	}
}

func TestMountRuntimeOptionsUseTimingAndPrivateLocalPermissions(t *testing.T) {
	c := testConnection()
	defaults := mountOptions(c, defaultStore().Settings)
	vfs := defaults["vfsOpt"].(map[string]any)
	if vfs["CacheMaxAge"] != int64(24*time.Hour) || vfs["WriteBack"] != int64(5*time.Second) || vfs["DirCacheTime"] != int64(300*time.Second) || defaults["mountOpt"].(map[string]any)["NetworkMode"] != true {
		t.Fatal("runtime defaults changed")
	}
	age, delay := 48, 0
	settings := Settings{CacheGiB: 7, CacheMaxAgeHours: &age, WriteBackSeconds: &delay}
	c.MountMode, c.DirectoryCacheSeconds = "local", 30
	options := mountOptions(c, settings)
	vfs = options["vfsOpt"].(map[string]any)
	if vfs["FilePerms"] != uint32(0600) || vfs["DirPerms"] != uint32(0700) || vfs["PollInterval"] != int64(0) || vfs["WriteBack"] != int64(0) || vfs["CacheMaxAge"] != int64(48*time.Hour) || options["mountOpt"].(map[string]any)["NetworkMode"] != false {
		t.Fatal("local runtime options differ")
	}
}

func TestMountInventoryAcceptsFolderPathsAndRejectsDuplicateAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mount")
	for _, duplicate := range []bool{false, true} {
		mounts := []engineMount{{Fs: "remote:", MountPoint: path}}
		if duplicate {
			mounts = append(mounts, engineMount{Fs: "other:", MountPoint: path + `\`})
		}
		rpc := &fakeRPC{handle: func(context.Context, string, any) (any, error) { return map[string]any{"mountPoints": mounts}, nil }}
		_, err := readMounts(context.Background(), rpc)
		if (err != nil) != duplicate {
			t.Fatalf("inventory validation: %v", err)
		}
	}
}

func TestExplorerUsesTheSavedFolderMountDestination(t *testing.T) {
	s := testService(t, nil)
	c := testConnection()
	c.Letter, c.MountMode, c.MountFolder = "", "local", filepath.Join(t.TempDir(), "mount")
	s.store.Connections = []Connection{c}
	s.mounted[c.ID] = true
	opened := ""
	s.open = func(target string) error { opened = target; return nil }
	if err := s.OpenExplorer(c.ID); err != nil || opened != c.MountFolder {
		t.Fatal("Explorer received the wrong folder mount")
	}
	s.mounted[c.ID] = false
	if err := s.OpenExplorer(c.ID); err == nil {
		t.Fatal("Explorer opened a disconnected folder")
	}
}
