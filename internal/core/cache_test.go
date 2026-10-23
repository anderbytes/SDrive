package core

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func cacheFixture(t *testing.T, s *Service, c Connection, metadata string, data *string) string {
	t.Helper()
	meta := filepath.Join(s.dataPath(), "vfsMeta", "sd_"+c.Remote, "nested", "file")
	if err := os.MkdirAll(filepath.Dir(meta), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(meta, []byte(metadata), 0600); err != nil {
		t.Fatal(err)
	}
	if data != nil {
		path := filepath.Join(s.dataPath(), "vfs", "sd_"+c.Remote, "nested", "file")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(*data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return meta
}

func TestCacheMetadataRequiresExplicitBoolean(t *testing.T) {
	for _, metadata := range []string{`{}`, `null`, `{"Dirty":null}`, `{"Dirty":0}`, `{"Dirty":"false"}`, `[]`, `broken`, `{"Dirty":false} {}`} {
		t.Run(metadata, func(t *testing.T) {
			s := testService(t, &fakeRPC{})
			c := testConnection()
			path := cacheFixture(t, s, c, metadata, nil)
			if _, err := s.cachedPending(context.Background(), c); err == nil {
				t.Fatal("invalid metadata declared clean")
			}
			s.store.Connections = []Connection{c}
			s.mounted[c.ID] = true
			if s.Unmount(c.ID) == nil || !s.mounted[c.ID] || !s.store.Connections[0].Recovery {
				t.Fatal("unsafe unmount")
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != metadata {
				t.Fatal("metadata modified")
			}
		})
	}
}

func TestCriticalCacheStatsValidation(t *testing.T) {
	for _, field := range []string{"uploadsInProgress", "uploadsQueued", "erroredFiles", "files", "bytesUsed", "outOfSpace"} {
		for _, value := range []any{nil, "wrong type", -1, 1.5} {
			t.Run(field+"/"+fmtValue(value), func(t *testing.T) {
				stats := cleanCacheStats()
				stats[field] = value
				rpc := &fakeRPC{handle: func(context.Context, string, any) (any, error) { return map[string]any{"diskCache": stats}, nil }}
				if _, _, err := readCacheStats(context.Background(), rpc, testConnection()); err == nil {
					t.Fatal("invalid statistics accepted")
				}
				delete(stats, field)
				if _, _, err := readCacheStats(context.Background(), rpc, testConnection()); err == nil {
					t.Fatal("missing field accepted")
				}
			})
		}
	}
	for _, stats := range []any{nil, map[string]any{}, map[string]any{"uploadsInProgress": math.MaxInt, "uploadsQueued": 1, "erroredFiles": 0, "files": 0, "bytesUsed": 0, "outOfSpace": false}} {
		rpc := &fakeRPC{handle: func(context.Context, string, any) (any, error) { return map[string]any{"diskCache": stats}, nil }}
		if _, _, err := readCacheStats(context.Background(), rpc, testConnection()); err == nil {
			t.Fatal("incomplete/overflowing statistics accepted")
		}
	}
}

func fmtValue(value any) string { b, _ := json.Marshal(value); return string(b) }

func TestCacheInspectionMetricsAndUnpairedData(t *testing.T) {
	s := testService(t, &fakeRPC{})
	c := testConnection()
	data := "pending data"
	path := cacheFixture(t, s, c, `{"Dirty":true}`, &data)
	result := s.inspectCache(context.Background(), c)
	if result.err != nil || result.pending != 1 || result.PendingBytes == nil || *result.PendingBytes != int64(len(data)) || result.UsedBytes == nil {
		t.Fatalf("invalid metrics: %+v, %v", result, result.err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.cachedPending(context.Background(), c); err == nil {
		t.Fatal("unpaired data declared clean")
	}
	if _, err := os.Stat(filepath.Join(s.dataPath(), "vfs", "sd_"+c.Remote, "nested", "file")); err != nil {
		t.Fatal("unpaired data lost")
	}
}

func TestCacheInspectionBoundsAndCancellation(t *testing.T) {
	s := testService(t, &fakeRPC{})
	c := testConnection()
	path := cacheFixture(t, s, c, `{"Dirty":false}`, nil)
	f, err := os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(metadataLimit + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := s.cachedPending(context.Background(), c); err == nil {
		t.Fatal("oversized metadata accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.cachedPending(ctx, c); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"Dirty":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "second"), []byte(`{"Dirty":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	visited := 0
	err = inspectFiles(ctx, filepath.Dir(path), func(string, os.FileInfo) error { visited++; cancel(); return nil })
	if !errors.Is(err, context.Canceled) || visited != 1 {
		t.Fatalf("walk did not stop: %d, %v", visited, err)
	}
}

func TestCacheInspectionRejectsLinksAndUnexpectedRoots(t *testing.T) {
	for _, tree := range []string{"cache", filepath.Join("cache", "vfs"), filepath.Join("cache", "vfsMeta")} {
		t.Run(tree, func(t *testing.T) {
			s := testService(t, &fakeRPC{})
			path := filepath.Join(s.dir, tree)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("preserve"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := s.cachedPending(context.Background(), testConnection()); err == nil {
				t.Fatal("unexpected root accepted")
			}
		})
	}
	t.Run("linked metadata", func(t *testing.T) {
		s := testService(t, &fakeRPC{})
		c := testConnection()
		path := cacheFixture(t, s, c, `{"Dirty":false}`, nil)
		if err := os.Symlink(path, filepath.Join(filepath.Dir(path), "link")); err != nil {
			t.Skipf("symlink creation unavailable: %v", err)
		}
		if _, err := s.cachedPending(context.Background(), c); err == nil {
			t.Fatal("metadata link accepted")
		}
	})
}

func TestFailedSamplesAreCachedButSafetyChecksAreFresh(t *testing.T) {
	s := testService(t, &fakeRPC{})
	c := testConnection()
	s.store.Connections = []Connection{c}
	path := cacheFixture(t, s, c, `{}`, nil)
	first := s.sampleCache(context.Background(), c)
	if first.err == nil {
		t.Fatal("bad metadata accepted")
	}
	if err := os.WriteFile(path, []byte(`{"Dirty":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := s.sampleCache(context.Background(), c); got.err == nil || got.CheckedAt != first.CheckedAt {
		t.Fatal("failed sample not retained")
	}
	if _, err := s.cachedPending(context.Background(), c); err != nil {
		t.Fatal("safety check reused failed sample")
	}
	delete(s.inspections, c.ID)
	if got := s.sampleCache(context.Background(), c); got.err != nil {
		t.Fatal("invalidation did not allow retry")
	}
}

func TestSnapshotUnknownAndSampleExpiry(t *testing.T) {
	s := testService(t, &fakeRPC{})
	c := testConnection()
	s.store.Connections = []Connection{c}
	s.mounted[c.ID] = true
	path := cacheFixture(t, s, c, `{"Dirty":false}`, nil)
	first := s.Snapshot().Connections[0]
	if first.Pending == nil || *first.Pending != 0 || first.Cache.CheckedAt == nil {
		t.Fatalf("first sample: %+v", first)
	}
	if err := os.WriteFile(path, []byte(`{"Dirty":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := s.Snapshot().Connections[0]; got.Pending == nil || *got.Pending != 0 || !got.Cache.CheckedAt.Equal(*first.Cache.CheckedAt) {
		t.Fatal("sample not reused")
	}
	// Authoritative checks never use the clean display sample.
	if count, err := s.pending(context.Background(), c); err != nil || count != 1 {
		t.Fatalf("stale safety check: %d, %v", count, err)
	}
	s.inspections[c.ID].at = time.Now().Add(-31 * time.Second)
	if got := s.Snapshot().Connections[0]; got.Pending == nil || *got.Pending != 1 {
		t.Fatal("expired sample not refreshed")
	}
	s.rpc = &fakeRPC{handle: func(context.Context, string, any) (any, error) { return nil, errors.New("password=secret") }}
	unknown := s.Snapshot().Connections[0]
	if unknown.Pending != nil || unknown.Cache.Error == "" || strings.Contains(unknown.Cache.Error, "secret") {
		t.Fatal("failed status not safely represented")
	}
	b, _ := json.Marshal(unknown)
	if !strings.Contains(string(b), `"pending":null`) {
		t.Fatal("unknown lost across bridge")
	}
}

type blockedInspection struct {
	context.Context
	calls            atomic.Int32
	entered, release chan struct{}
}

func (c *blockedInspection) Err() error {
	if c.calls.Add(1) == 2 {
		close(c.entered)
		<-c.release
	}
	return c.Context.Err()
}

func TestConcurrentInspectionAndInvalidation(t *testing.T) {
	s := testService(t, &fakeRPC{})
	c := testConnection()
	s.store.Connections = []Connection{c}
	ctx := &blockedInspection{Context: context.Background(), entered: make(chan struct{}), release: make(chan struct{})}
	done := make(chan localCache, 1)
	go func() { done <- s.sampleCache(ctx, c) }()
	<-ctx.entered
	if got := s.sampleCache(context.Background(), c); got.err == nil {
		t.Fatal("concurrent scan reused incomplete result")
	}
	s.mu.Lock()
	delete(s.inspections, c.ID)
	s.mu.Unlock()
	close(ctx.release)
	if got := <-done; got.err == nil {
		t.Fatal("invalidated result published")
	}
	if got := s.sampleCache(context.Background(), c); got.err != nil {
		t.Fatal(got.err)
	}
}

func TestExitRejectsUnknownDisconnectedCache(t *testing.T) {
	s := testService(t, &fakeRPC{})
	c := testConnection()
	c.Recovery = false
	s.store.Connections = []Connection{c}
	cacheFixture(t, s, c, `{}`, nil)
	quit := false
	s.quit = func() { quit = true }
	if s.Exit() == nil || quit || s.exiting {
		t.Fatal("unknown cache allowed exit or prevented retry")
	}
}
