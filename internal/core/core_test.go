package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func cleanCacheStats() map[string]any {
	return map[string]any{"uploadsInProgress": 0, "uploadsQueued": 0, "erroredFiles": 0, "files": 0, "bytesUsed": 0, "outOfSpace": false}
}

type fakeRPC struct {
	mu     sync.Mutex
	calls  []string
	handle func(context.Context, string, any) (any, error)
}

func (f *fakeRPC) Alive() bool { return true }
func (f *fakeRPC) Call(ctx context.Context, method string, input, out any) error {
	f.mu.Lock()
	f.calls = append(f.calls, method)
	f.mu.Unlock()
	var result any = map[string]any{}
	var err error
	if method == "config/listremotes" {
		result = map[string]any{"remotes": []string{}}
	}
	if method == "vfs/stats" {
		result = map[string]any{"diskCache": cleanCacheStats()}
	}
	if f.handle != nil {
		result, err = f.handle(ctx, method, input)
	}
	if err != nil {
		return err
	}
	if out != nil {
		b, _ := json.Marshal(result)
		return json.Unmarshal(b, out)
	}
	return nil
}
func fixture(p Provider) map[string]string {
	values := map[string]string{}
	for _, f := range p.Fields {
		if f.Default != "" {
			values[f.Key] = f.Default
		}
		if f.Required {
			values[f.Key] = "example"
		}
	}
	if p.Backend == "webdav" {
		values["url"] = "https://storage.example/dav"
	}
	if p.ID == "azureblob" {
		values["key"] = "fake-secret"
	}
	return values
}
func testService(t *testing.T, rpc RPC) *Service {
	t.Helper()
	s, err := New(t.TempDir(), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	s.rpc = rpc
	return s
}
func testConnection() Connection {
	return Connection{ID: strings.Repeat("1", 32), Remote: strings.Repeat("2", 32), Name: "Teste", Provider: "ftp", Letter: "S", Path: "", Recovery: true}
}
func waitSession(t *testing.T, s *Service, id string) ConfigSession {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ss, err := s.ConfigStatus(id)
		if err != nil {
			t.Fatal(err)
		}
		if !ss.Busy {
			return ss
		}
		time.Sleep(time.Millisecond * 5)
	}
	t.Fatal("setup did not return")
	return ConfigSession{}
}

func TestAllEighteenProfiles(t *testing.T) {
	if len(Catalog()) != 18 {
		t.Fatal("catalog must contain 18 profiles")
	}
	backends := map[string]bool{}
	ids := map[string]bool{}
	for _, p := range Catalog() {
		t.Run(p.ID, func(t *testing.T) {
			if ids[p.ID] {
				t.Fatal("duplicate ID")
			}
			ids[p.ID] = true
			backends[p.Backend] = true
			params, err := parameters(p, fixture(p), false, "explicit")
			if err != nil {
				t.Fatal(err)
			}
			if p.Backend == "webdav" && params["vendor"] == "" {
				t.Fatal("missing WebDAV preset")
			}
			rpc := &fakeRPC{handle: func(_ context.Context, method string, input any) (any, error) {
				if method == "config/create" {
					b, _ := json.Marshal(input)
					var request struct {
						Type       string
						Parameters map[string]string
						Opt        struct{ NonInteractive, Obscure bool }
					}
					if err := json.Unmarshal(b, &request); err != nil {
						t.Errorf("invalid config request: %v", err)
					}
					if request.Type != p.Backend {
						t.Errorf("incorrect backend: %s", request.Type)
					}
					for key, want := range params {
						if request.Parameters[key] != want {
							t.Errorf("configuration field %s was not forwarded", key)
						}
					}
					if !request.Opt.NonInteractive || !request.Opt.Obscure || request.Parameters["config_is_local"] != "true" {
						t.Error("setup must use local non-interactive configuration and obscure passwords")
					}
					if p.ID == "drive" && request.Parameters["scope"] != "drive" {
						t.Error("Google Drive write scope not requested")
					}
					if p.ID == "sftp" && request.Parameters["host_keys"] == "" {
						t.Error("verified SSH host key not forwarded")
					}
					return ConfigResult{}, nil
				}
				return map[string]any{}, nil
			}}
			s := testService(t, rpc)
			input := ConfigInput{Connection: Connection{Name: "Teste", Provider: p.ID, Letter: "S"}, Values: fixture(p), Transport: "explicit"}
			if p.ID == "sftp" {
				input.HostKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFSUafnJMyfUeN3l4gveT1G3bqpA9bwp2aGVkEL80q9X"
			}
			id, err := s.BeginConfig(input)
			if err != nil {
				t.Fatal(err)
			}
			ss := waitSession(t, s, id)
			if !ss.Complete {
				t.Fatalf("not completed: %s", ss.Error)
			}
			if len(s.store.Connections) != 1 {
				t.Fatal("connection not persisted")
			}
			if err = s.EndConfig(id); err != nil {
				t.Fatal(err)
			}
		})
	}
	if len(backends) != 15 {
		t.Fatalf("expected 15 backends, got %d", len(backends))
	}
}

