package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRPCAuthenticationAndErrorRedaction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "user" || password != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/fail" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"password=private-password"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	e := &Engine{url: server.URL, user: "user", password: "secret", done: make(chan struct{}), client: server.Client()}
	if err := e.checkAuthentication(); err != nil {
		t.Fatal(err)
	}
	var result struct{ Ok bool }
	if err := e.Call(context.Background(), "success", map[string]string{}, &result); err != nil || !result.Ok {
		t.Fatalf("RPC: %v", err)
	}
	if err := e.Call(context.Background(), "fail", map[string]string{}, nil); err == nil || strings.Contains(err.Error(), "private-password") {
		t.Fatal("error must exist without exposing sensitive response body")
	}
}

func TestUnprotectedRPCIsRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	e := &Engine{url: server.URL, client: server.Client()}
	if e.checkAuthentication() == nil {
		t.Fatal("accepted unauthenticated API access")
	}
}

func TestRcloneEnvironmentIsProcessScoped(t *testing.T) {
	t.Setenv("RCLONE_CONFIG_PASS", "unrelated-global-value")
	t.Setenv("RCLONE_RC_NO_AUTH", "true")
	for _, value := range environment("RCLONE_RC_PASS=child-only") {
		if strings.HasPrefix(value, "RCLONE_CONFIG_PASS=") || strings.HasPrefix(value, "RCLONE_RC_NO_AUTH=") {
			t.Fatal("inherited global rclone configuration")
		}
	}
}

func TestRPCRejectsInvalidResponsesAndCancelledRequests(t *testing.T) {
	for _, tc := range []struct {
		name, body         string
		cancelled, stopped bool
	}{
		{name: "malformed JSON", body: `{"ok":`},
		{name: "empty response"},
		{name: "wrong shape", body: `[]`},
		{name: "cancelled", body: `{"ok":true}`, cancelled: true},
		{name: "stopped", body: `{"ok":true}`, stopped: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			e := &Engine{url: server.URL, done: make(chan struct{}), client: server.Client()}
			if tc.stopped {
				close(e.done)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelled {
				cancel()
			}
			var result struct{ Ok bool }
			if err := e.Call(ctx, "test", map[string]string{}, &result); err == nil || result.Ok {
				t.Fatalf("invalid RPC succeeded: %+v, %v", result, err)
			}
		})
	}
}

func TestInitRejectsPlaintextWithoutChangingConfiguration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rclone.conf")
	data := []byte("[account]\ntype = webdav\npass = private-value\n")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	err := Init("must-not-be-executed.exe", dir)
	if err == nil || strings.Contains(err.Error(), "private-value") {
		t.Fatalf("plaintext config not safely rejected: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(data) {
		t.Fatalf("existing credentials modified: %v", err)
	}
}

func TestRPCSendsJSONPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("invalid request: %s, %s", r.Method, r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["remote"] != "Documents/é" {
			t.Errorf("payload changed: %v, %v", body, err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	e := &Engine{url: server.URL, done: make(chan struct{}), client: server.Client()}
	if err := e.Call(context.Background(), "operations/list", map[string]string{"remote": "Documents/é"}, nil); err != nil {
		t.Fatal(err)
	}
}
