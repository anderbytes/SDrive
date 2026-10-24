package core

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sdrive/internal/platform"
)

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("operation blocked")
		var zero T
		return zero
	}
}

func TestSlowFolderCallsAllowCancellationAndRejectLateResults(t *testing.T) {
	for _, confirm := range []bool{false, true} {
		t.Run(fmt.Sprintf("confirm=%v", confirm), func(t *testing.T) {
			started, finish := make(chan struct{}), make(chan struct{})
			rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
				if method == "operations/list" {
					close(started)
					<-finish
					return map[string]any{"list": []any{}}, nil
				}
				return ConfigResult{}, nil
			}}
			s := testService(t, rpc)
			input := ConfigInput{Connection: Connection{Name: "Draft", Provider: "webdav", Letter: "S"}, Values: map[string]string{"url": "https://example.test/dav"}, Guided: true}
			id, err := s.BeginConfig(input)
			if err != nil {
				t.Fatal(err)
			}
			waitSession(t, s, id)
			result := make(chan error, 1)
			go func() {
				if confirm {
					_, err := s.ConfirmConfig(id, "", MountTarget{Letter: "S"})
					result <- err
				} else {
					_, err := s.ListConfigFolders(id, "")
					result <- err
				}
			}()
			receive(t, started)
			status := make(chan error, 1)
			go func() { _, err := s.ConfigStatus(id); status <- err }()
			if err := receive(t, status); err != nil {
				t.Fatal(err)
			}
			ended := make(chan error, 1)
			go func() { ended <- s.EndConfig(id) }()
			if err := receive(t, ended); err != nil {
				t.Fatal(err)
			}
			next, err := s.BeginConfig(input)
			if err != nil {
				t.Fatal(err)
			}
			waitSession(t, s, next)
			close(finish)
			if err := receive(t, result); err == nil {
				t.Fatal("obsolete folder response accepted")
			}
			if len(s.store.Connections) != 0 {
				t.Fatal("cancelled destination committed")
			}
			if err := s.EndConfig(next); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCancellationWaitsForLateConfigurationBeforeDeleting(t *testing.T) {
	started, cancelled, finish, deleted := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	rpc := &fakeRPC{handle: func(ctx context.Context, method string, _ any) (any, error) {
		switch method {
		case "config/create":
			close(started)
			<-ctx.Done()
			close(cancelled)
			<-finish
		case "config/delete":
			close(deleted)
		}
		return ConfigResult{}, nil
	}}
	s := testService(t, rpc)
	id, err := s.BeginConfig(ConfigInput{Connection: Connection{Name: "Draft", Provider: "webdav", Letter: "S"}, Values: map[string]string{"url": "https://example.test/dav"}})
	if err != nil {
		t.Fatal(err)
	}
	receive(t, started)
	ended := make(chan error, 1)
	go func() { ended <- s.EndConfig(id) }()
	receive(t, cancelled)
	select {
	case <-deleted:
		t.Fatal("credentials deleted before configuration finished")
	default:
	}
	if _, err := s.BeginConfig(ConfigInput{}); err == nil {
		t.Fatal("replacement setup admitted during cancellation")
	}
	close(finish)
	if err := receive(t, ended); err != nil {
		t.Fatal(err)
	}
	receive(t, deleted)
	if len(s.store.Connections) != 0 || s.session != nil {
		t.Fatal("late configuration committed")
	}
}

type switchableRPC struct {
	fakeRPC
	stopped atomic.Bool
}

func (r *switchableRPC) Alive() bool { return !r.stopped.Load() }

func TestSnapshotKeepsItsEngineDuringRestart(t *testing.T) {
	started, finish := make(chan struct{}), make(chan struct{})
	c := testConnection()
	old := &switchableRPC{}
	old.handle = func(_ context.Context, method string, _ any) (any, error) {
		if method == "mount/listmounts" {
			close(started)
			<-finish
			return map[string]any{"mountPoints": []engineMount{{Fs: remote(c), MountPoint: c.Letter + ":"}}}, nil
		}
		return map[string]any{"diskCache": cleanCacheStats()}, nil
	}
	s := testService(t, old)
	s.store.Connections = []Connection{c}
	s.mounted[c.ID] = true
	if err := saveStore(s.dir, s.store); err != nil {
		t.Fatal(err)
	}
	result := make(chan View, 1)
	go func() { result <- s.Snapshot() }()
	receive(t, started)
	old.stopped.Store(true)
	next := &fakeRPC{}
	s.restart = func() (RPC, error) { return next, nil }
	if err := s.RestartEngine(); err != nil {
		t.Fatal(err)
	}
	close(finish)
	view := receive(t, result)
	if view.Connections[0].Pending != nil || s.mounted[c.ID] {
		t.Fatal("old engine status applied to replacement")
	}
	next.mu.Lock()
	defer next.mu.Unlock()
	for _, method := range next.calls {
		if method != "config/listremotes" {
			t.Fatalf("snapshot switched engines: %s", method)
		}
	}
}

func TestMountReservesConnectionAcrossCalls(t *testing.T) {
	for _, blocked := range []string{"config/get", "mount/mount"} {
		t.Run(blocked, func(t *testing.T) {
			started, finish := make(chan struct{}), make(chan struct{})
			rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
				if method == blocked {
					close(started)
					<-finish
				}
				if method == "config/get" {
					return map[string]string{"type": "ftp", "host": "example.test"}, nil
				}
				if method == "mount/listmounts" {
					return map[string]any{"mountPoints": []engineMount{}}, nil
				}
				return map[string]any{}, nil
			}}
			s := testService(t, rpc)
			s.winFsp = func() bool { return true }
			c := testConnection()
			c.Recovery = false
			for letter := 'D'; letter <= 'Z'; letter++ {
				if !platform.DriveUsed(string(letter)) {
					c.Letter = string(letter)
					break
				}
			}
			other := c
			other.ID = strings.Repeat("3", 32)
			other.Remote = strings.Repeat("4", 32)
			other.Letter = "Z"
			if c.Letter == "Z" {
				other.Letter = "Y"
			}
			s.store.Connections = []Connection{other, c}
			if err := saveStore(s.dir, s.store); err != nil {
				t.Fatal(err)
			}
			var quit atomic.Bool
			s.quit = func() { quit.Store(true) }
			mounted := make(chan error, 1)
			go func() { mounted <- s.Mount(c.ID) }()
			receive(t, started)
			if err := s.Mount(c.ID); err == nil {
				t.Fatal("duplicate mount admitted")
			}
			if err := s.Remove(c.ID); err == nil {
				t.Fatal("active connection removed")
			}
			if err := s.Exit(); err == nil || quit.Load() {
				t.Fatal("exited during mount")
			}
			if err := s.Remove(other.ID); err != nil {
				t.Fatal(err)
			}
			snapshot := make(chan View, 1)
			go func() { snapshot <- s.Snapshot() }()
			if view := receive(t, snapshot); !view.Connections[0].Busy {
				t.Fatal("mount reservation lost")
			}
			close(finish)
			if err := receive(t, mounted); err != nil {
				t.Fatal(err)
			}
			if len(s.store.Connections) != 1 || s.store.Connections[0].ID != c.ID || !s.store.Connections[0].Recovery || !s.mounted[c.ID] {
				t.Fatal("mount applied to wrong connection after removal")
			}
		})
	}
}

