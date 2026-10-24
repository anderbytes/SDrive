package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"sdrive/internal/core"
	"sdrive/internal/engine"
	"sdrive/internal/platform"
)

// Profiles contain synthetic data only, and never use the user's data directory.
type integrationFixture struct {
	root, profile, destination, url, letter string
	blocked                                 atomic.Bool
	requests, downloads                     atomic.Int64
	upload                                  chan struct{}
	rpc                                     *engine.Engine
	service                                 *core.Service
}

func integrationRoot(t *testing.T) string {
	t.Helper()
	base, err := filepath.Abs(filepath.Join(".tools", "integration"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(base, "run-")
	if err != nil {
		t.Fatal(err)
	}
	if err = platform.ProtectDir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			// Never print keys, RPC credentials, configuration or raw engine output.
			_ = platform.AtomicWrite(filepath.Join(root, "result.json"), []byte(`{"result":"failed","data":"synthetic only"}`))
			t.Logf("isolated recovery evidence retained at %s", root)
		} else if err := os.RemoveAll(root); err != nil {
			t.Errorf("remove isolated fixture: %v", err)
		}
	})
	return root
}

func integrationEngine(t *testing.T, dir string) *engine.Engine {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := platform.ProtectDir(dir); err != nil {
		t.Fatal(err)
	}
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
	t.Cleanup(func() {
		rpc.Close()
		if rpc.Alive() {
			t.Error("isolated engine survived cleanup")
		}
	})
	return rpc
}

func integrationCall(t *testing.T, rpc core.RPC, method string, input, output any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := rpc.Call(ctx, method, input, output); err != nil {
		t.Fatalf("%s: %v", method, err)
	}
}

func newIntegrationFixture(t *testing.T, mount bool) *integrationFixture {
	t.Helper()
	if os.Getenv("SDRIVE_TEST_MOUNT") != "1" && (mount || os.Getenv("SDRIVE_TEST_ENGINE") != "1") {
		t.Skip("opt-in: SDRIVE_TEST_ENGINE=1 for local service tests; SDRIVE_TEST_MOUNT=1 for native mounts")
	}
	f := &integrationFixture{upload: make(chan struct{}, 1)}
	if mount {
		if !platform.WinFspInstalled() {
			t.Skip("native mount not run: compatible WinFsp is not installed")
		}
		for c := 'Z'; c >= 'D'; c-- {
			if !platform.DriveUsed(string(c)) {
				f.letter = string(c)
				break
			}
		}
		if f.letter == "" {
			t.Skip("native mount not run: no free letter between D and Z")
		}
	}
	if _, err := engine.FindBinary(); err != nil {
		t.Fatal(err)
	}
	f.root = integrationRoot(t)
	if mount {
		free, err := platform.FreeBytes(f.root)
		if err != nil || free < 6<<30 {
			t.Skip("native mount not run: fixture needs at least 6 GiB free")
		}
	}
	f.profile, f.destination = filepath.Join(f.root, "client"), filepath.Join(f.root, "destination")
	if err := os.MkdirAll(filepath.Join(f.destination, "library"), 0700); err != nil {
		t.Fatal(err)
	}
	server := integrationEngine(t, filepath.Join(f.root, "server"))
	var address struct{ ID, Addr string }
	integrationCall(t, server, "serve/start", map[string]any{"type": "webdav", "fs": f.destination, "addr": "127.0.0.1:0", "vfsOpt": map[string]any{"DirCacheTime": 0}}, &address)
	upstream, err := url.Parse("http://" + address.Addr)
	if err != nil || upstream.Hostname() != "127.0.0.1" {
		t.Fatal("fixture server is not on loopback")
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) { w.WriteHeader(http.StatusBadGateway) }
	gate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		if r.Method == http.MethodGet {
			f.downloads.Add(1)
		}
		if r.Method == http.MethodPut && f.blocked.Load() {
			select {
			case f.upload <- struct{}{}:
			default:
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(gate.Close)
	f.url = gate.URL
	return f
}

func openIntegrationService(t *testing.T, dir string, quit func()) (*core.Service, *engine.Engine) {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := core.New(dir, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	core.SetAutoStart(s, func(bool) error { return nil })
	var rpc *engine.Engine
	core.SetDesktop(s, nil, quit, func() (core.RPC, error) {
		rpc = integrationEngine(t, dir)
		return rpc, nil
	})
	core.StartEngine(s)
	if v := s.Snapshot(); v.SettingsError != "" || v.EngineError != "" || rpc == nil {
		t.Fatal("isolated service failed to initialize")
	}
	return s, rpc
}

func configureIntegration(t *testing.T, s *core.Service, endpoint, letter string) core.Connection {
	t.Helper()
	if letter == "" {
		letters := s.Snapshot().Letters
		if len(letters) == 0 {
			t.Skip("no free drive letter for disposable configuration")
		}
		letter = letters[len(letters)-1]
	}
	id, err := s.BeginConfig(core.ConfigInput{
		Connection: core.Connection{Name: "Synthetic WebDAV", Provider: "webdav", Letter: letter},
		Values:     map[string]string{"url": endpoint}, AllowInsecure: true, Guided: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	waitIntegration(t, "WebDAV authentication", 20*time.Second, func() bool {
		status, err := s.ConfigStatus(id)
		if err != nil || status.Error != "" {
			t.Fatal("local authentication failed")
		}
		return !status.Busy && status.Authenticated
	})
	folders, err := s.ListConfigFolders(id, "")
	if err != nil || len(folders) != 1 || folders[0].Path != "library" {
		t.Fatalf("local folder discovery failed: %v", err)
	}
	connectionID, err := s.ConfirmConfig(id, "library", core.MountTarget{Letter: letter})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.EndConfig(id); err != nil {
		t.Fatal(err)
	}
	connections := s.Snapshot().Connections
	if len(connections) != 1 || connections[0].ID != connectionID {
		t.Fatal("configuration was not persisted exactly once")
	}
	return connections[0].Connection
}

func waitIntegration(t *testing.T, stage string, limit time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for {
		if condition() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", stage)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func integrationHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func assertIntegrationFile(t *testing.T, path, expected string) {
	t.Helper()
	waitIntegration(t, "destination file content", 25*time.Second, func() bool {
		hash, err := integrationHash(path)
		return err == nil && hash == expected
	})
}

func integrationDirty(t *testing.T, dir string) bool {
	return integrationDirtyAt(t, filepath.Join(dir, "cache"))
}

func integrationDirtyAt(t *testing.T, cache string) bool {
	t.Helper()
	dirty := false
	complete := true
	err := filepath.WalkDir(filepath.Join(cache, "vfsMeta"), func(path string, entry os.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var info struct{ Dirty *bool }
		if err = json.Unmarshal(data, &info); err != nil || info.Dirty == nil {
			// rclone truncates metadata before rewriting; retry incomplete observations.
			complete = false
			return nil
		}
		dirty = dirty || *info.Dirty
		return nil
	})
	if err != nil {
		t.Fatalf("inspect synthetic metadata: %v", err)
	}
	return complete && dirty
}
