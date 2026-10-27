package core

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNetworkDestinationProbeUsesConnectionSession(t *testing.T) {
	rpc := &fakeRPC{handle: func(_ context.Context, _ string, input any) (any, error) {
		args := input.(map[string]string)
		return networkResponse(args["session"], args["action"], 0), nil
	}}
	s := testService(t, rpc)
	c := testConnection()
	s.store.Connections = []Connection{c}
	s.startNetwork(context.Background(), c, rpc)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "fixture") }))
	defer server.Close()
	client, err := s.destinationClient(c)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	view := s.sampleNetwork(context.Background(), c, rpc)
	if view.Sent == nil || *view.Sent <= 0 || *view.Received <= 7 || *view.FilesReceived != 0 {
		t.Fatal(view)
	}
}

func networkResponse(session, action string, bytes int64) map[string]any {
	return map[string]any{"session": session, "active": action != "stop", "startedAt": "2026-10-27T12:00:00Z", "checkedAt": time.Now().UTC(), "sent": bytes, "received": bytes, "filesSent": bytes, "filesReceived": bytes}
}

func TestNetworkSessionRetainsDisconnectedCountersAndResetsOnReconnect(t *testing.T) {
	var count int64 = 12
	fail := false
	rpc := &fakeRPC{handle: func(_ context.Context, method string, input any) (any, error) {
		if method != "sdrive/network" {
			return nil, nil
		}
		if fail {
			return nil, errors.New("synthetic failure")
		}
		args := input.(map[string]string)
		return networkResponse(args["session"], args["action"], count), nil
	}}
	s := testService(t, rpc)
	c := testConnection()
	s.store.Connections = []Connection{c}
	s.startNetwork(context.Background(), c, rpc)
	first := s.networkSessions[c.ID].id
	if got := s.sampleNetwork(context.Background(), c, rpc); got.Sent == nil || *got.Sent != 12 || got.State != "active" {
		t.Fatal(got)
	}
	fail = true
	if got := s.sampleNetwork(context.Background(), c, rpc); got.State != "stale" || *got.Sent != 12 {
		t.Fatal(got)
	}
	fail = false
	count = 24
	s.mu.Lock()
	s.stopNetwork(c)
	s.mu.Unlock()
	fail = true
	if got := s.sampleNetwork(context.Background(), c, rpc); got.State != "disconnected" || *got.Sent != 24 {
		t.Fatal(got)
	}
	fail = false
	count = 0
	s.startNetwork(context.Background(), c, rpc)
	if got := s.networkSessions[c.ID]; got.id == first || *got.view.Sent != 0 {
		t.Fatal(got)
	}
}

func TestNetworkReaderRejectsInvalidOrMissingCounters(t *testing.T) {
	for _, field := range []string{"sent", "received", "filesSent", "filesReceived", "active", "startedAt", "checkedAt", "session"} {
		for _, bad := range []any{nil, -1, "invalid"} {
			t.Run(field+"/invalid", func(t *testing.T) {
				rpc := &fakeRPC{handle: func(context.Context, string, any) (any, error) {
					result := networkResponse("session", "stats", 1)
					result[field] = bad
					return result, nil
				}}
				if _, err := readNetwork(context.Background(), &networkSession{id: "session", rpc: rpc}, "stats"); err == nil {
					t.Fatalf("accepted %s=%v", field, bad)
				}
			})
		}
	}
}

func TestNetworkOldResponseCannotContaminateReconnection(t *testing.T) {
	entered, resume := make(chan struct{}), make(chan struct{})
	rpc := &fakeRPC{handle: func(_ context.Context, _ string, input any) (any, error) {
		args := input.(map[string]string)
		if args["action"] == "stats" {
			close(entered)
			<-resume
		}
		return networkResponse(args["session"], args["action"], 9), nil
	}}
	s := testService(t, rpc)
	c := testConnection()
	s.store.Connections = []Connection{c}
	s.startNetwork(context.Background(), c, rpc)
	finished := make(chan NetworkView, 1)
	go func() { finished <- s.sampleNetwork(context.Background(), c, rpc) }()
	<-entered
	s.startNetwork(context.Background(), c, rpc)
	current := s.networkSessions[c.ID]
	close(resume)
	if got := <-finished; got.State != "" {
		t.Fatal("old response crossed session", got)
	}
	if s.networkSessions[c.ID] != current {
		t.Fatal("session replaced")
	}
}

func TestNetworkCounterResetAndExternalSSHRemainExplicit(t *testing.T) {
	count := int64(12)
	rpc := &fakeRPC{handle: func(_ context.Context, _ string, input any) (any, error) {
		args := input.(map[string]string)
		return networkResponse(args["session"], args["action"], count), nil
	}}
	s := testService(t, rpc)
	c := testConnection()
	c.Provider = "sftp"
	s.store.Connections = []Connection{c}
	s.startNetwork(context.Background(), c, rpc)
	s.mu.Lock()
	s.setNetworkCoverage(c, map[string]string{"ssh": "external"})
	s.mu.Unlock()
	if got := s.sampleNetwork(context.Background(), c, rpc); !got.Partial || got.State != "partial" {
		t.Fatal(got)
	}
	count = 0
	if got := s.sampleNetwork(context.Background(), c, rpc); got.State != "stale" || *got.Sent != 12 {
		t.Fatal(got)
	}
}
