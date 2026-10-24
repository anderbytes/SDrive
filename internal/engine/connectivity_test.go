package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestChildEnvironmentExcludesInheritedProxies(t *testing.T) {
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY"} {
		t.Setenv(key, "http://unrelated.example.test")
	}
	for _, entry := range environment() {
		key := strings.SplitN(entry, "=", 2)[0]
		switch strings.ToUpper(key) {
		case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY":
			t.Fatal("inherited proxy")
		}
	}
}

func memoryEngineForTest(t *testing.T, binary string, options Options) *Engine {
	t.Helper()
	dir := t.TempDir()
	options.Memory = true
	options.CacheDir = filepath.Join(dir, "cache")
	e, err := StartWithOptions(binary, dir, nil, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Close)
	return e
}

func TestConnectionEnginesEnforceIndependentTransferLimits(t *testing.T) {
	if os.Getenv("SDRIVE_TEST_ENGINE") != "1" {
		t.Skip("set SDRIVE_TEST_ENGINE=1 for disposable real-engine checks")
	}
	binary, err := filepath.Abs(filepath.Join("..", "..", ".tools", "rclone", "rclone.exe"))
	if err != nil {
		t.Fatal(err)
	}
	server := memoryEngineForTest(t, binary, Options{})
	storage := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var address struct{ ID, Addr string }
	if err = server.Call(ctx, "serve/start", map[string]any{"type": "webdav", "fs": storage, "addr": "127.0.0.1:0"}, &address); err != nil {
		t.Fatal(err)
	}
	const fileSize = 512 << 10
	payload := []byte(strings.Repeat("synthetic-data-", fileSize/15+1))[:fileSize]
	local := t.TempDir()
	if err = os.WriteFile(filepath.Join(local, "Café 日本 😀.bin"), payload, 0600); err != nil {
		t.Fatal(err)
	}
	clients := []*Engine{memoryEngineForTest(t, binary, Options{UploadLimit: 128 << 10, DownloadLimit: 128 << 10}), memoryEngineForTest(t, binary, Options{UploadLimit: 512 << 10, DownloadLimit: 512 << 10})}
	for _, client := range clients {
		if err = client.Call(ctx, "config/create", map[string]any{"name": "drive", "type": "webdav", "parameters": map[string]string{"url": "http://" + address.Addr, "user": "", "pass": "synthetic-secret"}, "opt": map[string]bool{"nonInteractive": true, "obscure": true}}, nil); err != nil {
			t.Fatal(err)
		}
	}
	for _, upload := range []bool{true, false} {
		var elapsed [2]time.Duration
		var failures [2]error
		var wg sync.WaitGroup
		for i, client := range clients {
			wg.Add(1)
			go func(i int, client *Engine) {
				defer wg.Done()
				args := map[string]string{"srcFs": local, "srcRemote": "Café 日本 😀.bin", "dstFs": "drive:", "dstRemote": fmt.Sprintf("file-%d.bin", i)}
				if !upload {
					args = map[string]string{"srcFs": "drive:", "srcRemote": fmt.Sprintf("file-%d.bin", i), "dstFs": local, "dstRemote": fmt.Sprintf("download-%d.bin", i)}
				}
				start := time.Now()
				failures[i] = client.Call(ctx, "operations/copyfile", args, nil)
				elapsed[i] = time.Since(start)
			}(i, client)
		}
		wg.Wait()
		for _, failure := range failures {
			if failure != nil {
				t.Fatal(failure)
			}
		}
		if elapsed[0] < 2500*time.Millisecond || elapsed[1] >= elapsed[0]*3/4 {
			t.Fatalf("limits not independent (upload=%t, slow=%s, fast=%s)", upload, elapsed[0], elapsed[1])
		}
		t.Logf("isolated transfer upload=%t slow=%s fast=%s", upload, elapsed[0].Round(time.Millisecond), elapsed[1].Round(time.Millisecond))
	}
	for i := range clients {
		data, err := os.ReadFile(filepath.Join(local, fmt.Sprintf("download-%d.bin", i)))
		if err != nil || string(data) != string(payload) {
			t.Fatal("content changed")
		}
	}
	// Memory configuration must never create a credential file, including on refresh.
	for _, client := range clients {
		if strings.Contains(strings.Join(client.cmd.Args, " "), "--config NUL") == false {
			t.Fatal("memory worker uses a persistent config")
		}
	}
}

func TestProviderProxyDoesNotAffectLocalRPC(t *testing.T) {
	if os.Getenv("SDRIVE_TEST_ENGINE") != "1" {
		t.Skip("set SDRIVE_TEST_ENGINE=1 for disposable real-engine checks")
	}
	binary, err := filepath.Abs(filepath.Join("..", "..", ".tools", "rclone", "rclone.exe"))
	if err != nil {
		t.Fatal(err)
	}
	var proxied atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied.Add(1)
		if r.Header.Get("Proxy-Authorization") == "" {
			t.Error("missing proxy authentication")
		}
		w.WriteHeader(http.StatusProxyAuthRequired)
	}))
	defer proxy.Close()
	proxyURL := strings.Replace(proxy.URL, "http://", "http://synthetic-user:synthetic-secret@", 1)
	worker := memoryEngineForTest(t, binary, Options{ProxyURL: proxyURL})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// RPC remains usable while the provider's proxy rejects authentication.
	if err = worker.Call(ctx, "core/version", nil, &map[string]any{}); err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"name": "drive", "type": "http", "parameters": map[string]string{"url": "http://storage.invalid/"}, "opt": map[string]bool{"nonInteractive": true}}
	if err = worker.Call(ctx, "config/create", input, nil); err != nil {
		t.Fatal(err)
	}
	err = worker.Call(ctx, "operations/stat", map[string]string{"fs": "drive:", "remote": "file.txt"}, &map[string]any{})
	if err == nil || proxied.Load() == 0 {
		t.Fatal("provider traffic did not use rejecting proxy")
	}
	if strings.Contains(err.Error(), "synthetic-secret") || strings.Contains(err.Error(), "storage.invalid") {
		t.Fatal("provider/proxy data leaked")
	}
	if err = worker.Call(ctx, "core/version", nil, &map[string]any{}); err != nil {
		t.Fatal("proxy affected local RPC")
	}
}
