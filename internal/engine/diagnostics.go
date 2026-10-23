package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"

	"sdrive/internal/diagnostics"
)

// exec drains stderr before Wait returns. The writer never retains a whole stream.
type logWriter struct {
	e       *Engine
	ready   chan<- string
	line    []byte
	discard bool
}

const maxLogLine = 64 << 10

func (w *logWriter) Write(data []byte) (int, error) {
	n := len(data)
	for len(data) > 0 {
		i := bytes.IndexByte(data, '\n')
		part := data
		if i >= 0 {
			part = data[:i]
		}
		if !w.discard {
			if len(w.line)+len(part) > maxLogLine {
				w.line = nil
				w.discard = true
				w.e.logSignal("log_oversized")
			} else {
				w.line = append(w.line, part...)
			}
		}
		if i < 0 {
			break
		}
		if !w.discard {
			w.record(w.line)
		}
		w.line = w.line[:0]
		w.discard = false
		data = data[i+1:]
	}
	return n, nil
}

func (w *logWriter) finish() {
	if len(w.line) > 0 && !w.discard {
		w.record(w.line)
	}
	w.line = nil
}

func (w *logWriter) record(data []byte) {
	var record struct {
		Msg   string `json:"msg"`
		Level string `json:"level"`
	}
	if json.Unmarshal(data, &record) != nil {
		w.e.logSignal("log_invalid")
		return
	}
	const marker = "Serving remote control on "
	if strings.HasPrefix(record.Msg, marker) {
		address := strings.TrimSpace(strings.TrimPrefix(record.Msg, marker))
		if u, err := url.Parse(address); err == nil {
			port, _ := strconv.Atoi(u.Port())
			if u.Scheme == "http" && u.Hostname() == "127.0.0.1" && port > 0 && port <= 65535 && u.User == nil && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" && (u.Path == "" || u.Path == "/") {
				select {
				case w.ready <- "http://" + u.Host:
				default:
				}
			}
		}
		return
	}
	if strings.EqualFold(record.Level, "error") || strings.EqualFold(record.Level, "critical") || strings.EqualFold(record.Level, "fatal") {
		kind := diagnostics.Kind(providerFailure(record.Msg))
		if strings.Contains(strings.ToLower(record.Msg), "failed to load config file") {
			kind = "configuration"
		}
		w.e.logSignal(kind)
	}
}

func (e *Engine) logSignal(kind string) {
	e.statusMu.Lock()
	changed := e.lastSignal != kind
	e.lastSignal = kind
	e.statusMu.Unlock()
	if changed {
		e.diagnostics.Record("engine_log", "signal", kind, nil)
	}
}

func (e *Engine) wait(cmd *exec.Cmd, logs *logWriter) {
	err := cmd.Wait()
	logs.finish()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		e.logSignal("log_read")
	}
	code := int64(-1)
	if cmd.ProcessState != nil {
		code = int64(cmd.ProcessState.ExitCode())
	}
	e.statusMu.Lock()
	e.exitCode = &code
	result := "unexpected"
	if e.requested {
		result = "requested"
	}
	if e.forced {
		result = "forced"
	}
	kind := e.lastSignal
	if kind == "" {
		kind = "unknown"
	}
	e.statusMu.Unlock()
	e.diagnostics.Record("engine_exit", result, kind, &code)
	close(e.done)
}

func (e *Engine) supervisionFailure(cmd *exec.Cmd, logs *logWriter) error {
	e.forced = cmd.Process.Kill() == nil
	e.wait(cmd, logs)
	e.diagnostics.Record("engine_start", "failed", "engine", nil)
	return fmt.Errorf("%s", e.exitSummary("could not supervise the rclone process"))
}

func (e *Engine) EngineExitSummary() string {
	return e.exitSummary("Engine stopped")
}

func (e *Engine) exitSummary(stage string) string {
	e.statusMu.Lock()
	defer e.statusMu.Unlock()
	if e.exitCode == nil {
		return ""
	}
	kind := e.lastSignal
	if kind == "" {
		kind = "unknown"
	}
	return fmt.Sprintf("%s (exit code %d; last signal: %s). Restart SDrive to recover your drives.", stage, *e.exitCode, kind)
}
