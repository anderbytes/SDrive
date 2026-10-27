package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNetworkCountersIncludeMetadataAndIsolateWorkers(t *testing.T) {
	if os.Getenv("SDRIVE_TEST_ENGINE") != "1" {
		t.Skip("set SDRIVE_TEST_ENGINE=1 for disposable real-engine checks")
	}
	binary, err := filepath.Abs(filepath.Join("..", "..", ".tools", "rclone", binaryName()))
	if err != nil {
		t.Fatal(err)
	}
	server := memoryEngineForTest(t, binary, Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var address struct{ Addr string }
	if err := server.Call(ctx, "serve/start", map[string]any{"type": "webdav", "fs": t.TempDir(), "addr": "127.0.0.1:0"}, &address); err != nil {
		t.Fatal(err)
	}
	clients := []*Engine{memoryEngineForTest(t, binary, Options{}), memoryEngineForTest(t, binary, Options{})}
	type counters struct{ Sent, Received, FilesSent, FilesReceived int64 }
	stats := func(client *Engine, action, session string) counters {
		t.Helper()
		var result counters
		if err := client.Call(ctx, "sdrive/network", map[string]string{"action": action, "session": session}, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	for _, client := range clients {
		if err := client.Call(ctx, "config/create", map[string]any{"name": "drive", "type": "webdav", "parameters": map[string]string{"url": "http://" + address.Addr}, "opt": map[string]bool{"nonInteractive": true}}, nil); err != nil {
			t.Fatal(err)
		}
		stats(client, "start", "connection")
	}
	if err := clients[0].Call(ctx, "operations/list", map[string]string{"fs": "drive:", "remote": ""}, nil); err != nil {
		t.Fatal(err)
	}
	listing := stats(clients[0], "stats", "connection")
	if listing.Sent <= 0 || listing.Received <= 0 || listing.FilesSent != 0 || listing.FilesReceived != 0 {
		t.Fatalf("metadata was not isolated: %+v", listing)
	}
	if got := stats(clients[1], "stats", "connection"); got != (counters{}) {
		t.Fatalf("workers shared traffic: %+v", got)
	}
	for range 5 {
		if got := stats(clients[0], "stats", "connection"); got != listing {
			t.Fatalf("control API counted as network: %+v", got)
		}
	}
	local := t.TempDir()
	payload := []byte(strings.Repeat("synthetic network fixture\n", 4096))
	if err := os.WriteFile(filepath.Join(local, "upload.bin"), payload, 0600); err != nil {
		t.Fatal(err)
	}
	if err := clients[0].Call(ctx, "operations/copyfile", map[string]string{"srcFs": local, "srcRemote": "upload.bin", "dstFs": "drive:", "dstRemote": "remote.bin"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := clients[0].Call(ctx, "operations/copyfile", map[string]string{"srcFs": "drive:", "srcRemote": "remote.bin", "dstFs": local, "dstRemote": "download.bin"}, nil); err != nil {
		t.Fatal(err)
	}
	transfers := stats(clients[0], "stats", "connection")
	if transfers.FilesSent != int64(len(payload)) || transfers.FilesReceived != int64(len(payload)) || transfers.Sent <= transfers.FilesSent || transfers.Received <= transfers.FilesReceived {
		t.Fatalf("file/socket layers incorrect: %+v", transfers)
	}
	stopped := stats(clients[0], "stop", "connection")
	if err := clients[0].Call(ctx, "operations/list", map[string]string{"fs": "drive:", "remote": ""}, nil); err != nil {
		t.Fatal(err)
	}
	if got := stats(clients[0], "stats", "connection"); got != stopped {
		t.Fatalf("stopped session grew: %+v", got)
	}
	if got := stats(clients[0], "start", "reconnected"); got != (counters{}) {
		t.Fatalf("reconnect did not reset: %+v", got)
	}
}
