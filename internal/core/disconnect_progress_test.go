package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConcurrentDisconnectProgressIsOrdered(t *testing.T) {
	started, release := make(chan struct{}, 2), make(chan struct{})
	s := testService(t, &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
		if method == "mount/unmount" {
			started <- struct{}{}
			<-release
		}
		return map[string]any{"diskCache": cleanCacheStats()}, nil
	}})
	one, two := testConnection(), testConnection()
	two.ID, two.Remote, two.Letter = strings.Repeat("3", 32), strings.Repeat("4", 32), "T"
	s.store.Connections = []Connection{one, two}
	s.mounted[one.ID], s.mounted[two.ID] = true, true
	var progress []DisconnectProgress
	SetDisconnectProgress(s, func(event DisconnectProgress) { progress = append(progress, event) })
	var workers sync.WaitGroup
	for _, c := range s.store.Connections {
		workers.Go(func() {
			if err := s.Unmount(c.ID); err != nil {
				t.Error(err)
			}
		})
	}
	receive(t, started)
	receive(t, started)
	close(release)
	workers.Wait()
	stages := map[string][]string{}
	for index, event := range progress {
		if event.Sequence != uint64(index+1) {
			t.Fatal("concurrent progress was reordered")
		}
		stages[event.ID] = append(stages[event.ID], event.Stage)
	}
	for _, id := range []string{one.ID, two.ID} {
		if got := stages[id]; len(got) != 3 || got[0] != "checkingUploads" || got[1] != "unmounting" || got[2] != "succeeded" {
			t.Fatalf("drive %s lost lifecycle events: %v", id, got)
		}
	}
}

func TestDisconnectNotifiesFailureThroughEveryEntryPoint(t *testing.T) {
	for _, trigger := range []string{"manual", "exit", "inactivity"} {
		for _, failure := range []string{"none", "vfs/stats", "mount/unmount", "save settings"} {
			t.Run(trigger+"/"+failure, func(t *testing.T) {
				s := testService(t, &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
					if method == failure {
						return nil, context.DeadlineExceeded
					}
					return map[string]any{"diskCache": cleanCacheStats()}, nil
				}})
				c := testConnection()
				s.store.Connections = []Connection{c}
				s.mounted[c.ID] = true
				if trigger == "inactivity" {
					enableTestAccess(t, s, AccessPolicy{IdleMinutes: 1, DisconnectIdle: true})
					s.access.idle = func() (time.Duration, error) { return 2 * time.Minute, nil }
				}
				if failure == "save settings" {
					if err := os.Remove(filepath.Join(s.dir, "settings.json")); err != nil && !os.IsNotExist(err) {
						t.Fatal(err)
					}
					if err := os.Mkdir(filepath.Join(s.dir, "settings.json"), 0700); err != nil {
						t.Fatal(err)
					}
				}
				var progress []DisconnectProgress
				SetDisconnectProgress(s, func(event DisconnectProgress) {
					progress = append(progress, event)
					if event.Stage == "succeeded" && (s.busy[c.ID] || s.mounted[c.ID] || s.store.Connections[0].Recovery) {
						t.Error("success published before disconnect cleanup")
					}
				})
				switch trigger {
				case "manual":
					_ = s.Unmount(c.ID)
				case "exit":
					_ = s.Exit()
				case "inactivity":
					CheckInactivity(s)
				}
				want := "succeeded"
				if failure != "none" {
					want = "failed"
				}
				if len(progress) < 2 || progress[0].Stage != "checkingUploads" || progress[len(progress)-1].Stage != want {
					t.Fatalf("unexpected progress: %+v", progress)
				}
				for index, event := range progress {
					if event.Sequence != uint64(index+1) || event.ID != c.ID {
						t.Fatalf("out-of-order progress: %+v", progress)
					}
				}
				before := len(progress)
				s.busy[c.ID] = true
				_ = s.unmount(c.ID)
				if len(progress) != before {
					t.Fatal("rejected duplicate replaced the active operation")
				}
			})
		}
	}
}
