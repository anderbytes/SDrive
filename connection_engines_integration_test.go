package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sdrive/internal/core"
	"sdrive/internal/engine"
)

func TestConnectionEnginePersistenceAndLegacyImport(t *testing.T) {
	f := newIntegrationFixture(t, false)
	s, legacy := openIntegrationService(t, f.profile, nil)
	old := configureIntegration(t, s, f.url, "")
	core.SetConnectionEngine(s, func(c core.Connection, proxy string) (core.RPC, error) {
		return startConnectionEngine(f.profile, c, proxy, nil)
	})
	defer core.CloseConnectionEngines(s)
	if err := s.TestConnection(old.ID); err != nil {
		t.Fatalf("legacy import failed: %v", err)
	}
	path := filepath.Join(f.profile, "connections", "sd_"+old.Remote, "rclone.conf")
	raw, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(raw), "RCLONE_ENCRYPT_V0:") {
		t.Fatal("isolated config not encrypted")
	}
	fields, err := s.ConnectionFields(old.ID)
	if err != nil || fields.Values["url"] != f.url {
		t.Fatal("isolated access details missing")
	}
	core.CloseConnectionEngines(s)
	reopened, _ := openIntegrationService(t, f.profile, nil)
	core.SetConnectionEngine(reopened, func(c core.Connection, proxy string) (core.RPC, error) {
		return startConnectionEngine(f.profile, c, proxy, nil)
	})
	defer core.CloseConnectionEngines(reopened)
	if err = reopened.TestConnection(old.ID); err != nil {
		t.Fatalf("reopened isolated config failed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var original map[string]string
	if err = legacy.Call(ctx, "config/get", map[string]string{"name": "sd_" + old.Remote}, &original); err != nil || original["url"] != f.url {
		t.Fatal("migration changed the legacy source")
	}
	if reopened.Snapshot().Connections[0].Remote != old.Remote {
		t.Fatal("migration changed cache namespace")
	}
}

func TestSessionOnlyEngineDoesNotPersistCredentials(t *testing.T) {
	if os.Getenv("SDRIVE_TEST_ENGINE") != "1" {
		t.Skip("set SDRIVE_TEST_ENGINE=1 for disposable real-engine checks")
	}
	dir := integrationRoot(t)
	c := core.Connection{Remote: strings.Repeat("1", 32), SessionOnly: true}
	rpc, err := startConnectionEngine(dir, c, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rpc.(*engine.Engine).Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = rpc.Call(ctx, "config/create", map[string]any{"name": "sd_" + c.Remote, "type": "webdav", "parameters": map[string]string{"url": "https://example.invalid", "user": "synthetic-user", "pass": "synthetic-secret"}, "opt": map[string]bool{"nonInteractive": true, "obscure": true}}, nil); err != nil {
		t.Fatal(err)
	}
	rpc.(*engine.Engine).Close()
	if _, err = os.Stat(filepath.Join(dir, "connections")); !os.IsNotExist(err) {
		t.Fatal("memory worker created persistent credentials")
	}
}
