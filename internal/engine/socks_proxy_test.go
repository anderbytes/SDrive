package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sdrive/internal/diagnostics"
	"sdrive/internal/testutil"
)

func TestSOCKSProxyTransfersDNSFailuresAndReconnect(t *testing.T) {
	if os.Getenv("SDRIVE_TEST_ENGINE") != "1" {
		t.Skip("set SDRIVE_TEST_ENGINE=1 for disposable real-engine checks")
	}
	binary, err := filepath.Abs(filepath.Join("..", "..", ".tools", "rclone", binaryName()))
	if err != nil {
		t.Fatal(err)
	}
	server := memoryEngineForTest(t, binary, Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var address struct{ ID, Addr string }
	if err := server.Call(ctx, "serve/start", map[string]any{"type": "webdav", "fs": t.TempDir(), "addr": "127.0.0.1:0"}, &address); err != nil {
		t.Fatal(err)
	}
	target, _ := url.Parse("http://" + address.Addr)
	forward := httputil.NewSingleHostReverseProxy(target)
	var directRequests atomic.Int64
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		directRequests.Add(1)
		forward.ServeHTTP(w, r)
	}))
	defer storage.Close()
	const payload = "synthetic SOCKS content: Café 日本 😀"
	for _, scheme := range []string{"socks5", "socks5h"} {
		for _, auth := range []bool{false, true} {
			name := scheme + "/anonymous"
			user, password := "", ""
			if auth {
				name = scheme + "/authenticated"
				user, password = "synthetic-user", "synthetic-secret"
			}
			t.Run(name, func(t *testing.T) {
				proxy := testutil.NewSOCKSProxy(t, storage.Listener.Addr().String(), user, password)
				proxyURL, _ := url.Parse(strings.Replace(proxy.URL, "socks5:", scheme+":", 1))
				if auth {
					proxyURL.User = url.UserPassword(user, password)
				}
				log := diagnostics.New(t.TempDir())
				defer log.Close()
				start := func(route, destination string) *Engine {
					t.Helper()
					dir := t.TempDir()
					worker, err := StartWithOptions(binary, dir, log, Options{Memory: true, CacheDir: filepath.Join(dir, "cache"), ProxyURL: route, ConnectTimeout: 2, IdleTimeout: 3})
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(worker.Close)
					if err := worker.Call(ctx, "config/create", map[string]any{"name": "drive", "type": "webdav", "parameters": map[string]string{"url": destination}, "opt": map[string]bool{"nonInteractive": true}}, nil); err != nil {
						t.Fatal(err)
					}
					return worker
				}
				local := t.TempDir()
				remote := strings.ReplaceAll(name, "/", "-") + ".txt"
				if err := os.WriteFile(filepath.Join(local, "source.txt"), []byte(payload), 0600); err != nil {
					t.Fatal(err)
				}
				worker := start(proxyURL.String(), "http://storage.invalid/")
				if err := worker.Call(ctx, "operations/copyfile", map[string]string{"srcFs": local, "srcRemote": "source.txt", "dstFs": "drive:", "dstRemote": remote}, nil); err != nil {
					t.Fatal(err)
				}
				worker.Close()
				worker = start(proxyURL.String(), "http://storage.invalid/")
				if err := worker.Call(ctx, "operations/copyfile", map[string]string{"srcFs": "drive:", "srcRemote": remote, "dstFs": local, "dstRemote": "download.txt"}, nil); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(filepath.Join(local, "download.txt"))
				if err != nil || string(data) != payload || proxy.Domains.Load() == 0 {
					t.Fatal("reconnected transfer changed content or resolved the destination locally")
				}
				worker.Close()
				assertFailure := func(route string) {
					t.Helper()
					before := directRequests.Load()
					worker := start(route, storage.URL)
					defer worker.Close()
					failureCtx, stop := context.WithTimeout(ctx, 8*time.Second)
					defer stop()
					err := worker.Call(failureCtx, "operations/stat", map[string]any{"fs": "drive:", "remote": remote, "_config": map[string]any{"LowLevelRetries": 1}}, &map[string]any{})
					if err == nil || directRequests.Load() != before {
						t.Fatal("SOCKS failure silently used direct storage access")
					}
					if strings.Contains(err.Error(), "synthetic-secret") || strings.Contains(err.Error(), "synthetic-user") {
						t.Fatal("proxy error exposed credentials")
					}
					if err := worker.Call(ctx, "core/version", nil, &map[string]any{}); err != nil {
						t.Fatal("SOCKS failure affected local RPC")
					}
					if strings.Contains(strings.Join(worker.cmd.Args, " "), route) {
						t.Fatal("proxy exposed in command arguments")
					}
				}
				if auth {
					wrong := *proxyURL
					wrong.User = url.UserPassword(user, "wrong-password")
					assertFailure(wrong.String())
				}
				proxy.Close()
				assertFailure(proxyURL.String())
				for _, secret := range []string{"synthetic-user", "synthetic-secret", "wrong-password", "storage.invalid"} {
					if strings.Contains(log.Report(), secret) {
						t.Fatal("SOCKS details leaked in diagnostics")
					}
				}
			})
		}
	}
}
