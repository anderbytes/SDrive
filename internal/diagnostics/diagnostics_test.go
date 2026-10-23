package diagnostics

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPrivateInputNeverReachesHistoryOrReport(t *testing.T) {
	dir := t.TempDir()
	c := New(dir)
	secret := "https://user:password@private.example/a?token=private-token\nC:\\Users\\Private\\file.txt"
	c.Operation("setup", errors.New(secret))
	c.Record(secret, "failed", "unknown", nil)
	c.Record("setup", secret, "unknown", nil)
	c.Record("setup", "failed", secret, nil)
	c.Close()
	for _, value := range []string{c.Report(), read(t, filepath.Join(dir, "diagnostics.jsonl"))} {
		if !strings.Contains(value, "unknown") {
			t.Fatal("missing generic failure")
		}
		for _, private := range []string{"private", "password", "token", "Users"} {
			if strings.Contains(value, private) {
				t.Fatalf("private data crossed boundary: %s", private)
			}
		}
	}
}

func TestHistoryRevalidatesEveryFieldAndBoundsReads(t *testing.T) {
	dir := t.TempDir()
	e := Event{Time: time.Now().UTC().Format(time.RFC3339Nano), Stage: "setup", Result: "failed", Kind: "unknown"}
	data, _ := json.Marshal(e)
	bad := strings.Replace(string(data), "unknown", "secret-token", 1)
	unknownField := strings.TrimSuffix(string(data), "}") + `,"secret":"private"}`
	path := filepath.Join(dir, "diagnostics.jsonl")
	if err := os.WriteFile(path, []byte(string(data)+"\n"+bad+"\n"+unknownField+"\n"+string(data)+" {}\n"+"broken\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c := New(dir)
	c.Close()
	report := c.Report()
	if !strings.Contains(report, "last 1 events") || !strings.Contains(report, "discarded invalid history: true") || strings.Contains(report, "private") || strings.Contains(report, "secret-token") {
		t.Fatal(report)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("private", maxFile)), 0600); err != nil {
		t.Fatal(err)
	}
	c = New(dir)
	c.Record("application", "started", "none", nil)
	c.Close()
	if !strings.Contains(c.Report(), "discarded invalid history: true") {
		t.Fatal("oversized history was trusted")
	}
	if info, err := os.Stat(path); err != nil || info.Size() > maxFile {
		t.Fatalf("oversized history retained: %v", err)
	}
}

func TestRotationAndMemoryRetention(t *testing.T) {
	dir := t.TempDir()
	path, previous := filepath.Join(dir, "diagnostics.jsonl"), filepath.Join(dir, "diagnostics.previous.jsonl")
	e := Event{Time: time.Now().UTC().Format(time.RFC3339Nano), Stage: "mount", Result: "ok", Kind: "none"}
	data, _ := json.Marshal(e)
	for range 4000 {
		if err := appendEvent(path, previous, append(data, '\n')); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{path, previous} {
		if info, err := os.Stat(path); err != nil || info.Size() > maxFile {
			t.Fatalf("rotation bound: %v", err)
		}
	}
	c := New(dir)
	c.Close()
	if !strings.Contains(c.Report(), "last 256 events") {
		t.Fatal("history memory limit")
	}
}

func TestNonblockingQueueConcurrencyAndUnavailableStorage(t *testing.T) {
	c := &Collector{queue: make(chan Event, 1), done: make(chan struct{})}
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 100 {
				c.Operation("recovery", errors.New("private"))
				_ = c.Report()
			}
		})
	}
	wg.Wait()
	if len(c.events) != maxEvents || c.dropped != 999 {
		t.Fatal("unbounded queue or memory")
	}
	close(c.done)
	c.Close()
	c.Close()
	c.Operation("exit", nil)
	if len(c.events) != maxEvents {
		t.Fatal("recorded after closure")
	}
	c = New(filepath.Join(t.TempDir(), "missing"))
	c.Operation("recovery", nil)
	c.Close()
	if !strings.Contains(c.Report(), "Storage errors: true") || !strings.Contains(c.Report(), "recovery ok") {
		t.Fatal(c.Report())
	}
}

func TestShutdownIsBounded(t *testing.T) {
	c := &Collector{queue: make(chan Event), done: make(chan struct{})}
	start := time.Now()
	c.Close()
	if time.Since(start) > 2*time.Second {
		t.Fatal("shutdown blocked on storage")
	}
}

func TestVersionAllowlist(t *testing.T) {
	for _, value := range []string{"", "C:\\Private", "https://secret", "1.2\npassword", strings.Repeat("1", 65), "v...", "123456", "1..3", "1.2.3.4.5", "1.2.99999999999"} {
		if Version(value) != "unavailable" {
			t.Fatal("unsafe version")
		}
	}
	for _, value := range []string{"v1.75.2", "0.1.0", "10.0.26100", "2.1.25156.0"} {
		if Version(value) != value {
			t.Fatal("valid version rejected")
		}
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