func TestTrustBoundaries(t *testing.T) {
	p, _ := provider("webdav")
	for _, values := range []map[string]string{{"url": "http://example.test/dav"}, {"url": "https://user:secret@example.test/dav"}, {"url": "https://example.test", "command": "calc.exe"}, {"url": "https://example.test\nsecret"}} {
		if _, err := parameters(p, values, false, ""); err == nil {
			t.Fatalf("accepted unsafe input: %v", values)
		}
	}
	if _, err := parameters(p, map[string]string{"url": "http://example.test/dav"}, true, ""); err != nil {
		t.Fatal(err)
	}
	p, _ = provider("ftp")
	if _, err := parameters(p, fixture(p), false, "plain"); err == nil {
		t.Fatal("insecure FTP accepted without confirmation")
	}
	c := testConnection()
	for _, path := range []string{"../bad", "/root", "x:\\bad", "a/../b"} {
		c.Path = path
		if validateConnection(&c) == nil {
			t.Errorf("accepted path: %s", path)
		}
	}
}

func TestWizardSecretsAndContinuation(t *testing.T) {
	rpc := &fakeRPC{handle: func(_ context.Context, method string, input any) (any, error) {
		if method == "config/create" {
			return ConfigResult{State: "sensitive-state", Option: &Question{Name: "password", IsPassword: true, DefaultStr: "do-not-return"}}, nil
		}
		return ConfigResult{}, nil
	}}
	s := testService(t, rpc)
	id, err := s.BeginConfig(ConfigInput{Connection: Connection{Name: "Nuvem", Provider: "dropbox", Letter: "S"}})
	if err != nil {
		t.Fatal(err)
	}
	ss := waitSession(t, s, id)
	if ss.Question == nil || ss.Question.DefaultStr != "" {
		t.Fatal("password exposed")
	}
	encoded, _ := json.Marshal(ss)
	if strings.Contains(string(encoded), "sensitive-state") {
		t.Fatal("internal state exposed")
	}
	if err = s.AnswerConfig(id, "private-answer"); err != nil {
		t.Fatal(err)
	}
	ss = waitSession(t, s, id)
	if !ss.Complete {
		t.Fatal(ss.Error)
	}
	disk, _ := os.ReadFile(filepath.Join(s.dir, "settings.json"))
	if strings.Contains(string(disk), "private-answer") {
		t.Fatal("secret saved in settings")
	}
}

