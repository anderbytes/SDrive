package core

import (
	"context"
	"fmt"
	"time"
)

type CapacityView struct {
	Total     *uint64    `json:"total"`
	Used      *uint64    `json:"used"`
	Free      *uint64    `json:"free"`
	CheckedAt *time.Time `json:"checkedAt"`
	State     string     `json:"state"`
}

type capacityInspection struct {
	connection Connection
	rpc        RPC
	at         time.Time
	view       CapacityView
}

func (s *Service) sampleCapacity(ctx context.Context, c Connection, rpc RPC) CapacityView {
	s.mu.Lock()
	i := s.index(c.ID)
	if i < 0 || s.store.Connections[i] != c || s.busy[c.ID] || s.connectionRPC(c) != rpc || !s.mounted[c.ID] {
		s.mu.Unlock()
		return CapacityView{}
	}
	if previous := s.capacities[c.ID]; previous != nil && previous.connection == c && previous.rpc == rpc && time.Since(previous.at) < time.Minute {
		view := previous.view
		s.mu.Unlock()
		return view
	}
	if s.capacities == nil {
		s.capacities = map[string]*capacityInspection{}
	}
	inspection := &capacityInspection{connection: c, rpc: rpc, at: time.Now()}
	s.capacities[c.ID] = inspection
	s.mu.Unlock()
	capacityCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	view := readCapacity(capacityCtx, rpc, c)
	view.CheckedAt = &inspection.at
	s.mu.Lock()
	defer s.mu.Unlock()
	i = s.index(c.ID)
	if s.capacities[c.ID] != inspection || i < 0 || s.store.Connections[i] != c || s.connectionRPC(c) != rpc || !s.mounted[c.ID] || s.busy[c.ID] {
		return CapacityView{}
	}
	inspection.view = view
	return view
}

func readCapacity(ctx context.Context, rpc RPC, c Connection) CapacityView {
	view := CapacityView{State: "unavailable"}
	var info struct{ Features *struct{ About *bool } }
	if callRPC(ctx, rpc, "operations/fsinfo", map[string]string{"fs": remote(c)}, &info) != nil || info.Features == nil || info.Features.About == nil {
		return view
	}
	if !*info.Features.About {
		view.State = "unsupported"
		return view
	}
	var result struct{ Total, Used, Free *uint64 }
	if callRPC(ctx, rpc, "operations/about", map[string]string{"fs": remote(c)}, &result) != nil {
		return view
	}
	if result.Total != nil && ((result.Used != nil && *result.Used > *result.Total) || (result.Free != nil && *result.Free > *result.Total)) {
		return view
	}
	if result.Total == nil && result.Used == nil && result.Free == nil {
		return view
	}
	view.Total, view.Used, view.Free, view.State = result.Total, result.Used, result.Free, "available"
	return view
}

// Refresh merges remote listings with dirty VFS entries; ForgetAll can retain stale directories with pending writes.
func (s *Service) RefreshDirectoryCache(id string) error {
	s.mu.Lock()
	i := s.index(id)
	if i < 0 || !s.mounted[id] || s.busy[id] || s.exiting {
		s.mu.Unlock()
		return fmt.Errorf("Connect the drive and wait for its operations before refreshing files.")
	}
	c := s.store.Connections[i]
	rpc := s.connectionRPC(c)
	s.busy[id] = true
	s.snapshotID++
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var result struct{ Result map[string]string }
	err := callRPC(ctx, rpc, "vfs/refresh", map[string]string{"fs": remote(c), "recursive": "true"}, &result)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.busy[id] = false
	s.snapshotID++
	i = s.index(id)
	if i < 0 || s.store.Connections[i] != c || s.connectionRPC(c) != rpc || !s.mounted[id] || s.exiting {
		return fmt.Errorf("Connection changed. Refresh its files again.")
	}
	if err != nil || len(result.Result) == 0 {
		return fmt.Errorf("Could not refresh directory metadata. Your local files remain preserved.")
	}
	for _, status := range result.Result {
		if status != "OK" {
			return fmt.Errorf("Could not refresh directory metadata. Your local files remain preserved.")
		}
	}
	return nil
}
