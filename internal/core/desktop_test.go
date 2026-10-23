package core

import (
	"context"
	"fmt"
	"testing"
)

func TestAboutUsesRunningEngineAndHandlesUnavailableVersions(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		rpc := &fakeRPC{handle: func(ctx context.Context, method string, _ any) (any, error) {
			if method == "core/version" {
				if _, ok := ctx.Deadline(); !ok {
					t.Error("version request has no deadline")
				}
				if unavailable {
					return nil, fmt.Errorf("engine unavailable")
				}
				return map[string]string{"version": "v-running-test"}, nil
			}
			return map[string]any{}, nil
		}}
		s := testService(t, rpc)
		SetDesktopStatus(s, func() bool { return false }, nil, "0.1.0")
		info := s.About()
		want := "v-running-test"
		if unavailable {
			want = ""
		}
		if info.AppVersion != "0.1.0" || info.RcloneVersion != want || info.WindowsVersion == "" || info.Architecture == "" || s.DesktopVisible() {
			t.Fatalf("invalid diagnostics: %+v", info)
		}
	}
	s := &Service{}
	if !s.DesktopVisible() || s.About().RcloneVersion != "" {
		t.Fatal("headless defaults or absent engine are incorrect")
	}
}

func TestSnapshotObserverRunsOutsideServiceMutex(t *testing.T) {
	s := testService(t, &fakeRPC{})
	delivered := make(chan uint64, 1)
	SetDesktopStatus(s, nil, func(sequence uint64, view View) {
		_ = Language(s)
		if view.Settings.Language == "" {
			t.Error("incomplete snapshot")
		}
		delivered <- sequence
	}, "0.1.0")
	go s.Snapshot()
	if receive(t, delivered) == 0 {
		t.Fatal("snapshot generation absent")
	}
}
