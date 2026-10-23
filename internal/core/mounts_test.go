package core

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPendingCombinesLiveAndDurableUploads(t *testing.T) {
	for _, tc := range []struct {
		name      string
		stats     any
		metadata  string
		want      int
		wantError bool
	}{
		{"clean", map[string]any{}, `{"Dirty":false}`, 0, false},
		{"live uploads", map[string]any{"uploadsInProgress": 2, "uploadsQueued": 3, "erroredFiles": 1}, `{"Dirty":true}`, 6, false},
		{"durable upload", map[string]any{}, `{"Dirty":true}`, 1, false},
		{"corrupt metadata", map[string]any{}, `broken`, 0, true},
		{"missing stats", nil, `{"Dirty":false}`, 0, true},
		{"disk full", map[string]any{"outOfSpace": true}, `{"Dirty":false}`, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testService(t, &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
				return map[string]any{"diskCache": tc.stats}, nil
			}})
			c := testConnection()
			root := filepath.Join(s.dataPath(), "vfsMeta", "sd_"+c.Remote, "nested")
			if err := os.MkdirAll(root, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "file"), []byte(tc.metadata), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := s.pending(context.Background(), c)
			if (err != nil) != tc.wantError || got != tc.want {
				t.Fatalf("pending = %d, %v; want %d, error %v", got, err, tc.want, tc.wantError)
			}
		})
	}
}

func TestUnmountFailurePreservesRecoveryAndCanRetry(t *testing.T) {
	for _, failure := range []string{"vfs/stats", "mount/unmount", "save settings"} {
		t.Run(failure, func(t *testing.T) {
			fail := true
			rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
				if fail && method == failure {
					return nil, context.DeadlineExceeded
				}
				return map[string]any{"diskCache": map[string]any{}}, nil
			}}
			s := testService(t, rpc)
			c := testConnection()
			s.store.Connections = []Connection{c}
			s.mounted[c.ID] = true
			if failure == "save settings" {
				if err := os.Mkdir(filepath.Join(s.dir, "settings.json"), 0700); err != nil {
					t.Fatal(err)
				}
			} else if err := saveStore(s.dir, s.store); err != nil {
				t.Fatal(err)
			}
			if err := s.Unmount(c.ID); err == nil {
				t.Fatal("unmount failure ignored")
			}
			if !s.store.Connections[0].Recovery || s.busy[c.ID] || s.mounted[c.ID] != (failure != "save settings") {
				t.Fatal("incorrect state after failed unmount")
			}
			if failure != "save settings" {
				persisted, err := loadStore(s.dir)
				if err != nil || !persisted.Connections[0].Recovery {
					t.Fatalf("durable recovery state lost: %v", err)
				}
				fail = false
				if err := s.Unmount(c.ID); err != nil {
					t.Fatal(err)
				}
				persisted, err = loadStore(s.dir)
				if err != nil || persisted.Connections[0].Recovery || s.mounted[c.ID] || s.errors[c.ID] != "" {
					t.Fatalf("retry did not persist clean state: %v", err)
				}
			}
		})
	}
}

func TestCorruptCacheBlocksUnmountBeforeRPC(t *testing.T) {
	rpc := &fakeRPC{}
	s := testService(t, rpc)
	c := testConnection()
	s.store.Connections = []Connection{c}
	s.mounted[c.ID] = true
	root := filepath.Join(s.dataPath(), "vfsMeta", "sd_"+c.Remote)
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	rpc.calls = nil
	if s.Unmount(c.ID) == nil || !s.mounted[c.ID] || !s.store.Connections[0].Recovery {
		t.Fatal("corrupt cache did not preserve mounted recovery state")
	}
	if !reflect.DeepEqual(rpc.calls, []string{"vfs/stats"}) {
		t.Fatalf("unsafe RPC calls: %v", rpc.calls)
	}
}

func TestRecoveryRejectsDestinationChanges(t *testing.T) {
	for _, key := range []string{"type", "host", "user", "port", "path", "missing hash"} {
		t.Run(key, func(t *testing.T) {
			c := testConnection()
			c.Recovery = false
			config := map[string]string{"type": "ftp", "host": "example.test", "user": "original", "port": "21"}
			hash, err := checkDestination(c, config)
			if err != nil {
				t.Fatal(err)
			}
			c.Recovery, c.TargetHash = true, hash
			switch key {
			case "path":
				c.Path = "another-folder"
			case "missing hash":
				c.TargetHash = ""
			default:
				config[key] = "another-destination"
			}
			if _, err := checkDestination(c, config); err == nil {
				t.Fatal("pending uploads could be sent to a different destination")
			}
		})
	}
}
