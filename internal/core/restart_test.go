package core

import (
	"context"
	"reflect"
	"testing"
)

type stoppedRPC struct{ fakeRPC }

func (*stoppedRPC) Alive() bool { return false }

func TestRestartEngineGuards(t *testing.T) {
	for _, state := range []string{"active engine", "setup", "busy", "exiting"} {
		t.Run(state, func(t *testing.T) {
			s := testService(t, &stoppedRPC{})
			switch state {
			case "active engine":
				s.rpc = &fakeRPC{}
			case "setup":
				s.session = &ConfigSession{}
			case "busy":
				s.busy["connection"] = true
			case "exiting":
				s.exiting = true
			}
			called := false
			s.restart = func() (RPC, error) { called = true; return &fakeRPC{}, nil }
			if s.RestartEngine() == nil || called {
				t.Fatal("restart allowed during an active operation")
			}
		})
	}
}

func TestRestartFailureThenSuccessPreservesRecovery(t *testing.T) {
	s := testService(t, &stoppedRPC{})
	c := testConnection()
	s.store.Connections = []Connection{c}
	s.mounted[c.ID] = true
	s.errors[c.ID] = "engine stopped"
	if err := saveStore(s.dir, s.store); err != nil {
		t.Fatal(err)
	}
	s.restart = func() (RPC, error) { return nil, context.DeadlineExceeded }
	if s.RestartEngine() == nil || s.engineError == "" || !s.store.Connections[0].Recovery {
		t.Fatal("failed restart lost recovery state or error")
	}
	rpc := &fakeRPC{}
	s.restart = func() (RPC, error) { return rpc, nil }
	if err := s.RestartEngine(); err != nil {
		t.Fatal(err)
	}
	if s.rpc != rpc || s.engineError != "" || len(s.mounted) != 0 || len(s.errors) != 0 {
		t.Fatal("successful restart retained stale engine state")
	}
	persisted, err := loadStore(s.dir)
	if err != nil || !reflect.DeepEqual(persisted.Connections, []Connection{c}) || !reflect.DeepEqual(s.store.Connections, []Connection{c}) {
		t.Fatalf("restart changed pending destination: %v", err)
	}
	if !reflect.DeepEqual(rpc.calls, []string{"config/listremotes"}) {
		t.Fatalf("unexpected restart operations: %v", rpc.calls)
	}
}
