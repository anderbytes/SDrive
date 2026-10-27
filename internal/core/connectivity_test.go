package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOAuthRequiresUserApplicationOutsideGuidedSetup(t *testing.T) {
	for _, id := range []string{"drive", "onedrive", "sharepoint", "dropbox", "box", "pcloud", "gcs", "yandex"} {
		p, _ := provider(id)
		if !p.Available {
			t.Fatalf("%s hidden behind project registration", id)
		}
		for _, guided := range []bool{false, true} {
			rpc := &fakeRPC{}
			s := testService(t, rpc)
			_, err := s.BeginConfig(ConfigInput{Connection: Connection{Name: "Synthetic", Provider: id, Letter: "S"}, Guided: guided, Values: map[string]string{}})
			if err == nil {
				t.Fatal("missing user credentials accepted")
			}
			rpc.mu.Lock()
			for _, method := range rpc.calls {
				if method == "config/create" {
					t.Error("authorization started without user credentials")
				}
			}
			rpc.mu.Unlock()
		}
	}
}

func TestOAuthClientChangeCannotReuseSecret(t *testing.T) {
	old := map[string]string{"type": "drive", "client_id": "original", "client_secret": "original-secret", "token": "old"}
	rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
		if method == "config/get" {
			return old, nil
		}
		return ConfigResult{}, nil
	}}
	s := testService(t, rpc)
	c := testConnection()
	c.Provider = "drive"
	c.Recovery = false
	s.store.Connections = []Connection{c}
	_, err := s.BeginConfig(ConfigInput{Connection: c, Values: map[string]string{"client_id": "replacement"}, Guided: true})
	if err == nil {
		t.Fatal("old secret reused with new client ID")
	}
	if !reflect.DeepEqual(s.store.Connections, []Connection{c}) {
		t.Fatal("failed renewal changed saved identity")
	}
}

