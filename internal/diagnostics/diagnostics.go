// Package diagnostics stores only allowlisted technical events, never raw errors.
package diagnostics

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxFile = 128 << 10
const maxEvents = 256

type Event struct {
	Time     string `json:"time"`
	Stage    string `json:"stage"`
	Result   string `json:"result"`
	Kind     string `json:"kind"`
	ExitCode *int64 `json:"exitCode,omitempty"`
}

type Collector struct {
	mu          sync.Mutex
	events      []Event
	queue       chan Event
	done        chan struct{}
	closed      bool
	unavailable bool
	dropped     uint64
	discarded   bool
}

func allowed(value, list string) bool {
	return strings.Contains("|"+list+"|", "|"+value+"|") && !strings.Contains(value, "|")
}

func valid(e Event) bool {
	t, err := time.Parse(time.RFC3339Nano, e.Time)
	return err == nil && t.UTC().Format(time.RFC3339Nano) == e.Time &&
		allowed(e.Stage, "application|profile|engine_find|key|engine_init|engine_start|engine_auth|engine_log|engine_exit|engine_rpc|setup|mount|unmount|cache|recovery|exit") &&
		allowed(e.Result, "started|ok|failed|requested|forced|unexpected|signal") &&
		allowed(e.Kind, "none|unknown|network|credentials|permission|engine|invalid|configuration|log_read|log_invalid|log_oversized") &&
		(e.ExitCode == nil || (e.Stage == "engine_exit" && *e.ExitCode >= -1 && *e.ExitCode <= 1<<32-1))
}

// Kind deliberately ignores Error(): arbitrary errors often contain private paths.
func Kind(err error) string {
	if err == nil {
		return "none"
	}
	var failure interface{ FailureKind() string }
	if errors.As(err, &failure) && allowed(failure.FailureKind(), "network|credentials|permission|engine|invalid|configuration") {
		return failure.FailureKind()
	}
	return "unknown"
}

func New(dir string) *Collector {
	c := &Collector{queue: make(chan Event, 64), done: make(chan struct{})}
	for _, name := range []string{"diagnostics.previous.jsonl", "diagnostics.jsonl"} {
		c.load(filepath.Join(dir, name))
	}
	go c.write(dir)
	return c
}

func (c *Collector) load(path string) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		c.unavailable = true
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxFile+1))
	if err != nil {
		c.unavailable = true
		return
	}
	if len(data) > maxFile {
		c.discarded = true
		return
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 1024), 1024)
	for scanner.Scan() {
		var e Event
		d := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		d.DisallowUnknownFields()
		if d.Decode(&e) != nil || !valid(e) || d.Decode(new(any)) != io.EOF {
			c.discarded = true
			continue
		}
		c.retain(e)
	}
	if scanner.Err() != nil {
		c.discarded = true
	}
}

func (c *Collector) retain(e Event) {
	c.events = append(c.events, e)
	if len(c.events) > maxEvents {
		c.events = c.events[len(c.events)-maxEvents:]
	}
}

func (c *Collector) Record(stage, result, kind string, code *int64) {
	if c == nil {
		return
	}
	e := Event{Time: time.Now().UTC().Format(time.RFC3339Nano), Stage: stage, Result: result, Kind: kind}
	if code != nil {
		v := *code
		e.ExitCode = &v
	}
	if !valid(e) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.retain(e)
	select {
	case c.queue <- e:
	default:
		c.dropped++
	}
}

func (c *Collector) Operation(stage string, err error) {
	result := "ok"
	if err != nil {
		result = "failed"
	}
	c.Record(stage, result, Kind(err), nil)
}

func (c *Collector) write(dir string) {
	defer close(c.done)
	path, previous := filepath.Join(dir, "diagnostics.jsonl"), filepath.Join(dir, "diagnostics.previous.jsonl")
	// Only these two diagnostic files may be removed; credentials and cache are never touched.
	for _, name := range []string{previous, path} {
		if info, err := os.Stat(name); err == nil && info.Size() > maxFile {
			if os.Remove(name) != nil {
				c.failed()
			}
		}
	}
	for e := range c.queue {
		data, _ := json.Marshal(e)
		data = append(data, '\n')
		if err := appendEvent(path, previous, data); err != nil {
			c.failed()
		}
	}
}

func appendEvent(path, previous string, data []byte) error {
	if info, err := os.Stat(path); err == nil && info.Size()+int64(len(data)) > maxFile {
		if err = os.Remove(previous); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err = os.Rename(path, previous); err != nil {
			return err
		}
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	n, err := f.Write(data)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return closeErr
}

func (c *Collector) failed() { c.mu.Lock(); c.unavailable = true; c.mu.Unlock() }

func (c *Collector) Report() string {
	if c == nil {
		return "Local diagnostics unavailable.\n"
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var b strings.Builder
	fmt.Fprintf(&b, "Local diagnostics: last %d events; retention: 256 in memory, 2 x 128 KiB on disk.\n", len(c.events))
	fmt.Fprintf(&b, "Storage errors: %t; dropped persistence records: %d; discarded invalid history: %t.\n", c.unavailable, c.dropped, c.discarded)
	for _, e := range c.events {
		fmt.Fprintf(&b, "%s %s %s %s", e.Time, e.Stage, e.Result, e.Kind)
		if e.ExitCode != nil {
			fmt.Fprintf(&b, " exit=%d", *e.ExitCode)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func (c *Collector) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		close(c.queue)
	}
	c.mu.Unlock()
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-c.done:
	case <-timer.C:
	}
}

// Version accepts numeric release/build identifiers, never arbitrary component text.
func Version(value string) string {
	if len(value) == 0 || len(value) > 64 {
		return "unavailable"
	}
	for i, r := range value {
		if r >= '0' && r <= '9' || r == '.' || i == 0 && r == 'v' {
			continue
		}
		return "unavailable"
	}
	parts := strings.Split(strings.TrimPrefix(value, "v"), ".")
	if len(parts) < 3 || len(parts) > 4 {
		return "unavailable"
	}
	for _, part := range parts {
		if _, err := strconv.ParseUint(part, 10, 32); err != nil {
			return "unavailable"
		}
	}
	return value
}