func TestCancelNeverDeletesOriginalConnection(t *testing.T) {
	rpc := &fakeRPC{handle: func(ctx context.Context, method string, input any) (any, error) {
		if method == "config/create" {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return map[string]any{}, nil
	}}
	s := testService(t, rpc)
	c := testConnection()
	c.Recovery = false
	s.store.Connections = []Connection{c}
	id, err := s.BeginConfig(ConfigInput{Connection: c, Values: map[string]string{"host": "example.test", "user": "user"}, Transport: "explicit"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.EndConfig(id); err != nil {
		t.Fatal(err)
	}
	if s.store.Connections[0].Remote != c.Remote {
		t.Fatal("original destination changed")
	}
}

func TestRecoveryPreventsDestinationChangesAndRemoval(t *testing.T) {
	s := testService(t, &fakeRPC{})
	c := testConnection()
	s.store.Connections = []Connection{c}
	if s.Remove(c.ID) == nil {
		t.Fatal("removal allowed during recovery")
	}
	c.Path = "another"
	if s.UpdateConnection(c) == nil {
		t.Fatal("destination change allowed during recovery")
	}
	if _, err := s.BeginConfig(ConfigInput{Connection: c, Values: map[string]string{"host": "example", "user": "user"}, Transport: "explicit"}); err == nil {
		t.Fatal("destination reconfiguration allowed")
	}
	quit := false
	s.quit = func() { quit = true }
	if s.Exit() == nil || quit {
		t.Fatal("exit ignored pending recovery")
	}
}

func TestUnmountChecksDurableDirtyFiles(t *testing.T) {
	rpc := &fakeRPC{handle: func(_ context.Context, method string, input any) (any, error) {
		if method == "vfs/stats" {
			return map[string]any{"diskCache": map[string]any{"uploadsInProgress": 0, "uploadsQueued": 0, "erroredFiles": 0, "files": 0, "bytesUsed": 0, "outOfSpace": true}}, nil
		}
		return map[string]any{}, nil
	}}
	s := testService(t, rpc)
	c := testConnection()
	s.store.Connections = []Connection{c}
	s.mounted[c.ID] = true
	if s.Unmount(c.ID) == nil {
		t.Fatal("unmounted without space or confirming uploads")
	}
	if !s.mounted[c.ID] || !s.store.Connections[0].Recovery {
		t.Fatal("recovery state lost")
	}
	root := filepath.Join(s.dataPath(), "vfsMeta", "sd_"+c.Remote)
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file"), []byte(`{"Dirty":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	n, err := s.cachedPending(context.Background(), c)
	if err != nil || n != 1 {
		t.Fatalf("pending files: %d %v", n, err)
	}
}

func TestUnmountRetainsRecoveryOnLastMomentWrite(t *testing.T) {
	s := testService(t, nil)
	c := testConnection()
	s.store.Connections = []Connection{c}
	s.mounted[c.ID] = true
	s.rpc = &fakeRPC{handle: func(_ context.Context, method string, input any) (any, error) {
		if method == "mount/unmount" {
			root := filepath.Join(s.dataPath(), "vfsMeta", "sd_"+c.Remote)
			_ = os.MkdirAll(root, 0700)
			_ = os.WriteFile(filepath.Join(root, "file"), []byte(`{"Dirty":true}`), 0600)
		}
		return map[string]any{"diskCache": cleanCacheStats()}, nil
	}}
	if s.Unmount(c.ID) == nil {
		t.Fatal("final pending file not detected")
	}
	if s.mounted[c.ID] || !s.store.Connections[0].Recovery {
		t.Fatal("incorrect final state")
	}
}

func TestCleanUnmountAndExit(t *testing.T) {
	s := testService(t, &fakeRPC{})
	c := testConnection()
	s.store.Connections = []Connection{c}
	s.mounted[c.ID] = true
	quit := false
	s.quit = func() { quit = true }
	if err := s.Exit(); err != nil {
		t.Fatal(err)
	}
	if !quit || s.store.Connections[0].Recovery {
		t.Fatal("clean exit not completed")
	}
}

func TestLoadCorruptStoreDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	_ = os.WriteFile(path, []byte("broken"), 0600)
	if _, err := loadStore(dir); err == nil {
		t.Fatal("accepted corrupted settings")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "broken" {
		t.Fatal("original file changed")
	}
}
func TestRejectedProviderErrorsDoNotExposeSecrets(t *testing.T) {
	s := testService(t, &fakeRPC{handle: func(context.Context, string, any) (any, error) {
		return ConfigResult{Error: "password=private-value"}, nil
	}})
	id, err := s.BeginConfig(ConfigInput{Connection: Connection{Name: "Teste", Provider: "box", Letter: "S"}})
	if err != nil {
		t.Fatal(err)
	}
	ss := waitSession(t, s, id)
	if ss.Error == "" || strings.Contains(ss.Error, "private-value") {
		t.Fatal("error not sanitized")
	}
}

func TestConnectionFieldsExcludeSecrets(t *testing.T) {
	s := testService(t, &fakeRPC{handle: func(context.Context, string, any) (any, error) {
		return map[string]string{"host": "example.test", "user": "user", "pass": "secret", "token": "private-token"}, nil
	}})
	c := testConnection()
	s.store.Connections = []Connection{c}
	fields, err := s.ConnectionFields(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fields["host"] != "example.test" || fields["pass"] != "" || fields["token"] != "" {
		t.Fatal("credential fields exposed")
	}
}

func TestMissingCacheStatsBlocksSafeUnmount(t *testing.T) {
	s := testService(t, &fakeRPC{handle: func(context.Context, string, any) (any, error) { return map[string]any{}, nil }})
	c := testConnection()
	s.store.Connections = []Connection{c}
	s.mounted[c.ID] = true
	if s.Unmount(c.ID) == nil || !s.store.Connections[0].Recovery {
		t.Fatal("incomplete response declared safe")
	}
}

func TestRecoveryCredentialsKeepRemoteAndRejectNewServer(t *testing.T) {
	rpc := &fakeRPC{handle: func(_ context.Context, method string, input any) (any, error) {
		if method == "config/get" {
			return map[string]string{"host": "example.test", "user": "user", "explicit_tls": "true"}, nil
		}
		return ConfigResult{}, nil
	}}
	s := testService(t, rpc)
	c := testConnection()
	s.store.Connections = []Connection{c}
	input := ConfigInput{Connection: c, Values: map[string]string{"host": "other.test", "user": "user", "pass": "new-password"}, Transport: "explicit"}
	if _, err := s.BeginConfig(input); err == nil {
		t.Fatal("server changed during recovery")
	}
	input.Values["host"] = "example.test"
	id, err := s.BeginConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	ss := waitSession(t, s, id)
	if !ss.Complete || s.store.Connections[0].Remote != c.Remote || !s.store.Connections[0].Recovery {
		t.Fatal("renewal lost original destination or pending state")
	}
}

func TestFailedCredentialRemovalRestoresConnection(t *testing.T) {
	rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
		if method == "config/delete" {
			return nil, context.DeadlineExceeded
		}
		return map[string]any{}, nil
	}}
	s := testService(t, rpc)
	c := testConnection()
	c.Recovery = false
	s.store.Connections = []Connection{c}
	if s.Remove(c.ID) == nil || len(s.store.Connections) != 1 {
		t.Fatal("failed removal did not preserve connection")
	}
	persisted, err := loadStore(s.dir)
	if err != nil || len(persisted.Connections) != 1 {
		t.Fatal("settings not restored")
	}
}

func TestTokenQuestionsAreMaskedEvenWithoutProviderHint(t *testing.T) {
	s := testService(t, &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
		if method == "config/create" {
			return ConfigResult{State: "token", Option: &Question{Name: "config_token", Type: "string", DefaultStr: "private-token"}}, nil
		}
		return map[string]any{}, nil
	}})
	id, err := s.BeginConfig(ConfigInput{Connection: Connection{Name: "Conta", Provider: "box", Letter: "S"}})
	if err != nil {
		t.Fatal(err)
	}
	ss := waitSession(t, s, id)
	if ss.Question == nil || !ss.Question.Sensitive || ss.Question.DefaultStr != "" {
		t.Fatal("token displayed in plain text")
	}
}

