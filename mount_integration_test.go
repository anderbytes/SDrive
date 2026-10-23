package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"sdrive/internal/core"
)

// Keep strict cache readers aligned with the pinned engine, not just RPC doubles.
func assertPinnedCacheSchema(t *testing.T, rpc core.RPC, c core.Connection, name string) {
	t.Helper()
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
	integrationCall(t, rpc, "vfs/stats", map[string]string{"fs": "sd_" + c.Remote + ":" + c.Path}, &stats)
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
		t.Fatal("pinned engine Dirty schema changed")
	}
}
