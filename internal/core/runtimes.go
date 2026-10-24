package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// The legacy engine owns the old encrypted file; each worker owns one remote.
func SetConnectionEngine(s *Service, start func(Connection, string) (RPC, error)) {
	s.startConnection = start
}

// Caller holds s.mu. Tests without a native factory retain their injected RPC.
func (s *Service) connectionRPC(c Connection) RPC {
	if rpc := s.workers[c.Remote]; rpc != nil {
		return rpc
	}
	return s.rpc
}

func (s *Service) prepareConnection(ctx context.Context, c Connection, importLegacy bool) (RPC, error) {
	if s.startConnection == nil {
		return s.rpc, nil
	}
	existingWorker := s.workers[c.Remote]
	if existingWorker != nil {
		if !existingWorker.Alive() {
			return nil, fmt.Errorf("Connection engine stopped. Reconnect to recover your files.")
		}
		if !importLegacy {
			return existingWorker, nil
		}
	}
	proxy := ""
	if c.UseProxy {
		p, err := s.readProxy()
		if err != nil {
			return nil, err
		}
		proxy = p.URL()
		if proxy == "" {
			return nil, fmt.Errorf("Configure a proxy before using it for this connection.")
		}
	}
	if s.starting == nil {
		s.starting = map[string]bool{}
	}
	if s.starting[c.Remote] {
		return nil, fmt.Errorf("connection operation in progress")
	}
	s.starting[c.Remote] = true
	recordIndex := s.index(c.ID)
	var record Connection
	hasRecord := recordIndex >= 0 && s.store.Connections[recordIndex].Remote == c.Remote
	if hasRecord {
		record = s.store.Connections[recordIndex]
	}
	source, start := s.rpc, s.startConnection
	s.mu.Unlock()
	rpc, err := func() (RPC, error) {
		rpc, err := existingWorker, error(nil)
		if rpc == nil {
			rpc, err = start(c, proxy)
		}
		if err != nil {
			return nil, err
		}
		keep := false
		defer func() {
			if !keep && existingWorker == nil {
				closeRPC(rpc)
			}
		}()
		if importLegacy && !c.SessionOnly {
			var stored map[string]string
			if err = callRPC(ctx, rpc, "config/get", map[string]string{"name": "sd_" + c.Remote}, &stored); err == nil && len(stored) == 0 {
				err = callRPC(ctx, source, "config/get", map[string]string{"name": "sd_" + c.Remote}, &stored)
				if err == nil && len(stored) > 0 {
					if p, e := provider(c.Provider); e == nil {
						if e = oauthParameters(p, stored); e != nil {
							return nil, e
						}
					}
					var result ConfigResult
					err = callRPC(ctx, rpc, "config/create", map[string]any{"name": "sd_" + c.Remote, "type": stored["type"], "parameters": stored, "opt": map[string]bool{"nonInteractive": true, "noObscure": true}}, &result)
					if err == nil {
						var imported map[string]string
						err = callRPC(ctx, rpc, "config/get", map[string]string{"name": "sd_" + c.Remote}, &imported)
						if err == nil {
							for key, value := range stored {
								if imported[key] != value {
									err = fmt.Errorf("Could not verify isolated credentials. Preserve the original profile.")
									break
								}
							}
						}
					}
				}
			}
			if err != nil {
				return nil, err
			}
			if p, e := provider(c.Provider); e == nil && len(stored) > 0 {
				if stored["type"] != p.Backend {
					return nil, fmt.Errorf("destination type does not match the saved connection")
				}
				if e = oauthParameters(p, stored); e != nil {
					return nil, e
				}
			}
		}
		keep = true
		return rpc, nil
	}()
	s.mu.Lock()
	delete(s.starting, c.Remote)
	if err != nil {
		return nil, err
	}
	recordIndex = s.index(c.ID)
	if s.rpc != source || s.exiting || ctx.Err() != nil || (hasRecord && (recordIndex < 0 || s.store.Connections[recordIndex] != record)) {
		if existingWorker == nil {
			closeRPC(rpc)
		}
		return nil, fmt.Errorf("connection operation changed; try again")
	}
	if s.workers == nil {
		s.workers = map[string]RPC{}
	}
	s.workers[c.Remote] = rpc
	return rpc, nil
}

func closeRPC(rpc RPC) {
	if closer, ok := rpc.(interface{ Close() }); ok {
		closer.Close()
	}
}

func CloseConnectionEngines(s *Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for remote, rpc := range s.workers {
		closeRPC(rpc)
		delete(s.workers, remote)
	}
}

func (s *Service) stopConnection(c Connection) {
	if rpc := s.workers[c.Remote]; rpc != nil {
		closeRPC(rpc)
		delete(s.workers, c.Remote)
	}
}

func (s *Service) ensureConnection(c Connection) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := s.prepareConnection(ctx, c, true)
	return err
}

// Caller holds s.mu. Only credentials for the explicitly converted remote are removed.
func (s *Service) forgetRemote(remote string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	old := Connection{Remote: remote}
	for _, c := range s.store.Connections {
		if c.Remote == remote {
			old = c
			break
		}
	}
	if _, err := s.prepareConnection(ctx, old, false); err != nil {
		return err
	}
	rpc := s.connectionRPC(old)
	if err := callRPC(ctx, rpc, "config/delete", map[string]string{"name": "sd_" + remote}, nil); err != nil {
		return err
	}
	if rpc != s.rpc {
		if err := callRPC(ctx, s.rpc, "config/delete", map[string]string{"name": "sd_" + remote}, nil); err != nil {
			return err
		}
	}
	s.stopConnection(old)
	return nil
}

// Legacy access details can be read without instantiating a provider or using a shared OAuth client.
func (s *Service) loadConnectionRPC(c Connection) error {
	if s.startConnection == nil || c.SessionOnly || s.workers[c.Remote] != nil {
		return nil
	}
	_, err := os.Stat(filepath.Join(s.dir, "connections", "sd_"+c.Remote, "rclone.conf"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err = s.prepareConnection(ctx, c, false)
	return err
}