func TestOrphanCleanupOnlyTouchesGeneratedUnusedRemotes(t *testing.T) {
	dir := t.TempDir()
	c := testConnection()
	c.Recovery = false
	store := Store{Connections: []Connection{c}, Settings: Settings{CacheGiB: 2}}
	if err := saveStore(dir, store); err != nil {
		t.Fatal(err)
	}
	orphan := "sd_" + strings.Repeat("3", 32)
	deleted := []string{}
	rpc := &fakeRPC{handle: func(_ context.Context, method string, input any) (any, error) {
		if method == "config/listremotes" {
			return map[string]any{"remotes": []string{"sd_" + c.Remote + ":", orphan + ":", "unrelated:", "sd_invalid:"}}, nil
		}
		if method == "config/delete" {
			deleted = append(deleted, input.(map[string]string)["name"])
		}
		return map[string]any{}, nil
	}}
	if _, err := New(dir, rpc, ""); err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0] != orphan {
		t.Fatalf("cleanup outside scope: %v", deleted)
	}
}

func TestDesktopCallbacksAreNotExposedByTheBridge(t *testing.T) {
	service := reflect.TypeOf(&Service{})
	for _, name := range []string{"SetDesktop", "AutoMount", "SetFilePicker", "SetLanguageChanged", "Language"} {
		if _, ok := service.MethodByName(name); ok {
			t.Fatalf("internal method exposed through bridge: %s", name)
		}
	}
}

func TestRecoveryRejectsChangedLibraryButAllowsRenewedTokens(t *testing.T) {
	c := Connection{Provider: "onedrive", Path: "Documents"}
	config := map[string]string{"type": "onedrive", "drive_id": "original-library", "token": "old"}
	hash, err := checkDestination(c, config)
	if err != nil {
		t.Fatal(err)
	}
	c.Recovery = true
	c.TargetHash = hash
	config["token"] = "renewed"
	if _, err = checkDestination(c, config); err != nil {
		t.Fatal("token renewal changed destination identity")
	}
	config["drive_id"] = "another-library"
	if _, err = checkDestination(c, config); err == nil {
		t.Fatal("cache could be sent to another library")
	}
}

func TestConfiguredLettersAreNotOfferedToNewConnections(t *testing.T) {
	s := testService(t, &fakeRPC{})
	before := s.Snapshot()
	if len(before.Letters) == 0 {
		t.Skip("no free drive letter")
	}
	c := testConnection()
	c.Letter = before.Letters[0]
	s.store.Connections = []Connection{c}
	for _, letter := range s.Snapshot().Letters {
		if letter == c.Letter {
			t.Fatal("reserved letter offered during setup")
		}
	}
}