func TestConnectionNetworkValidationAndSettingsCompatibility(t *testing.T) {
	valid := Connection{Name: "Synthetic", Provider: "webdav", Letter: "S", UploadLimit: 1024, DownloadLimit: 2048, UseProxy: true, ConnectTimeout: 10, IdleTimeout: 20, AlternateURL: "https://alternate.example.test/dav", SessionOnly: true}
	if err := validateConnection(&valid); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Connection){func(c *Connection) { c.UploadLimit = -1 }, func(c *Connection) { c.DownloadLimit = 1 << 41 }, func(c *Connection) { c.IdleTimeout = 3601 }, func(c *Connection) { c.PrivateBrowser = true }, func(c *Connection) { c.Browser = "cmd.exe" }, func(c *Connection) { c.AlternateURL = "https://user:secret@example.test" }, func(c *Connection) { c.AlternateURL = "http://example.test" }, func(c *Connection) { c.AutoMount = true }, func(c *Connection) { c.Provider = "ftp" }} {
		c := valid
		change(&c)
		if validateConnection(&c) == nil {
			t.Fatal("unsafe network options accepted")
		}
	}
	store := defaultStore()
	c := testConnection()
	c.Provider = "webdav"
	c.UploadLimit = 1024
	c.Browser = "firefox"
	c.PrivateBrowser = true
	store.Connections = []Connection{c}
	raw, err := encodeStore(store)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"version": 6`) {
		t.Fatal("old application could ignore safety fields")
	}
	loaded, err := decodeStore(raw)
	if err != nil || !reflect.DeepEqual(store, loaded) {
		t.Fatal("network fields did not round trip")
	}
}

func TestProxySettingsAreProtectedAndRedacted(t *testing.T) {
	s := testService(t, nil)
	p := ProxyConfig{Address: "http://proxy.example.test:8080", Username: "synthetic-user", Password: "synthetic-password"}
	if err := s.SaveProxy(p); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(s.dir, "proxy.enc"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), p.Password) || strings.Contains(string(raw), p.Username) {
		t.Fatal("proxy credentials stored unprotected")
	}
	view, err := s.ProxySettings()
	if err != nil || view.Password != "" || !view.HasPassword {
		t.Fatal("proxy secret returned through bridge")
	}
	if err = s.SaveProxy(view); err != nil {
		t.Fatal(err)
	}
	saved, err := s.readProxy()
	if err != nil || saved.Password != p.Password {
		t.Fatal("blank renewal lost saved proxy password")
	}
	view.Address = "https://other.example.test"
	if err = s.SaveProxy(view); err != nil {
		t.Fatal(err)
	}
	saved, _ = s.readProxy()
	if saved.Password != "" {
		t.Fatal("proxy secret carried to a different proxy")
	}
	if err = s.SaveProxy(ProxyConfig{Address: "http://user:secret@example.test"}); err == nil {
		t.Fatal("embedded proxy credentials accepted")
	}
	raw[len(raw)-1] ^= 1
	if err = os.WriteFile(filepath.Join(s.dir, "proxy.enc"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ProxySettings(); err == nil {
		t.Fatal("tampered proxy file accepted")
	}
}

func TestAuthorizationURLIsBoundToLocalCallback(t *testing.T) {
	if !validAuthorizationURL("http://127.0.0.1:53682/auth?state=synthetic") {
		t.Fatal("valid callback rejected")
	}
	for _, value := range []string{"https://example.test/auth?state=x", "http://localhost:53682/auth?state=x", "http://127.0.0.1:53682/other?state=x", "http://user:secret@127.0.0.1:53682/auth?state=x", "http://127.0.0.1:53682/auth?state=x&target=elsewhere", "http://127.0.0.1:53682/auth"} {
		if validAuthorizationURL(value) {
			t.Fatal("untrusted callback accepted")
		}
	}
}

func identityResponse(href, id string) string {
	return fmt.Sprintf(`<d:multistatus xmlns:d="DAV:"><d:response><d:href>%s</d:href><d:propstat><d:prop><d:resource-id><d:href>%s</d:href></d:resource-id></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>`, href, id)
}

func TestWebDAVResourceIdentityAndFallback(t *testing.T) {
	var resource atomic.Value
	resource.Store("urn:uuid:original")
	var primaryStatus atomic.Int64
	primaryStatus.Store(207)
	primary := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PROPFIND" || r.Header.Get("Depth") != "0" {
			t.Error("identity check was not read-only")
		}
		w.WriteHeader(int(primaryStatus.Load()))
		fmt.Fprint(w, identityResponse(r.URL.EscapedPath(), "urn:uuid:original"))
	}))
	defer primary.Close()
	alternate := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(207)
		fmt.Fprint(w, identityResponse(r.URL.EscapedPath(), resource.Load().(string)))
	}))
	defer alternate.Close()
	client := primary.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c := Connection{Provider: "webdav", Path: "Café 😀", SessionOnly: true, AlternateURL: alternate.URL}
	config := map[string]string{"url": primary.URL}
	selected, err := verifyWebDAV(context.Background(), client, &c, config, true)
	if err != nil || selected != primary.URL || c.ResourceID != resourceDigest("urn:uuid:original") {
		t.Fatalf("identity verification failed: %v", err)
	}
	resource.Store("urn:uuid:other")
	copy := c
	if _, err = verifyWebDAV(context.Background(), client, &copy, config, true); err == nil {
		t.Fatal("different alternate storage accepted")
	}
	resource.Store("urn:uuid:original")
	primaryStatus.Store(401)
	if _, err = verifyWebDAV(context.Background(), client, &copy, config, false); err == nil {
		t.Fatal("authentication failure caused fallback")
	}
	primary.Close()
	selected, err = verifyWebDAV(context.Background(), client, &copy, config, false)
	if err != nil || selected != alternate.URL {
		t.Fatalf("verified fallback failed: %v", err)
	}
	resource.Store("urn:uuid:other")
	if _, err = verifyWebDAV(context.Background(), client, &copy, config, false); err == nil {
		t.Fatal("fallback bypassed destination binding")
	}
}

func TestWebDAVResourceIdentityRejectsInvalidResponsesAndRedirects(t *testing.T) {
	for _, body := range []string{`<bad>`, identityResponse("/wrong", "urn:uuid:resource"), identityResponse("/dav", ""), strings.Repeat("x", (64<<10)+1)} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(207); fmt.Fprint(w, body) }))
		_, _, err := probeResource(context.Background(), server.Client(), server.URL+"/dav", "", "", "")
		server.Close()
		if err == nil {
			t.Fatal("invalid resource response accepted")
		}
	}
	targetCalls := atomic.Int64{}
	target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetCalls.Add(1) }))
	defer target.Close()
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer redirect.Close()
	client := redirect.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if _, _, err := probeResource(context.Background(), client, redirect.URL, "", "synthetic-user", "synthetic-password"); err == nil || targetCalls.Load() != 0 {
		t.Fatal("identity credentials followed a redirect")
	}
}

func TestConnectionEngineImportIsIsolatedAndVerified(t *testing.T) {
	c := testConnection()
	c.Recovery = false
	original := map[string]string{"type": "ftp", "host": "original.example.test", "user": "synthetic", "pass": "obscured"}
	legacy := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
		if method == "config/get" {
			return original, nil
		}
		return map[string]any{}, nil
	}}
	s := testService(t, legacy)
	s.store.Connections = []Connection{c}
	workers := map[string]*fakeRPC{}
	SetConnectionEngine(s, func(c Connection, _ string) (RPC, error) {
		worker := &fakeRPC{}
		workers[c.Remote] = worker
		return worker, nil
	})
	s.mu.Lock()
	rpc, err := s.prepareConnection(context.Background(), c, true)
	s.mu.Unlock()
	if err != nil || rpc == legacy {
		t.Fatalf("isolated import failed: %v", err)
	}
	if original["pass"] != "obscured" {
		t.Fatal("legacy credentials mutated")
	}
	second := c
	second.Remote = strings.Repeat("3", 32)
	s.mu.Lock()
	other, err := s.prepareConnection(context.Background(), second, false)
	s.mu.Unlock()
	if err != nil || other == rpc || len(workers) != 2 {
		t.Fatal("connections share an engine")
	}
	s.mu.Lock()
	s.stopConnection(c)
	remaining := s.connectionRPC(second)
	s.mu.Unlock()
	if remaining != other {
		t.Fatal("stopping one drive changed another")
	}
}

func TestSessionOnlyRecoveryRequiresSameLibrary(t *testing.T) {
	c := testConnection()
	c.Provider = "onedrive"
	c.SessionOnly = true
	c.AuthMethod = "oauth"
	c.Recovery = false
	config := map[string]string{"type": "onedrive", "drive_id": "original-library"}
	c.TargetHash, _ = checkDestination(c, config)
	c.Recovery = true
	raw, _ := json.Marshal(destinationFields(c, config))
	c.Binding = string(raw)
	rpc := &fakeRPC{}
	s := testService(t, rpc)
	s.store.Connections = []Connection{c}
	SetConnectionEngine(s, func(Connection, string) (RPC, error) { return &fakeRPC{}, nil })
	requested := c
	requested.Browser = "firefox"
	requested.PrivateBrowser = true
	id, err := s.BeginConfig(ConfigInput{Connection: requested, Guided: true, Values: map[string]string{"client_id": "own-app", "client_secret": "own-secret"}})
	if err != nil {
		t.Fatal(err)
	}
	status := waitSession(t, s, id)
	if !status.Authenticated || status.Error != "" {
		t.Fatalf("original session-only destination lost: %s", status.Error)
	}
	if status.connection.Browser != "firefox" || !status.connection.PrivateBrowser {
		t.Fatal("recovery discarded private browser preferences")
	}
	b, _ := json.Marshal(s.store)
	if strings.Contains(string(b), "own-secret") || strings.Contains(string(b), "synthetic-test-token") {
		t.Fatal("session credentials persisted")
	}
	if err = s.EndConfig(id); err != nil {
		t.Fatal(err)
	}
}

func TestConnectionEngineStartupReleasesServiceMutex(t *testing.T) {
	s := testService(t, &fakeRPC{})
	started, finish := make(chan struct{}), make(chan struct{})
	SetConnectionEngine(s, func(Connection, string) (RPC, error) { close(started); <-finish; return &fakeRPC{}, nil })
	done := make(chan error, 1)
	go func() {
		s.mu.Lock()
		_, err := s.prepareConnection(context.Background(), testConnection(), false)
		s.mu.Unlock()
		done <- err
	}()
	receive(t, started)
	acquired := make(chan struct{})
	go func() { s.mu.Lock(); close(acquired); s.mu.Unlock() }()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Error("startup held service mutex")
	}
	close(finish)
	if err := receive(t, done); err != nil {
		t.Fatal(err)
	}
}

func TestCredentialRemovalFailurePreservesPersistentMode(t *testing.T) {
	rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
		if method == "config/delete" {
			return nil, fmt.Errorf("synthetic failure")
		}
		return ConfigResult{}, nil
	}}
	s := testService(t, rpc)
	c := testConnection()
	c.Provider = "webdav"
	c.Recovery = false
	s.store.Connections = []Connection{c}
	next := c
	next.Remote = strings.Repeat("3", 32)
	next.SessionOnly = true
	ss := &ConfigSession{connection: next, oldRemote: c.Remote}
	if s.commitConfig(ss) == "" || ss.Complete || s.store.Connections[0].SessionOnly {
		t.Fatal("failed cleanup activated session-only credentials")
	}
	if !reflect.DeepEqual(s.store.Connections, []Connection{c}) {
		t.Fatal("failed conversion changed original profile")
	}
}

func TestAuthorizationBrowserFailureIsExplicitAndRedacted(t *testing.T) {
	rpc := &fakeRPC{handle: func(ctx context.Context, method string, _ any) (any, error) {
		if method == "config/oauthstatus" {
			return map[string]string{"status": "running", "authUrl": "http://127.0.0.1:53682/auth?state=synthetic-private-state"}, nil
		}
		if method == "config/create" {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return ConfigResult{}, nil
	}}
	s := testService(t, rpc)
	s.startConnection = func(Connection, string) (RPC, error) { return rpc, nil }
	s.open = func(string) error { return fmt.Errorf("do not expose synthetic-private-state") }
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	s.session = &ConfigSession{ID: "browser", Busy: true, cancel: cancel, connection: Connection{Provider: "drive"}}
	done := make(chan struct{})
	go func() { s.configure(ctx, "browser", rpc, "config/create", map[string]any{}); close(done) }()
	receive(t, done)
	ss, err := s.ConfigStatus("browser")
	if err != nil || ss.Error != "Could not launch the selected browser." || ss.Busy {
		t.Fatal("browser failure hidden or not sanitized")
	}
}

func TestSlowEngineDoesNotInvalidateOtherDriveSnapshot(t *testing.T) {
	s := testService(t, &fakeRPC{})
	c := testConnection()
	other := c
	other.ID = strings.Repeat("3", 32)
	other.Remote = strings.Repeat("4", 32)
	other.Letter = "T"
	s.store.Connections = []Connection{c, other}
	s.mounted[c.ID] = true
	s.mounted[other.ID] = true
	s.driveUsed = func(string) bool { return true }
	slow := &fakeRPC{handle: func(ctx context.Context, method string, _ any) (any, error) {
		if method == "mount/listmounts" {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return map[string]any{"diskCache": cleanCacheStats()}, nil
	}}
	healthy := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
		if method == "mount/listmounts" {
			return map[string]any{"mountPoints": []engineMount{{Fs: remote(other), MountPoint: "T:"}}}, nil
		}
		return map[string]any{"diskCache": cleanCacheStats()}, nil
	}}
	s.workers = map[string]RPC{c.Remote: slow, other.Remote: healthy}
	SetConnectionEngine(s, func(Connection, string) (RPC, error) { return nil, fmt.Errorf("unexpected startup") })
	view := s.Snapshot()
	if view.Connections[0].Pending != nil {
		t.Fatal("slow worker reported clean")
	}
	if view.Connections[1].Pending == nil || !view.Connections[1].Mounted || view.Connections[1].Error != "" {
		t.Fatalf("healthy worker invalidated: %+v", view.Connections[1])
	}
}

func TestStoppedConnectionEnginePreservesRecoveryAndAllowsReauthentication(t *testing.T) {
	s := testService(t, &fakeRPC{})
	c := testConnection()
	s.store.Connections = []Connection{c}
	s.mounted[c.ID] = true
	worker := &switchableRPC{}
	worker.stopped.Store(true)
	s.workers = map[string]RPC{c.Remote: worker}
	SetConnectionEngine(s, func(Connection, string) (RPC, error) { return &fakeRPC{}, nil })
	view := s.Snapshot()
	if view.Connections[0].Mounted || !view.Connections[0].Recovery || s.mounted[c.ID] {
		t.Fatal("dead worker left an unrecoverable mounted state")
	}
	if s.store.Connections[0] != c {
		t.Fatal("dead worker changed recovery destination")
	}
}
