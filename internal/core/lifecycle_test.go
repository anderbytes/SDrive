package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutoMountReportsEarlyFailuresAndContinues(t *testing.T) {
	for _, failure := range []string{"WinFsp", "occupied letter", "config read", "destination", "save"} {
		t.Run(failure, func(t *testing.T) {
			mounted := false
			rpc := &fakeRPC{handle: func(_ context.Context, method string, input any) (any, error) {
				if method == "config/get" {
					name := input.(map[string]string)["name"]
					if name == "sd_"+strings.Repeat("2", 32) {
						if failure == "config read" {
							return nil, fmt.Errorf("configuration unavailable")
						}
						if failure == "destination" {
							return map[string]string{"type": "s3"}, nil
						}
					}
					return map[string]string{"type": "ftp", "host": "example.test"}, nil
				}
				if method == "mount/mount" {
					mounted = true
				}
				if method == "mount/listmounts" {
					return map[string]any{"mountPoints": []engineMount{}}, nil
				}
				return map[string]any{}, nil
			}}
			s := testService(t, rpc)
			c := testConnection()
			c.Recovery, c.AutoMount = false, true
			other := c
			other.ID, other.Remote, other.Letter = strings.Repeat("3", 32), strings.Repeat("4", 32), "T"
			s.store.Connections = []Connection{c, other}
			s.driveUsed = func(letter string) bool { return failure == "occupied letter" && letter == "S" }
			checks := 0
			s.mountAvailable = func() bool {
				checks++
				return failure != "WinFsp" || checks > 1
			}
			if failure == "save" {
				if err := os.Remove(filepath.Join(s.dir, "settings.json")); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Join(s.dir, "settings.json"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			AutoMount(s)
			if s.errors[c.ID] == "" || s.busy[c.ID] || s.operations[c.ID] != "" {
				t.Fatal("automatic failure was hidden or left the connection busy")
			}
			if failure == "save" {
				if err := os.Remove(filepath.Join(s.dir, "settings.json")); err != nil {
					t.Fatal(err)
				}
			} else if !mounted || !s.mounted[other.ID] {
				t.Fatal("one failure prevented the next automatic connection")
			}
			if got := s.Snapshot().Connections[0].Error; got == "" {
				t.Fatal("automatic failure missing from bridge snapshot")
			}
			s.driveUsed = func(string) bool { return false }
			rpc.handle = func(_ context.Context, method string, _ any) (any, error) {
				if method == "config/get" {
					return map[string]string{"type": "ftp", "host": "example.test"}, nil
				}
				return map[string]any{}, nil
			}
			if err := s.Mount(c.ID); err != nil || s.errors[c.ID] != "" || s.operations[c.ID] != "" {
				t.Fatalf("successful retry did not clear error/state: %v", err)
			}
		})
	}
}

func TestConfirmCorrectsLetterWithoutDuplicatingConnection(t *testing.T) {
	s := testService(t, &fakeRPC{handle: func(_ context.Context, _ string, _ any) (any, error) {
		return map[string]any{"list": []any{}}, nil
	}})
	c := testConnection()
	c.Recovery = false
	s.session = &ConfigSession{ID: "draft", Authenticated: true, connection: c, ctx: context.Background(), rpc: s.rpc}
	s.driveUsed = func(letter string) bool { return letter == "S" }
	if _, err := s.ConfirmConfig("draft", "Documents", MountTarget{Letter: "S"}); err == nil || len(s.store.Connections) != 0 {
		t.Fatal("occupied letter saved a draft")
	}
	id, err := s.ConfirmConfig("draft", "Documents", MountTarget{Letter: "T"})
	if err != nil || id != c.ID {
		t.Fatalf("letter correction before confirmation failed: %v", err)
	}
	if _, err := s.ConfirmConfig("draft", "ignored", MountTarget{Letter: "U"}); err != nil {
		t.Fatal(err)
	}
	got := s.store.Connections[0]
	if len(s.store.Connections) != 1 || got.ID != c.ID || got.Remote != c.Remote || got.Path != "Documents" || got.Letter != "U" || s.session.connection != got {
		t.Fatal("correction changed the connection identity or destination")
	}
	persisted, err := loadStore(s.dir)
	if err != nil || persisted.Connections[0] != got {
		t.Fatalf("corrected letter not persisted: %v", err)
	}
}

func TestCompletedSetupLetterCorrectionPreservesSafetyAndRollsBack(t *testing.T) {
	for _, blocked := range []string{"mounted", "busy", "recovery", "unknown cache", "dirty cache", "occupied", "reserved", "invalid", "save"} {
		t.Run(blocked, func(t *testing.T) {
			s := testService(t, &fakeRPC{})
			c := testConnection()
			c.Recovery = blocked == "recovery"
			s.store.Connections = []Connection{c}
			s.session = &ConfigSession{ID: "draft", Authenticated: true, Complete: true, connection: c}
			s.driveUsed = func(string) bool { return blocked == "occupied" }
			s.mounted[c.ID], s.busy[c.ID] = blocked == "mounted", blocked == "busy"
			if blocked == "reserved" {
				other := c
				other.ID, other.Remote, other.Letter = strings.Repeat("3", 32), strings.Repeat("4", 32), "T"
				s.store.Connections = append(s.store.Connections, other)
			}
			if err := saveStore(s.dir, s.store); err != nil {
				t.Fatal(err)
			}
			if blocked == "unknown cache" || blocked == "dirty cache" {
				root := filepath.Join(s.dataPath(), "vfsMeta", "sd_"+c.Remote)
				if err := os.MkdirAll(root, 0700); err != nil {
					t.Fatal(err)
				}
				metadata := `{"Dirty":true}`
				if blocked == "unknown cache" {
					metadata = `{}`
				}
				if err := os.WriteFile(filepath.Join(root, "file"), []byte(metadata), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if blocked == "save" {
				if err := os.Remove(filepath.Join(s.dir, "settings.backup.json")); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Join(s.dir, "settings.backup.json"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			letter := "T"
			if blocked == "invalid" {
				letter = "C"
			}
			if _, err := s.ConfirmConfig("draft", "changed", MountTarget{Letter: letter}); err == nil {
				t.Fatal("unsafe correction accepted")
			}
			persisted, err := loadStore(s.dir)
			if err != nil || persisted.Connections[0] != c || s.store.Connections[0] != c || s.session.connection != c {
				t.Fatalf("rejected correction changed persisted/session state: %v", err)
			}
		})
	}
}

func TestConfirmRechecksLetterAfterDestinationValidation(t *testing.T) {
	started, finish := make(chan struct{}), make(chan struct{})
	s := testService(t, &fakeRPC{handle: func(_ context.Context, _ string, _ any) (any, error) {
		close(started)
		<-finish
		return map[string]any{"list": []any{}}, nil
	}})
	c := testConnection()
	c.Recovery = false
	s.session = &ConfigSession{ID: "draft", Authenticated: true, connection: c, ctx: context.Background(), rpc: s.rpc}
	s.driveUsed = func(string) bool { return false }
	done := make(chan error, 1)
	go func() { _, err := s.ConfirmConfig("draft", "", MountTarget{Letter: "T"}); done <- err }()
	receive(t, started)
	s.mu.Lock()
	s.driveUsed = func(string) bool { return true }
	s.mu.Unlock()
	close(finish)
	if err := receive(t, done); err == nil || len(s.store.Connections) != 0 {
		t.Fatal("letter occupied during validation was saved")
	}
}

func TestRecoverySetupCannotChangeLetterBeforeConfirmation(t *testing.T) {
	s := testService(t, &fakeRPC{})
	c := testConnection()
	s.store.Connections = []Connection{c}
	s.session = &ConfigSession{ID: "draft", Authenticated: true, inPlace: true, connection: c}
	s.driveUsed = func(string) bool { return false }
	if _, err := s.ConfirmConfig("draft", c.Path, MountTarget{Letter: "T"}); err == nil || s.store.Connections[0] != c || s.session.connection != c {
		t.Fatal("recovery setup changed its saved letter")
	}
}

func TestMountStagesAndDuplicateCallsDoNotOverwriteActiveOperation(t *testing.T) {
	validating, mount := make(chan struct{}), make(chan struct{})
	validated, mounted := make(chan struct{}), make(chan struct{})
	s := testService(t, &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
		switch method {
		case "config/get":
			close(validating)
			<-validated
			return map[string]string{"type": "ftp", "host": "example.test"}, nil
		case "mount/mount":
			close(mount)
			<-mounted
		case "mount/listmounts":
			return map[string]any{"mountPoints": []engineMount{}}, nil
		}
		return map[string]any{}, nil
	}})
	c := testConnection()
	c.Recovery = false
	s.store.Connections = []Connection{c}
	s.mountAvailable = func() bool { return true }
	s.driveUsed = func(string) bool { return false }
	s.errors[c.ID] = "previous failure"
	done := make(chan error, 1)
	go func() { done <- s.Mount(c.ID) }()
	receive(t, validating)
	if cv := s.Snapshot().Connections[0]; !cv.Busy || cv.Operation != "validating" {
		t.Fatalf("missing validation stage: %+v", cv)
	}
	if err := s.Mount(c.ID); err == nil {
		t.Fatal("duplicate mount accepted")
	}
	if err := s.TestConnection(c.ID); err == nil {
		t.Fatal("access test overlapped mount")
	}
	s.mu.Lock()
	unchanged := s.errors[c.ID] == "previous failure" && s.operations[c.ID] == "validating"
	s.mu.Unlock()
	if !unchanged {
		t.Fatal("rejected duplicate changed active operation")
	}
	close(validated)
	receive(t, mount)
	if cv := s.Snapshot().Connections[0]; !cv.Busy || cv.Operation != "mounting" {
		t.Fatalf("missing mount stage: %+v", cv)
	}
	close(mounted)
	if err := receive(t, done); err != nil || s.busy[c.ID] || s.operations[c.ID] != "" || s.errors[c.ID] != "" {
		t.Fatalf("mount completion did not clear state: %v", err)
	}
}

func TestDisconnectShowsCheckingWaitingAndUnmounting(t *testing.T) {
	progress := make(chan DisconnectProgress, 8)
	stats := make(chan chan struct{}, 2)
	unmount, unmounted := make(chan struct{}), make(chan struct{})
	calls := 0
	s := testService(t, &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
		if method == "vfs/stats" {
			calls++
			resume := make(chan struct{})
			stats <- resume
			<-resume
			data := cleanCacheStats()
			if calls == 1 {
				data["uploadsQueued"], data["files"] = 1, 1
			}
			return map[string]any{"diskCache": data}, nil
		}
		if method == "mount/unmount" {
			close(unmount)
			<-unmounted
		}
		return map[string]any{}, nil
	}})
	c := testConnection()
	SetDisconnectProgress(s, func(event DisconnectProgress) { progress <- event })
	s.store.Connections = []Connection{c}
	s.mounted[c.ID] = true
	done := make(chan error, 1)
	go func() { done <- s.Unmount(c.ID) }()
	first := receive(t, stats)
	s.mu.Lock()
	checking := s.busy[c.ID] && s.operations[c.ID] == "checkingUploads"
	s.mu.Unlock()
	if !checking {
		t.Fatal("checking stage missing")
	}
	close(first)
	second := receive(t, stats)
	s.mu.Lock()
	waiting := s.busy[c.ID] && s.operations[c.ID] == "waitingUploads"
	s.mu.Unlock()
	if !waiting {
		t.Fatal("waiting stage missing")
	}
	close(second)
	receive(t, unmount)
	s.mu.Lock()
	unmounting := s.busy[c.ID] && s.operations[c.ID] == "unmounting"
	s.mu.Unlock()
	if !unmounting {
		t.Fatal("unmounting stage missing")
	}
	close(unmounted)
	if err := receive(t, done); err != nil || s.busy[c.ID] || s.operations[c.ID] != "" || s.store.Connections[0].Recovery {
		t.Fatalf("disconnect did not clear state: %v", err)
	}
	for index, stage := range []string{"checkingUploads", "waitingUploads", "unmounting", "succeeded"} {
		event := receive(t, progress)
		if event.ID != c.ID || event.Name != c.Name || event.Stage != stage || event.Sequence != uint64(index+1) {
			t.Fatalf("unexpected lifecycle notification: %+v", event)
		}
	}
}

func TestAccessTestReservesConnectionAndClearsStageOnFailure(t *testing.T) {
	started, finish := make(chan struct{}), make(chan struct{})
	s := testService(t, &fakeRPC{handle: func(_ context.Context, _ string, _ any) (any, error) {
		close(started)
		<-finish
		return nil, context.DeadlineExceeded
	}})
	c := testConnection()
	c.Recovery = false
	s.store.Connections = []Connection{c}
	done := make(chan error, 1)
	go func() { done <- s.TestConnection(c.ID) }()
	receive(t, started)
	if err := s.Remove(c.ID); err == nil {
		t.Fatal("connection removed during access test")
	}
	if err := s.RestartEngine(); err == nil {
		t.Fatal("engine replaced during access test")
	}
	s.mu.Lock()
	validating := s.operations[c.ID] == "validating" && s.busy[c.ID]
	s.mu.Unlock()
	if !validating {
		t.Fatal("access test stage missing")
	}
	close(finish)
	if err := receive(t, done); err == nil || s.busy[c.ID] || s.operations[c.ID] != "" {
		t.Fatalf("failed access test retained its reservation: %v", err)
	}
}
