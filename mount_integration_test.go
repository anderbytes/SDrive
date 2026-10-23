package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sdrive/internal/engine"
	"sdrive/internal/platform"
)

func TestWindowsMountLifecycle(t *testing.T) {
	if os.Getenv("SDRIVE_TEST_MOUNT") != "1" {
		t.Skip("opt-in: SDRIVE_TEST_MOUNT=1; uses only a temporary folder and free drive letter")
	}
	if !platform.WinFspInstalled() {
		t.Skip("WinFsp not installed")
	}
	dir := t.TempDir()
	if _, err := platform.Key(dir); err != nil {
		t.Fatal(err)
	}
	binary, err := engine.FindBinary()
	if err != nil {
		t.Fatal(err)
	}
	if err = engine.Init(binary, dir); err != nil {
		t.Fatal(err)
	}
	rpc, err := engine.Start(binary, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rpc.Close()
	letter := ""
	for c := 'Z'; c >= 'D'; c-- {
		if !platform.DriveUsed(string(c)) {
			letter = string(c)
			break
		}
	}
	if letter == "" {
		t.Skip("no available drive letter")
	}
	remote := filepath.Join(dir, "files")
	_ = os.MkdirAll(remote, 0700)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	mount := letter + ":"
	if err = rpc.Call(ctx, "mount/mount", map[string]any{"fs": remote, "mountPoint": mount, "mountOpt": map[string]any{"NetworkMode": true}, "vfsOpt": map[string]any{"CacheMode": "full", "WriteBack": int64(100 * time.Millisecond)}}, nil); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = rpc.Call(context.Background(), "mount/unmount", map[string]string{"mountPoint": mount}, nil)
	}()
	name := "é file.txt"
	mountedFile := filepath.Join(mount+`\`, name)
	if err = os.WriteFile(mountedFile, []byte("initial"), 0600); err != nil {
		t.Fatal(err)
	}
	waitFile := func(path, expected string) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			if data, e := os.ReadFile(path); e == nil && string(data) == expected {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("temporary remote content was not confirmed")
	}
	waitFile(filepath.Join(remote, name), "initial")
	// Keep the strict cache reader aligned with the actual pinned engine, not just mocks.
	var stats struct {
		DiskCache *struct {
			UploadsInProgress *int
			UploadsQueued     *int
			ErroredFiles      *int
			Files             *int
			BytesUsed         *int64
			OutOfSpace        *bool
			PathMeta          string
		}
	}
	if err = rpc.Call(ctx, "vfs/stats", map[string]string{"fs": remote}, &stats); err != nil {
		t.Fatal(err)
	}
	d := stats.DiskCache
	if d == nil || d.UploadsInProgress == nil || d.UploadsQueued == nil || d.ErroredFiles == nil || d.Files == nil || d.BytesUsed == nil || d.OutOfSpace == nil {
		t.Fatal("pinned engine cache schema changed")
	}
	metadata, err := os.ReadFile(filepath.Join(d.PathMeta, name))
	if err != nil {
		t.Fatal(err)
	}
	var info struct{ Dirty *bool }
	if err = json.Unmarshal(metadata, &info); err != nil || info.Dirty == nil {
		t.Fatalf("pinned engine Dirty schema changed: %v", err)
	}
	if err = os.WriteFile(mountedFile, []byte("updated"), 0600); err != nil {
		t.Fatal(err)
	}
	waitFile(filepath.Join(remote, name), "updated")
	renamed := filepath.Join(mount+`\`, "renamed.txt")
	if err = os.Rename(mountedFile, renamed); err != nil {
		t.Fatal(err)
	}
	waitFile(filepath.Join(remote, "renamed.txt"), "updated")
	if err = os.Remove(renamed); err != nil {
		t.Fatal(err)
	}
	if err = rpc.Call(ctx, "mount/unmount", map[string]string{"mountPoint": mount}, nil); err != nil {
		t.Fatal(err)
	}
	if err = rpc.Call(ctx, "mount/mount", map[string]any{"fs": remote, "mountPoint": mount, "vfsOpt": map[string]any{"CacheMode": "full", "ReadOnly": true}}, nil); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(mount+`\`, "blocked.txt"), []byte("blocked"), 0600); err == nil {
		t.Fatal("read-only mount accepted writes")
	}
}
