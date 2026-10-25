package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSMBTransportCommandAvailability(t *testing.T) {
	if os.Getenv("SDRIVE_TEST_ENGINE") != "1" {
		t.Skip("set SDRIVE_TEST_ENGINE=1 for disposable real-engine checks")
	}
	rpc := integrationEngine(t, filepath.Join(integrationRoot(t), "profile"))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := rpc.Call(ctx, "config/create", map[string]any{"name": "security_fixture", "type": "smb", "parameters": map[string]string{"host": "127.0.0.1", "user": "synthetic"}, "opt": map[string]bool{"nonInteractive": true}}, nil); err != nil {
		t.Fatal("could not configure disposable fixture")
	}
	// A server-root request must reach the extension without negotiating any server connection.
	if err := rpc.Call(ctx, "backend/command", map[string]string{"fs": "security_fixture:", "command": "transport-security"}, nil); err == nil {
		t.Fatal("server root unexpectedly reported protection")
	}
	var info struct{ Features struct{ Command bool } }
	if err := rpc.Call(ctx, "operations/fsinfo", map[string]string{"fs": "security_fixture:"}, &info); err != nil || !info.Features.Command {
		t.Fatal("modified engine does not expose SMB backend commands")
	}
}

// Use only a disposable server: the test authenticates and negotiates both shares without writing files.
func TestSMBTransportNegotiationIntegration(t *testing.T) {
	if os.Getenv("SDRIVE_TEST_SMB") != "1" {
		t.Skip("set SDRIVE_TEST_SMB=1 with disposable SMB fixture settings")
	}
	for _, key := range []string{"SDRIVE_SMB_HOST", "SDRIVE_SMB_USER", "SDRIVE_SMB_PASSWORD", "SDRIVE_SMB_ENCRYPTED_SHARE", "SDRIVE_SMB_UNENCRYPTED_SHARE"} {
		if os.Getenv(key) == "" {
			t.Fatalf("SMB opt-in requires %s", key)
		}
	}
	root := integrationRoot(t)
	rpc := integrationEngine(t, filepath.Join(root, "profile"))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	params := map[string]string{"host": os.Getenv("SDRIVE_SMB_HOST"), "user": os.Getenv("SDRIVE_SMB_USER"), "pass": os.Getenv("SDRIVE_SMB_PASSWORD"), "domain": os.Getenv("SDRIVE_SMB_DOMAIN"), "port": os.Getenv("SDRIVE_SMB_PORT")}
	if params["port"] == "" {
		delete(params, "port")
	}
	if err := rpc.Call(ctx, "config/create", map[string]any{"name": "security_fixture", "type": "smb", "parameters": params, "opt": map[string]bool{"nonInteractive": true, "obscure": true}}, nil); err != nil {
		t.Fatal("could not configure disposable SMB fixture")
	}
	for _, tc := range []struct {
		key  string
		want bool
	}{{"SDRIVE_SMB_ENCRYPTED_SHARE", true}, {"SDRIVE_SMB_UNENCRYPTED_SHARE", false}} {
		var response struct {
			Result *struct {
				Encrypted *bool `json:"encrypted"`
			} `json:"result"`
		}
		err := rpc.Call(ctx, "backend/command", map[string]string{"fs": "security_fixture:" + os.Getenv(tc.key), "command": "transport-security"}, &response)
		if err != nil || response.Result == nil || response.Result.Encrypted == nil {
			t.Fatal("SMB negotiation did not return a security sample")
		}
		if *response.Result.Encrypted != tc.want {
			t.Fatalf("unexpected negotiated encryption for %s", tc.key)
		}
	}
	if err := rpc.Call(ctx, "config/delete", map[string]string{"name": "security_fixture"}, nil); err != nil {
		t.Fatal("could not remove disposable SMB configuration")
	}
}
