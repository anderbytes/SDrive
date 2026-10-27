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
	running    bool
	cancel     context.CancelFunc
}

func sameCapacityDestination(a, b Connection) bool {
	return a.ID == b.ID && a.Remote == b.Remote && a.Provider == b.Provider && a.Path == b.Path && a.TargetHash == b.TargetHash
}

// Caller holds s.mu. Deleting the entry also rejects a late result from this mount.
func (s *Service) clearCapacity(id string) {
	if inspection := s.capacities[id]; inspection != nil && inspection.cancel != nil {
		inspection.cancel()
	}
	delete(s.capacities, id)
}

func (s *Service) sampleCapacity(c Connection, rpc RPC) CapacityView {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.index(c.ID)
	if i < 0 || !sameCapacityDestination(s.store.Connections[i], c) || s.connectionRPC(c) != rpc || !s.mounted[c.ID] || rpc == nil || !rpc.Alive() {
		return CapacityView{}
	}
	inspection := s.capacities[c.ID]
	if inspection != nil && (!sameCapacityDestination(inspection.connection, c) || inspection.rpc != rpc) {
		s.clearCapacity(c.ID)
		inspection = nil
	}
	if inspection == nil {
		if s.capacities == nil {
			s.capacities = map[string]*capacityInspection{}
		}
		inspection = &capacityInspection{connection: c, rpc: rpc, view: CapacityView{State: "unavailable"}}
		s.capacities[c.ID] = inspection
	}
	if inspection.running || time.Since(inspection.at) < 5*time.Minute || s.busy[c.ID] || s.exiting {
		return inspection.view
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	inspection.running, inspection.cancel = true, cancel
	go func() {
		defer cancel()
		view := readCapacity(ctx, rpc, c)
		now := time.Now()
		s.mu.Lock()
		defer s.mu.Unlock()
		inspection.running, inspection.cancel = false, nil
		i := s.index(c.ID)
		if s.capacities[c.ID] != inspection || i < 0 || !sameCapacityDestination(s.store.Connections[i], c) || s.connectionRPC(c) != rpc || !rpc.Alive() || !s.mounted[c.ID] || s.exiting {
			return
		}
		inspection.at = now
		previous := inspection.view
		losesFields := previous.Total != nil && view.Total == nil || previous.Used != nil && view.Used == nil || previous.Free != nil && view.Free == nil
		if previous.CheckedAt != nil && (view.State != "available" || losesFields) {
			inspection.view.State = "stale"
			return
		}
		if view.State == "available" {
			view.CheckedAt = &now
		}
		inspection.view = view
	}()
	return inspection.view
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
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return err
	}
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
