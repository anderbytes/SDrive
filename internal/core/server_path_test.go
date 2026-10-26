package core

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func TestServerPathFormatsDestinationsWithoutCredentials(t *testing.T) {
	for _, tc := range []struct {
		provider string
		config   map[string]string
		want     string
	}{
		{"webdav", map[string]string{"url": "https://user:secret@example.test/dav?token=secret#secret"}, "https://example.test/dav/team/docs"},
		{"s3", map[string]string{"endpoint": "objects.example.test"}, "https://objects.example.test/team/docs"},
		{"azureblob", map[string]string{"account": "example"}, "https://example.blob.core.windows.net/team/docs"},
		{"sftp", map[string]string{"host": "2001:db8::1", "port": "2222"}, "[2001:db8::1]:2222/team/docs"},
		{"ftp", map[string]string{"host": "files.example.test"}, "files.example.test/team/docs"},
		{"smb", map[string]string{"host": "server"}, `\\server\team\docs`},
		{"drive", map[string]string{"token": "secret"}, "team/docs"},
		{"sftp", map[string]string{"host": "user:secret@server"}, "team/docs"},
		{"webdav", map[string]string{"url": "https://bad\nsecret"}, "team/docs"},
	} {
		t.Run(tc.provider+tc.want, func(t *testing.T) {
			got := serverPath(Connection{Provider: tc.provider, Path: "team/docs"}, tc.config)
			if got != tc.want || strings.Contains(got, "secret") {
				t.Fatalf("got %q; want %q", got, tc.want)
			}
		})
	}
}

func TestServerPathConcurrentReadersShareOneConfigurationRead(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
		if method == "config/get" {
			close(started)
			<-release
		}
		return map[string]string{"type": "webdav", "url": "https://example.test/dav"}, nil
	}}
	c := testConnection()
	c.Provider = "webdav"
	s := testService(t, rpc)
	s.store.Connections = []Connection{c}
	first := make(chan string, 1)
	go func() { first <- s.connectionServerPath(context.Background(), c) }()
	<-started
	if got := s.connectionServerPath(context.Background(), c); got != c.Path {
		t.Fatal("an unfinished local read must return the folder immediately")
	}
	var readers sync.WaitGroup
	for i := 0; i < 20; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for j := 0; j < 200; j++ {
				s.connectionServerPath(context.Background(), c)
			}
		}()
	}
	close(release)
	readers.Wait()
	if got := <-first; got != "https://example.test/dav/" || len(rpc.calls) != 1 {
		t.Fatal("concurrent readers did not share the result", got)
	}
}

func TestServerPathCachesLocalReadsAndRefreshesAfterReconfiguration(t *testing.T) {
	c := testConnection()
	c.Provider, c.Recovery = "webdav", false
	rpc := &fakeRPC{remotes: map[string]map[string]string{"sd_" + c.Remote: {"type": "webdav", "url": "https://example.test/dav?token=secret"}}}
	s := testService(t, rpc)
	s.store.Connections = []Connection{c}
	for i := 0; i < 2; i++ {
		if got := s.connectionServerPath(context.Background(), c); got != "https://example.test/dav/" {
			t.Fatal(got)
		}
	}
	if len(rpc.calls) != 1 {
		t.Fatal("list refresh repeated the configuration read")
	}
	c.Path = "docs"
	s.store.Connections[0] = c
	if got := s.connectionServerPath(context.Background(), c); got != "https://example.test/dav/docs" || len(rpc.calls) != 2 {
		t.Fatal("destination change did not invalidate summary", got)
	}
	s.access.locked = true
	// A locked snapshot must never include any connection metadata.
	if got := s.Snapshot(); len(got.Connections) != 0 {
		t.Fatal("locked view exposed destinations")
	}
}
