package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"sdrive/internal/diagnostics"
)

func diagnosticProcessHelper() {
	mode := os.Getenv("SDRIVE_DIAGNOSTIC_HELPER")
	if mode == "early" {
		fmt.Fprint(os.Stderr, `{"level":"error","msg":"invalid_grant private-token https://private/?token=secret C:\\Private"}`)
		time.Sleep(100 * time.Millisecond)
		os.Exit(17)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(19)
	}
	stop := make(chan struct{})
	var once sync.Once
	go http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := r.BasicAuth(); !ok {
			w.WriteHeader(401)
			return
		}
		w.Write([]byte(`{}`))
		if r.URL.Path == "/stop-test" || r.URL.Path == "/core/quit" && mode != "forced" {
			once.Do(func() { close(stop) })
		}
	}))
	_ = json.NewEncoder(os.Stderr).Encode(map[string]string{"msg": "Serving remote control on http://" + listener.Addr().String(), "level": "info"})
	<-stop
	if mode == "unexpected" {
		fmt.Fprint(os.Stderr, `{"level":"error","msg":"i/o timeout private-token"}`)
		os.Exit(17)
	}
	os.Exit(0)
}

func TestEngineExitAndFinalLogArePreserved(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"early", "unexpected", "requested", "forced"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("SDRIVE_DIAGNOSTIC_HELPER", mode)
			log := diagnostics.New(t.TempDir())
			defer log.Close()
			e, err := Start(binary, t.TempDir(), log)
			if mode == "early" {
				if err == nil {
					e.Close()
					t.Fatal("startup exit accepted")
				}
				if !strings.Contains(err.Error(), "exit code 17") || !strings.Contains(err.Error(), "credentials") || strings.Contains(err.Error(), "private") {
					t.Fatal(err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer e.Close()
			if mode == "unexpected" {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				_ = e.Call(ctx, "stop-test", nil, nil)
				cancel()
				select {
				case <-e.done:
				case <-time.After(5 * time.Second):
					t.Fatal("exit not observed")
				}
			} else {
				e.Close()
			}
			report := log.Report()
			if !strings.Contains(report, "engine_exit "+mode) || strings.Contains(report, "private") {
				t.Fatal(report)
			}
			if mode == "unexpected" && (!strings.Contains(e.EngineExitSummary(), "exit code 17") || !strings.Contains(e.EngineExitSummary(), "network")) {
				t.Fatal(e.EngineExitSummary())
			}
			if mode == "requested" && !strings.Contains(report, "exit=0") {
				t.Fatal("clean shutdown lost")
			}
		})
	}
}

func TestLogStreamIsBoundedDrainedAndSanitized(t *testing.T) {
	log := diagnostics.New(t.TempDir())
	defer log.Close()
	e := &Engine{diagnostics: log}
	ready := make(chan string, 1)
	w := &logWriter{e: e, ready: ready}
	for range 100 {
		_, _ = w.Write([]byte(strings.Repeat("private", 1000)))
	}
	if len(w.line) > maxLogLine || !w.discard {
		t.Fatal("oversized log retained")
	}
	_, _ = w.Write([]byte("\ninvalid private\n"))
	_, _ = w.Write([]byte(`{"level":"error","msg":"Failed to load config file C:\\Private: secret"}` + "\n"))
	for range 100 {
		_, _ = w.Write([]byte(`{"level":"error","msg":"permission denied private"}` + "\n"))
	}
	for _, address := range []string{"http://127.0.0.1:1234@private.example/", "http://127.0.0.1:1234/?token=secret", "http://127.0.0.1:1234/private", "http://127.0.0.1:999999/"} {
		data, _ := json.Marshal(map[string]string{"msg": "Serving remote control on " + address})
		_, _ = w.Write(append(data, '\n'))
	}
	if len(ready) != 0 {
		t.Fatal("accepted an unsafe readiness address")
	}
	_, _ = w.Write([]byte(`{"msg":"Serving remote control on http://127.0.0.1:1234/"}` + "\n"))
	_, _ = w.Write([]byte(`{"level":"error","msg":"invalid_grant secret"}`))
	w.finish()
	if len(ready) != 1 || <-ready != "http://127.0.0.1:1234" {
		t.Fatal("oversized log blocked readiness")
	}
	report := log.Report()
	for _, kind := range []string{"log_oversized", "log_invalid", "configuration", "permission", "credentials"} {
		if !strings.Contains(report, kind) {
			t.Fatalf("missing %s", kind)
		}
	}
	if strings.Count(report, "engine_log signal permission") != 1 || strings.Contains(report, "private") || strings.Contains(report, "secret") {
		t.Fatal(report)
	}
}
