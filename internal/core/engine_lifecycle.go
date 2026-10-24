package core

import (
	"fmt"
)

func (s *Service) RestartEngine() (resultErr error) {
	defer func() { s.diagnostics.Operation("engine_start", resultErr) }()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.persistenceReady(); err != nil {
		return err
	}
	if s.exiting {
		return fmt.Errorf("exit in progress")
	}
	if s.rpc != nil && s.rpc.Alive() {
		return fmt.Errorf("the engine is active; unmount drives before restarting the application")
	}
	if s.session != nil {
		return fmt.Errorf("cancel setup before restarting the engine")
	}
	for _, busy := range s.busy {
		if busy {
			return fmt.Errorf("wait for operations to finish")
		}
	}
	if s.restart == nil {
		return fmt.Errorf("restart unavailable")
	}
	for _, mounted := range s.mounted {
		if mounted && s.startConnection != nil {
			return fmt.Errorf("unmount drives before restarting the engine")
		}
	}
	for remote, worker := range s.workers {
		closeRPC(worker)
		delete(s.workers, remote)
	}
	rpc, err := s.restart()
	if err != nil {
		s.engineError = err.Error()
		return err
	}
	s.rpc = rpc
	s.engineError = ""
	s.mounted = map[string]bool{}
	s.inspections = nil
	s.errors = map[string]string{}
	s.localizedErrors = nil
	_ = s.cleanup()
	return nil
}

func AutoMount(s *Service) {
	s.mu.Lock()
	if s.settingsError != "" || s.recoveryHold {
		s.mu.Unlock()
		return
	}
	connections := append([]Connection{}, s.store.Connections...)
	s.mu.Unlock()
	for _, c := range connections {
		if c.AutoMount && !c.SessionOnly {
			_ = s.Mount(c.ID)
		}
	}
}
