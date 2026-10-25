package core

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCatalogTransportEncryption(t *testing.T) {
	optional := map[string]bool{"nextcloud": true, "owncloud": true, "webdav": true, "ftp": true, "s3": true, "azureblob": true, "mega": true, "smb": true}
	for _, p := range Catalog() {
		want := "required"
		if optional[p.ID] {
			want = "optional"
		}
		if p.TransportEncryption != want {
			t.Errorf("%s: %s, want %s", p.ID, p.TransportEncryption, want)
		}
	}
}

func TestTransportSecurityUsesEffectiveEngineConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, provider, state, protocol, source string
		config                                  map[string]string
		response                                any
		fail                                    bool
	}{
		{"HTTPS", "webdav", "encrypted", "https", "configuration", map[string]string{"type": "webdav", "url": "https://example.test/dav"}, nil, false},
		{"HTTP", "nextcloud", "unencrypted", "http", "configuration", map[string]string{"type": "webdav", "url": "http://example.test/dav"}, nil, false},
		{"missing URL", "webdav", "unknown", "", "", map[string]string{"type": "webdav"}, nil, false},
		{"invalid URL", "webdav", "unknown", "", "", map[string]string{"type": "webdav", "url": "not-a-url"}, nil, false},
		{"FTP", "ftp", "unencrypted", "ftp", "configuration", map[string]string{"type": "ftp"}, nil, false},
		{"explicit FTPS", "ftp", "encrypted", "ftps", "configuration", map[string]string{"type": "ftp", "explicit_tls": "true"}, nil, false},
		{"implicit FTPS", "ftp", "encrypted", "ftps", "configuration", map[string]string{"type": "ftp", "tls": "true"}, nil, false},
		{"conflicting FTPS", "ftp", "unknown", "", "", map[string]string{"type": "ftp", "tls": "true", "explicit_tls": "true"}, nil, false},
		{"SFTP", "sftp", "encrypted", "ssh", "protocol", map[string]string{"type": "sftp"}, nil, false},
		{"cloud HTTPS", "drive", "encrypted", "https", "protocol", map[string]string{"type": "drive"}, nil, false},
		{"S3 default", "s3", "encrypted", "https", "configuration", map[string]string{"type": "s3"}, nil, false},
		{"S3 HTTP", "s3", "unencrypted", "http", "configuration", map[string]string{"type": "s3", "endpoint": "http://example.test"}, nil, false},
		{"Azure SAS HTTP", "azureblob", "unencrypted", "http", "configuration", map[string]string{"type": "azureblob", "sas_url": "http://example.test/container?sig=synthetic"}, nil, false},
		{"MEGA default", "mega", "unencrypted", "http", "configuration", map[string]string{"type": "mega"}, nil, false},
		{"MEGA HTTPS", "mega", "encrypted", "https", "configuration", map[string]string{"type": "mega", "use_https": "true"}, nil, false},
		{"MEGA invalid", "mega", "unknown", "", "", map[string]string{"type": "mega", "use_https": "invalid"}, nil, false},
		{"SMB encrypted", "smb", "encrypted", "smb", "negotiated", map[string]string{"type": "smb"}, map[string]any{"result": map[string]bool{"encrypted": true}}, false},
		{"SMB unencrypted", "smb", "unencrypted", "smb", "negotiated", map[string]string{"type": "smb"}, map[string]any{"result": map[string]bool{"encrypted": false}}, false},
		{"SMB missing", "smb", "unknown", "", "", map[string]string{"type": "smb"}, map[string]any{"result": map[string]any{}}, false},
		{"SMB invalid", "smb", "unknown", "", "", map[string]string{"type": "smb"}, map[string]any{"result": map[string]any{"encrypted": "true"}}, false},
		{"SMB unsupported", "smb", "unknown", "", "", map[string]string{"type": "smb"}, nil, true},
		{"backend mismatch", "sftp", "unknown", "", "", map[string]string{"type": "ftp"}, nil, false},
		{"missing configuration", "sftp", "unknown", "", "", nil, nil, false},
		{"unhandled engine override", "webdav", "unknown", "", "", map[string]string{"type": "webdav", "url": "https://example.test", "override.proxy": "http://example.test"}, nil, false},
		{"Azure account key precedes SAS", "azureblob", "encrypted", "https", "configuration", map[string]string{"type": "azureblob", "account": "synthetic", "key": "synthetic", "sas_url": "http://example.test"}, nil, false},
		{"Azure emulator", "azureblob", "unencrypted", "http", "configuration", map[string]string{"type": "azureblob", "use_emulator": "true"}, nil, false},
		{"Azure malformed emulator", "azureblob", "unknown", "", "", map[string]string{"type": "azureblob", "use_emulator": "invalid"}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Connection{Provider: tc.provider, Remote: "synthetic", Path: "share", PrimaryURL: "https://primary.test/dav", AlternateURL: "https://alternate.test/dav"}
			rpc := &fakeRPC{handle: func(ctx context.Context, method string, input any) (any, error) {
				if method == "config/get" {
					return tc.config, nil
				}
				if method != "backend/command" {
					t.Fatalf("unexpected method %s", method)
				}
				args := input.(map[string]string)
				if args["fs"] != remote(c) || args["command"] != "transport-security" {
					t.Fatal("wrong remote or command")
				}
				if tc.fail {
					return nil, errors.New("unavailable")
				}
				return tc.response, nil
			}}
			got := readTransportSecurity(context.Background(), rpc, c)
			if got.State != tc.state || got.Protocol != tc.protocol || got.Source != tc.source {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestTransportSecuritySamplingInvalidatesChangedSessions(t *testing.T) {
	c := Connection{ID: "synthetic", Remote: "remote", Provider: "webdav"}
	calls := 0
	rpc := &fakeRPC{handle: func(ctx context.Context, method string, in any) (any, error) {
		calls++
		return map[string]string{"type": "webdav", "url": "https://example.test"}, nil
	}}
	s := testService(t, rpc)
	s.store.Connections = []Connection{c}
	s.mounted[c.ID] = true
	first := s.sampleTransportSecurity(context.Background(), c, rpc)
	second := s.sampleTransportSecurity(context.Background(), c, rpc)
	if first.State != "encrypted" || first.CheckedAt != second.CheckedAt || calls != 1 {
		t.Fatal("sample not reused")
	}
	s.transportInspections[c.ID].at = time.Now().Add(-time.Minute)
	_ = s.sampleTransportSecurity(context.Background(), c, rpc)
	if calls != 2 {
		t.Fatal("expired sample reused")
	}
	s.mounted[c.ID] = false
	if got := s.sampleTransportSecurity(context.Background(), c, rpc); got.State != "unknown" {
		t.Fatal("disconnected sample reused")
	}
	s.mounted[c.ID] = true
	s.busy[c.ID] = true
	if got := s.sampleTransportSecurity(context.Background(), c, rpc); got.State != "unknown" {
		t.Fatal("busy sample reused")
	}
	s.busy[c.ID] = false
	replacement := &fakeRPC{}
	s.workers = map[string]RPC{c.Remote: replacement}
	if got := s.sampleTransportSecurity(context.Background(), c, rpc); got.State != "unknown" {
		t.Fatal("old worker sample reused")
	}
}

func TestTransportSecurityDiscardsLateAndTimedOutResponses(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		c := Connection{ID: "synthetic", Remote: "remote", Provider: "smb"}
		started, release := make(chan struct{}), make(chan struct{})
		rpc := &fakeRPC{handle: func(ctx context.Context, method string, in any) (any, error) {
			if method == "config/get" {
				return map[string]string{"type": "smb"}, nil
			}
			close(started)
			if timeout {
				<-ctx.Done()
				return nil, ctx.Err()
			}
			<-release
			return map[string]any{"result": map[string]bool{"encrypted": true}}, nil
		}}
		s := testService(t, rpc)
		s.store.Connections = []Connection{c}
		s.mounted[c.ID] = true
		done := make(chan TransportSecurityView, 1)
		go func() { done <- s.sampleTransportSecurity(context.Background(), c, rpc) }()
		<-started
		if !timeout {
			s.mu.Lock()
			delete(s.transportInspections, c.ID)
			s.mu.Unlock()
			close(release)
		}
		if got := <-done; got.State != "unknown" {
			t.Fatal("late or timed-out response claimed encryption")
		}
	}
}
