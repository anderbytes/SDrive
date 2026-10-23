package engine

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProviderFailureCategoriesAreSanitized(t *testing.T) {
	for _, tc := range []struct{ raw, kind string }{
		{"invalid_grant: synthetic-secret", "credentials"},
		{"Authentication failed: synthetic-secret", "credentials"},
		{"insufficient_scope synthetic-secret", "permission"},
		{"AccessDenied synthetic-secret", "permission"},
		{"dial tcp: no such host synthetic-secret", "network"},
		{"i/o timeout synthetic-secret", "network"},
		{"unexpected provider failure synthetic-secret", "unknown"},
	} {
		for _, async := range []bool{false, true} {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if async && r.URL.Path == "/config/create" {
					_, _ = w.Write([]byte(`{"jobid":1}`))
					return
				}
				if async {
					_ = json.NewEncoder(w).Encode(map[string]any{"finished": true, "success": false, "error": tc.raw, "output": nil})
				} else {
					w.WriteHeader(http.StatusInternalServerError)
					_ = json.NewEncoder(w).Encode(map[string]any{"error": tc.raw, "input": map[string]string{"pass": "synthetic-secret"}})
				}
			}))
			e := &Engine{url: server.URL, done: make(chan struct{}), client: server.Client()}
			var err error
			if async {
				err = e.Call(context.Background(), "config/create", map[string]any{}, &map[string]any{})
			} else {
				err = e.Call(context.Background(), "operations/list", map[string]any{}, nil)
			}
			server.Close()
			var failure interface{ FailureKind() string }
			if !errors.As(err, &failure) || failure.FailureKind() != tc.kind || strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatalf("unsafe category: %v", err)
			}
		}
	}
}

func TestLocalRPCAuthenticationIsNotProviderCredentialFailure(t *testing.T) {
	for _, status := range []int{401, 403} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"invalid_grant synthetic-secret"}`))
		}))
		e := &Engine{url: server.URL, done: make(chan struct{}), client: server.Client()}
		err := e.Call(context.Background(), "config/create", map[string]any{}, nil)
		server.Close()
		var failure interface{ FailureKind() string }
		if !errors.As(err, &failure) || failure.FailureKind() != "engine" || strings.Contains(err.Error(), "synthetic-secret") {
			t.Fatal("local engine authentication confused with provider account")
		}
	}
}
