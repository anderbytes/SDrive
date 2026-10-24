package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Caller holds s.mu. Presence alone cannot certify an isolated credential file.
func (s *Service) isolatedRecoveryInventory(known, available map[string]bool) error {
	for _, c := range s.store.Connections {
		if c.SessionOnly {
			var binding map[string]string
			if c.Binding == "" || json.Unmarshal([]byte(c.Binding), &binding) != nil {
				return fmt.Errorf("credentials for a restored connection are missing; manual recovery is required")
			}
			available["sd_"+c.Remote] = true
		}
	}
	if s.startConnection == nil {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(s.dir, "connections"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("could not verify saved credentials; cleanup is suspended")
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !generatedRemote(entry.Name()) {
			return fmt.Errorf("could not verify saved credentials; cleanup is suspended")
		}
		c := Connection{Remote: entry.Name()[3:]}
		for _, saved := range s.store.Connections {
			if saved.Remote == c.Remote {
				c = saved
				break
			}
		}
		if c.SessionOnly {
			continue
		}
		ctx, done := context.WithTimeout(context.Background(), 10*time.Second)
		rpc, err := s.prepareConnection(ctx, c, false)
		var result struct{ Remotes []string }
		if err == nil {
			err = callRPC(ctx, rpc, "config/listremotes", map[string]any{}, &result)
		}
		done()
		if err != nil || result.Remotes == nil {
			return fmt.Errorf("could not verify saved credentials; cleanup is suspended")
		}
		for _, name := range result.Remotes {
			if name != entry.Name() || !known[name] {
				return fmt.Errorf("credentials absent from restored settings remain preserved; manual recovery is required")
			}
			available[name] = true
		}
	}
	return nil
}