func TestSnapshotReconcilesMountsWithoutClearingRecovery(t *testing.T) {
	for _, state := range []string{"missing", "matching", "wrong destination", "unknown"} {
		t.Run(state, func(t *testing.T) {
			c := testConnection()
			rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
				if method == "mount/listmounts" {
					mounts := []engineMount{}
					if state == "unknown" {
						return nil, nil
					}
					if state == "matching" || state == "wrong destination" {
						fs := remote(c)
						if state == "wrong destination" {
							fs = "another:"
						}
						mounts = append(mounts, engineMount{Fs: fs, MountPoint: "s:\\"})
					}
					return map[string]any{"mountPoints": mounts}, nil
				}
				return map[string]any{"diskCache": cleanCacheStats()}, nil
			}}
			s := testService(t, rpc)
			s.store.Connections = []Connection{c}
			s.mounted[c.ID] = true
			view := s.Snapshot().Connections[0]
			if !s.store.Connections[0].Recovery {
				t.Fatal("reconciliation cleared recovery")
			}
			if state == "unknown" {
				if view.Pending != nil || view.Cache.Error == "" {
					t.Fatal("unknown mounts treated as safe")
				}
				return
			}
			if view.Mounted != (state == "matching") || s.mounted[c.ID] != view.Mounted {
				t.Fatalf("mount state: %+v", view)
			}
			if state != "matching" && view.Error == "" {
				t.Fatal("mount discrepancy hidden")
			}
		})
	}
}

func TestCancelTimeoutPreservesSetupForRetry(t *testing.T) {
	started, finish := make(chan struct{}), make(chan struct{})
	var deletes atomic.Int32
	rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
		if method == "config/create" {
			close(started)
			<-finish
		}
		if method == "config/delete" {
			deletes.Add(1)
		}
		return ConfigResult{}, nil
	}}
	s := testService(t, rpc)
	id, err := s.BeginConfig(ConfigInput{Connection: Connection{Name: "Draft", Provider: "webdav", Letter: "S"}, Values: map[string]string{"url": "https://example.test/dav"}})
	if err != nil {
		t.Fatal(err)
	}
	receive(t, started)
	if err := s.EndConfig(id); err == nil {
		t.Fatal("unconfirmed cancellation succeeded")
	}
	status, err := s.ConfigStatus(id)
	if err != nil || !status.closing || deletes.Load() != 0 {
		t.Fatal("unknown work was released or cleaned up")
	}
	close(finish)
	receive(t, status.done)
	if err := s.EndConfig(id); err != nil {
		t.Fatal(err)
	}
	if deletes.Load() != 1 || s.session != nil || len(s.store.Connections) != 0 {
		t.Fatal("cancel retry did not clean up exactly once")
	}
}

func TestOlderSnapshotCannotRestoreAnUnexpectedlyRemovedMount(t *testing.T) {
	started, finish := make(chan struct{}), make(chan struct{})
	var requests atomic.Int32
	c := testConnection()
	rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
		if method == "mount/listmounts" {
			if requests.Add(1) == 1 {
				close(started)
				<-finish
				return map[string]any{"mountPoints": []engineMount{{Fs: remote(c), MountPoint: c.Letter + ":"}}}, nil
			}
			return map[string]any{"mountPoints": []engineMount{}}, nil
		}
		return map[string]any{"diskCache": cleanCacheStats()}, nil
	}}
	s := testService(t, rpc)
	s.store.Connections = []Connection{c}
	s.mounted[c.ID] = true
	older := make(chan View, 1)
	go func() { older <- s.Snapshot() }()
	receive(t, started)
	if s.Snapshot().Connections[0].Mounted {
		t.Fatal("new snapshot retained missing mount")
	}
	close(finish)
	receive(t, older)
	if s.mounted[c.ID] || !s.store.Connections[0].Recovery {
		t.Fatal("obsolete snapshot restored the mount or cleared recovery")
	}
}
