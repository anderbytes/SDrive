package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"
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
	t.Cleanup(func() { CloseConnectionEngines(s) })
	started := time.Now()
	view := s.Snapshot().Connections[0]
	if time.Since(started) >= time.Second {
		t.Fatal("capacity query blocked the snapshot")
	}
	if view.Pending == nil || *view.Pending != 0 || view.Cache.Error != "" || !view.Mounted || view.Capacity.State != "unavailable" {
		t.Fatal("capacity failure interfered with mounted upload status")
	}
}

func waitCapacity(t *testing.T, s *Service, id string) CapacityView {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		inspection := s.capacities[id]
		if inspection != nil && !inspection.running {
			view := inspection.view
			s.mu.Unlock()
			return view
		}
		s.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	t.Fatal("capacity query did not complete")
	return CapacityView{}
}

func TestCapacityRetainsLastReadingUntilACompleteRefreshSucceeds(t *testing.T) {
	var stage, calls atomic.Int32
	rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
		if method == "operations/fsinfo" {
			return map[string]any{"Features": map[string]bool{"About": stage.Load() != 5}}, nil
		}
		calls.Add(1)
		switch stage.Load() {
		case 1:
			return nil, context.DeadlineExceeded
		case 2:
			return map[string]any{}, nil
		case 3:
			return map[string]any{"total": 10, "used": 20}, nil
		case 4:
			return map[string]any{"free": 60}, nil
		case 6:
			return map[string]any{"total": 100, "used": 40, "free": 60}, nil
		}
		return map[string]any{"total": 100, "used": 25, "free": 75}, nil
	}}
	s := testService(t, rpc)
	c := testConnection()
	s.store.Connections, s.mounted[c.ID] = []Connection{c}, true
	t.Cleanup(func() { CloseConnectionEngines(s) })
	s.sampleCapacity(c, rpc)
	first := waitCapacity(t, s, c.ID)
	if first.State != "available" || first.CheckedAt == nil {
		t.Fatalf("initial capacity: %+v", first)
	}
	// Cosmetic changes must not discard the mounted destination's reading.
	c.CardStyle, c.Name = "blue", "Renamed drive"
	s.mu.Lock()
	s.store.Connections[0] = c
	s.mu.Unlock()
	if got := s.sampleCapacity(c, rpc); got.CheckedAt != first.CheckedAt || calls.Load() != 1 {
		t.Fatal("fresh capacity was queried again or lost after appearance change")
	}
	for n := int32(1); n <= 6; n++ {
		stage.Store(n)
		s.mu.Lock()
		s.capacities[c.ID].at = time.Now().Add(-5 * time.Minute)
		s.mu.Unlock()
		if got := s.sampleCapacity(c, rpc); got.CheckedAt != first.CheckedAt {
			t.Fatal("refresh cleared the previous reading")
		}
		got := waitCapacity(t, s, c.ID)
		if n < 6 {
			if got.State != "stale" || got.CheckedAt != first.CheckedAt || *got.Total != 100 || *got.Used != 25 || *got.Free != 75 {
				t.Fatalf("stage %d did not preserve the whole reading: %+v", n, got)
			}
		} else if got.State != "available" || !got.CheckedAt.After(*first.CheckedAt) || *got.Used != 40 {
			t.Fatalf("successful refresh not accepted: %+v", got)
		}
	}
}

func TestCapacityQueryIsSharedAndRejectsObsoleteMounts(t *testing.T) {
	for _, change := range []string{"disconnect", "destination", "engine"} {
		t.Run(change, func(t *testing.T) {
			started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
			rpc := &fakeRPC{handle: func(ctx context.Context, method string, _ any) (any, error) {
				if method == "operations/fsinfo" {
					return map[string]any{"Features": map[string]bool{"About": true}}, nil
				}
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) < 8*time.Second {
					t.Error("capacity inherited the short snapshot deadline")
				}
				close(started)
				<-release
				defer close(finished)
				return map[string]any{"total": 100, "used": 25}, nil
			}}
			s := testService(t, rpc)
			c := testConnection()
			s.store.Connections, s.mounted[c.ID] = []Connection{c}, true
			s.sampleCapacity(c, rpc)
			<-started
			for i := 0; i < 10; i++ {
				if got := s.sampleCapacity(c, rpc); got.State != "unavailable" {
					t.Fatal("in-flight query exposed an empty state")
				}
			}
			s.mu.Lock()
			inspection := s.capacities[c.ID]
			switch change {
			case "disconnect":
				s.clearCapacity(c.ID)
				// A reconnect using the same RPC must still reject the old result.
				s.capacities[c.ID] = &capacityInspection{connection: c, rpc: rpc, view: CapacityView{State: "unavailable"}}
			case "destination":
				s.store.Connections[0].Path = "another-folder"
			case "engine":
				s.rpc = &fakeRPC{}
			}
			s.mu.Unlock()
			close(release)
			<-finished
			deadline := time.Now().Add(2 * time.Second)
			for {
				s.mu.Lock()
				running, checked := inspection.running, inspection.view.CheckedAt
				s.mu.Unlock()
				if !running {
					if checked != nil {
						t.Error("obsolete query published a capacity reading")
					}
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("obsolete query did not finish")
				}
				time.Sleep(time.Millisecond)
			}
			CloseConnectionEngines(s)
		})
	}
}
