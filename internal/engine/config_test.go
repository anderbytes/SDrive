package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"sdrive/internal/platform"
)

func TestMain(m *testing.M) {
	if processProbeCommand() {
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "rcd" && os.Getenv("SDRIVE_DIAGNOSTIC_HELPER") != "" {
		diagnosticProcessHelper()
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "--config-key" {
		key, err := platform.Key(os.Args[2])
		if err != nil {
			os.Exit(1)
		}
		fmt.Print(key)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestConfigCancellationWaitsForServerCompletion(t *testing.T) {
	for _, lostResponse := range []bool{false, true} {
		t.Run(fmt.Sprintf("lost-start-response=%v", lostResponse), func(t *testing.T) {
			started, stopped, finish := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			var group string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/config/create":
					var params map[string]any
					_ = json.NewDecoder(r.Body).Decode(&params)
					if params["_async"] != true {
						t.Error("configuration is not tracked as a job")
					}
					group, _ = params["_group"].(string)
					close(started)
					if lostResponse {
						conn, _, _ := w.(http.Hijacker).Hijack()
						_ = conn.Close()
						return
					}
					_, _ = w.Write([]byte(`{"jobid":7}`))
				case "/job/list":
					_, _ = w.Write([]byte(`{"jobids":[7]}`))
				case "/job/stop":
					once.Do(func() { close(stopped) })
					_, _ = w.Write([]byte(`{}`))
				case "/job/status":
					<-started
					finished := false
					select {
					case <-finish:
						finished = true
					default:
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"group": group, "finished": finished, "success": true, "output": map[string]any{}})
				default:
					t.Errorf("unexpected call %s", r.URL.Path)
				}
			}))
			defer server.Close()
			e := &Engine{url: server.URL, done: make(chan struct{}), client: server.Client()}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() { result <- e.Call(ctx, "config/create", map[string]any{}, &map[string]any{}) }()
			<-started
			cancel()
			select {
			case <-stopped:
			case <-time.After(3 * time.Second):
				t.Fatal("server cancellation was not requested")
			}
			select {
			case err := <-result:
				t.Fatalf("returned before server stopped: %v", err)
			default:
			}
			close(finish)
			select {
			case err := <-result:
				if err != context.Canceled {
					t.Fatalf("cancellation: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("did not finish")
			}
		})
	}
}

func TestConfigUnknownCompletionWaitsForEngineTermination(t *testing.T) {
	statusRead := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/config/create" {
			_, _ = w.Write([]byte(`{"jobid":1}`))
			return
		}
		once.Do(func() { close(statusRead) })
		_, _ = w.Write([]byte(`null`))
	}))
	defer server.Close()
	e := &Engine{url: server.URL, done: make(chan struct{}), client: server.Client()}
	result := make(chan error, 1)
	go func() { result <- e.Call(context.Background(), "config/create", map[string]any{}, nil) }()
	<-statusRead
	select {
	case err := <-result:
		t.Fatalf("unknown completion accepted: %v", err)
	default:
	}
	close(e.done)
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("stopped engine accepted")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("engine termination ignored")
	}
}

