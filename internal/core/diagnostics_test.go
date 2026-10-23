package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sdrive/internal/diagnostics"
)

func TestDiagnosticReportOmitsProfileAndUntrustedVersions(t *testing.T) {
	s := testService(t, &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
		if method == "core/version" {
			return map[string]string{"version": "https://private.example/?token=secret"}, nil
		}
		return map[string]any{}, nil
	}})
	s.appVersion = "0.1.0"
	s.store.Connections = []Connection{{ID: "private-id", Remote: "private-remote", Name: "private-name", Path: "private-path", Recovery: true}}
	s.settingsError = "private settings error"
	s.engineError = "private engine error"
	log := diagnostics.New(t.TempDir())
	defer log.Close()
	SetDiagnostics(s, log)
	report := s.Diagnostics()
	if strings.Contains(report, "private") || strings.Contains(report, "secret") || !strings.Contains(report, "rclone (running): unavailable") || !strings.Contains(report, "recovery required: 1") || !strings.Contains(report, "settings unavailable: true") {
		t.Fatal(report)
	}
	s.rpc = nil
	if !strings.Contains(s.Diagnostics(), "Engine running: false") {
		t.Fatal("report requires live engine")
	}
	s.diagnostics = nil
	if !strings.Contains(s.Diagnostics(), "Local diagnostics unavailable") {
		t.Fatal("missing unavailable status")
	}
}

func TestDiagnosticOperationsDoNotDependOnStorage(t *testing.T) {
	s := testService(t, nil)
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	log := diagnostics.New(blocked)
	defer log.Close()
	SetDiagnostics(s, log)
	if err := s.Mount("private-id"); err == nil {
		t.Fatal("expected validation error")
	}
	if err := s.Unmount("private-id"); err == nil {
		t.Fatal("expected validation error")
	}
	if err := s.VerifyRecovery(); err == nil {
		t.Fatal("expected recovery guard")
	}
	if err := s.RestoreSettings(); err == nil {
		t.Fatal("expected restoration guard")
	}
	_, _ = s.BeginConfig(ConfigInput{})
	_ = s.EndConfig("private-session")
	s.rpc = &fakeRPC{}
	s.recoveryHold = true
	if err := os.WriteFile(filepath.Join(s.dir, recoveryMarker), []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyRecovery(); err != nil || s.recoveryHold {
		t.Fatalf("diagnostic storage blocked recovery: %v", err)
	}
	for _, stage := range []string{"mount failed", "unmount failed", "recovery failed", "recovery ok", "setup failed", "setup ok"} {
		if !strings.Contains(log.Report(), stage) {
			t.Fatalf("missing %s", stage)
		}
	}
	if strings.Contains(log.Report(), "private") {
		t.Fatal("operation arguments leaked")
	}
}

type stoppedDiagnosticRPC struct{ RPC }

func (stoppedDiagnosticRPC) Alive() bool { return false }
func (stoppedDiagnosticRPC) EngineExitSummary() string {
	return "Engine stopped (exit code 17; last signal: network). Restart SDrive to recover your drives."
}

func TestSnapshotPreservesEngineExitContext(t *testing.T) {
	s := testService(t, stoppedDiagnosticRPC{})
	if got := s.Snapshot().EngineError; !strings.Contains(got, "exit code 17") {
		t.Fatalf("lost exit context: %s", got)
	}
}
