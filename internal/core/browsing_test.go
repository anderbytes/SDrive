package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestCapacityPreservesUnknownFieldsAndRejectsInvalidResponses(t *testing.T) {
	for _, tc := range []struct {
		name     string
		about    bool
		response any
		fail     bool
		state    string
	}{
		{"partial", true, map[string]any{"free": 0}, false, "available"},
		{"complete", true, map[string]any{"total": 100, "used": 25, "free": 75}, false, "available"},
		{"unsupported", false, nil, false, "unsupported"},
		{"empty", true, map[string]any{}, false, "unavailable"},
		{"failed", true, nil, true, "unavailable"},
		{"negative", true, map[string]any{"free": -1}, false, "unavailable"},
		{"over total", true, map[string]any{"total": 10, "free": 11}, false, "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
				if method == "operations/fsinfo" {
					return map[string]any{"Features": map[string]bool{"About": tc.about}}, nil
				}
				if tc.fail {
					return nil, fmt.Errorf("synthetic failure")
				}
				return tc.response, nil
			}}
			got := readCapacity(context.Background(), rpc, testConnection())
			if got.State != tc.state {
				t.Fatalf("unexpected state: %+v", got)
			}
			if tc.name == "partial" && (got.Total != nil || got.Used != nil || got.Free == nil || *got.Free != 0) {
				t.Fatal("missing capacity invented")
			}
			if tc.state != "available" && (got.Total != nil || got.Used != nil || got.Free != nil) {
				t.Fatal("unknown capacity invented")
			}
		})
	}
}

func TestDirectoryRefreshKeepsPendingFilesAndReservesConnection(t *testing.T) {
	started, finish := make(chan struct{}), make(chan struct{})
	rpc := &fakeRPC{handle: func(_ context.Context, method string, input any) (any, error) {
		if method != "vfs/refresh" {
			return nil, fmt.Errorf("unexpected call")
		}
		b, _ := json.Marshal(input)
		if string(b) != `{"fs":"sd_22222222222222222222222222222222:","recursive":"true"}` {
			return nil, fmt.Errorf("wrong refresh destination")
		}
		close(started)
		<-finish
		return map[string]any{"result": map[string]string{"": "OK"}}, nil
	}}
	s := testService(t, rpc)
	c := testConnection()
	s.store.Connections = []Connection{c}
	s.mounted[c.ID] = true
	content := "pending"
	metadata := cacheFixture(t, s, c, `{"Dirty":true}`, &content)
	done := make(chan error, 1)
	go func() { done <- s.RefreshDirectoryCache(c.ID) }()
	<-started
	if err := s.Unmount(c.ID); err == nil {
		t.Fatal("refresh did not reserve the connection")
	}
	close(finish)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(metadata); err != nil || string(b) != `{"Dirty":true}` {
		t.Fatal("refresh changed pending metadata")
	}
	if count, err := s.cachedPending(context.Background(), c); err != nil || count != 1 {
		t.Fatal("refresh lost pending data")
	}
}

func TestCapacityFailureDoesNotInvalidatePendingSnapshot(t *testing.T) {
	c := testConnection()
	rpc := &fakeRPC{handle: func(ctx context.Context, method string, _ any) (any, error) {
		switch method {
		case "mount/listmounts":
			return map[string]any{"mountPoints": []engineMount{{Fs: remote(c), MountPoint: "S:"}}}, nil
		case "vfs/stats":
			return map[string]any{"diskCache": cleanCacheStats()}, nil
		case "operations/fsinfo":
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("unexpected call")
	}}
	s := testService(t, rpc)
	s.store.Connections = []Connection{c}
	s.mounted[c.ID] = true
	s.driveUsed = func(string) bool { return true }
	view := s.Snapshot().Connections[0]
	if view.Pending == nil || *view.Pending != 0 || view.Cache.Error != "" || !view.Mounted || view.Capacity.State != "unavailable" {
		t.Fatal("capacity failure interfered with mounted upload status")
	}
}