func TestConfigurationJobProtocolWithRclone(t *testing.T) {
	if os.Getenv("SDRIVE_TEST_ENGINE") != "1" {
		t.Skip("set SDRIVE_TEST_ENGINE=1 for disposable real-engine checks")
	}
	binary, err := filepath.Abs(filepath.Join("..", "..", ".tools", "rclone", binaryName()))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if _, err := platform.Key(dir); err != nil {
		t.Fatal(err)
	}
	if err := Init(binary, dir); err != nil {
		t.Fatal(err)
	}
	e, err := Start(binary, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var result map[string]any
	if err := e.Call(ctx, "config/create", map[string]any{"name": "sdrive-disposable", "type": "local", "parameters": map[string]string{}, "opt": map[string]bool{"nonInteractive": true}}, &result); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"State", "Option", "Error"} {
		if _, ok := result[key]; !ok {
			t.Fatalf("real completion is missing %s", key)
		}
	}
	if result["State"] != "" || result["Option"] != nil || result["Error"] != "" {
		t.Fatal("unexpected real completion shape")
	}
	// FTP configuration has no provider callback: this checks persistence without contacting a server.
	for _, mode := range []string{"explicit", "implicit", "plain"} {
		params := map[string]string{"host": "example.invalid", "user": "synthetic-user", "pass": "synthetic-password", "tls": "false", "explicit_tls": "false"}
		if mode == "explicit" {
			params["explicit_tls"] = "true"
		}
		if mode == "implicit" {
			params["tls"], params["port"] = "true", "990"
		}
		name := "sdrive-disposable-ftp-" + mode
		if err := e.Call(ctx, "config/create", map[string]any{"name": name, "type": "ftp", "parameters": params, "opt": map[string]bool{"nonInteractive": true, "obscure": true}}, &result); err != nil {
			t.Fatal(err)
		}
		var stored map[string]string
		if err := e.Call(ctx, "config/get", map[string]string{"name": name}, &stored); err != nil {
			t.Fatal(err)
		}
		if stored["type"] != "ftp" || stored["tls"] != params["tls"] || stored["explicit_tls"] != params["explicit_tls"] || stored["pass"] == params["pass"] {
			t.Fatal("real FTP persistence differs from expected transport or obscuring")
		}
		if err := e.Call(ctx, "config/delete", map[string]string{"name": name}, nil); err != nil {
			t.Fatal(err)
		}
	}
	var config map[string]string
	if err := e.Call(ctx, "config/get", map[string]string{"name": "sdrive-disposable"}, &config); err != nil || config["type"] != "local" {
		t.Fatalf("real configuration: %v", err)
	}
	// Cancel after the server has accepted the write, before the client can poll it.
	e.client.Transport = cancelConfigTransport{base: e.client.Transport, cancel: cancel}
	err = e.Call(ctx, "config/update", map[string]any{"name": "sdrive-disposable", "parameters": map[string]string{}, "opt": map[string]bool{"nonInteractive": true}}, &result)
	if err != context.Canceled {
		t.Fatalf("real cancellation: %v", err)
	}
	cleanupCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	var jobs struct {
		IDs []int64 `json:"jobids"`
	}
	if err := e.Call(cleanupCtx, "job/list", map[string]any{}, &jobs); err != nil {
		t.Fatal(err)
	}
	for _, id := range jobs.IDs {
		var status struct {
			Group    string
			Finished bool
		}
		if err := e.Call(cleanupCtx, "job/status", map[string]any{"jobid": id}, &status); err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(status.Group, "sdrive-config-") && !status.Finished {
			t.Fatal("configuration returned with a live job")
		}
	}
	if err := e.Call(cleanupCtx, "config/delete", map[string]string{"name": "sdrive-disposable"}, nil); err != nil {
		t.Fatal(err)
	}
	var remotes struct{ Remotes []string }
	if err := e.Call(cleanupCtx, "config/listremotes", map[string]any{}, &remotes); err != nil || len(remotes.Remotes) != 0 {
		t.Fatalf("cleanup: %v", err)
	}
}

type cancelConfigTransport struct {
	base   http.RoundTripper
	cancel context.CancelFunc
}

func (c cancelConfigTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := c.base.RoundTrip(r)
	if r.URL.Path == "/config/update" {
		c.cancel()
	}
	return response, err
}

func TestConfigRejectsTerminalFailuresWithoutLeakingResponses(t *testing.T) {
	for _, body := range []string{"unauthorized", `{"finished":true,"success":false,"error":"private-password","output":{}}`, `{"finished":true,"success":true,"output":null}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/config/create" {
					if body == "unauthorized" {
						w.WriteHeader(http.StatusUnauthorized)
						_, _ = w.Write([]byte("private-password"))
						return
					}
					_, _ = w.Write([]byte(`{"jobid":1}`))
					return
				}
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			e := &Engine{url: server.URL, done: make(chan struct{}), client: server.Client()}
			err := e.Call(context.Background(), "config/create", map[string]any{}, &map[string]any{})
			if err == nil || strings.Contains(err.Error(), "private-password") {
				t.Fatalf("unsafe terminal failure: %v", err)
			}
		})
	}
}
