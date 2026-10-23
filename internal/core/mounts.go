package core

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"sdrive/internal/platform"
)

func (s *Service) Mount(id string) error {
	s.mu.Lock()
	if err := s.persistenceReady(); err != nil {
		s.mu.Unlock()
		return err
	}
	i := s.index(id)
	if i < 0 {
		s.mu.Unlock()
		return fmt.Errorf("connection not found")
	}
	if s.exiting {
		s.mu.Unlock()
		return fmt.Errorf("exit in progress")
	}
	c := s.store.Connections[i]
	if s.recoveryHold {
		cache, err := s.cacheRemotes()
		if err != nil {
			s.mu.Unlock()
			return err
		}
		if cache["sd_"+c.Remote] && (!c.Recovery || c.TargetHash == "") {
			s.mu.Unlock()
			return fmt.Errorf("the restored connection cannot prove the cache destination; preserve the data directory for manual recovery")
		}
	}
	if s.mounted[id] || s.busy[id] {
		s.mu.Unlock()
		return fmt.Errorf("connection already mounted or busy")
	}
	if s.session != nil && s.session.connection.ID == id && !s.session.Complete {
		s.mu.Unlock()
		return fmt.Errorf("complete or cancel reconfiguration")
	}
	if !platform.WinFspInstalled() {
		s.mu.Unlock()
		return fmt.Errorf("run the SDrive installer again to prepare virtual drives")
	}
	if platform.DriveUsed(c.Letter) {
		s.mu.Unlock()
		return fmt.Errorf("letter %s: is already in use", c.Letter)
	}
	for otherID, active := range s.busy {
		if active && otherID != id {
			other := s.index(otherID)
			if other >= 0 && s.store.Connections[other].Letter == c.Letter {
				s.mu.Unlock()
				return fmt.Errorf("letter in use by another operation")
			}
		}
	}
	if err := os.MkdirAll(s.dataPath(), 0700); err != nil {
		s.mu.Unlock()
		return err
	}
	free, err := platform.FreeBytes(s.dataPath())
	if err != nil || free < 5<<30 {
		s.mu.Unlock()
		return fmt.Errorf("keep at least 5 GiB free on the cache disk")
	}
	previous := c.Recovery
	ctxTarget, cancelTarget := context.WithTimeout(context.Background(), 5*time.Second)
	var config map[string]string
	err = s.call(ctxTarget, "config/get", map[string]string{"name": "sd_" + c.Remote}, &config)
	cancelTarget()
	if err != nil {
		s.mu.Unlock()
		return err
	}
	targetHash, err := checkDestination(c, config)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	previousHash := c.TargetHash
	s.store.Connections[i].TargetHash = targetHash
	s.store.Connections[i].Recovery = true
	if err = saveStore(s.dir, s.store); err != nil {
		s.store.Connections[i].Recovery = previous
		s.store.Connections[i].TargetHash = previousHash
		s.mu.Unlock()
		return err
	}
	s.busy[id] = true
	delete(s.inspections, id)
	cacheGiB := s.store.Settings.CacheGiB
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	err = s.call(ctx, "mount/mount", map[string]any{"fs": remote(c), "mountPoint": c.Letter + ":", "mountOpt": map[string]any{"NetworkMode": true, "VolumeName": c.Name}, "vfsOpt": map[string]any{"CacheMode": "full", "CacheMaxSize": int64(cacheGiB) << 30, "CacheMinFreeSpace": int64(5) << 30, "CacheMaxAge": int64(24 * time.Hour), "WriteBack": int64(5 * time.Second), "ReadOnly": c.ReadOnly}}, nil)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.busy[id] = false
	delete(s.inspections, id)
	if err != nil {
		s.errors[id] = err.Error()
		checkCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		var mounts struct{ MountPoints []struct{ MountPoint string } }
		if checkErr := s.call(checkCtx, "mount/listmounts", map[string]any{}, &mounts); checkErr == nil {
			active := false
			for _, mount := range mounts.MountPoints {
				if mount.MountPoint == c.Letter+":" {
					active = true
				}
			}
			if active {
				s.mounted[id] = true
			} else if !previous {
				count, cacheErr := s.cachedPending(checkCtx, c)
				if cacheErr == nil && count == 0 {
					s.store.Connections[i].Recovery = false
					if saveErr := saveStore(s.dir, s.store); saveErr != nil {
						s.store.Connections[i].Recovery = true
					}
				}
			}
		}
		return err
	}
	s.mounted[id] = true
	delete(s.errors, id)
	return nil
}

func checkDestination(c Connection, config map[string]string) (string, error) {
	p, err := provider(c.Provider)
	if err != nil {
		return "", err
	}
	if config["type"] != p.Backend {
		return "", fmt.Errorf("destination type does not match the saved connection")
	}
	identity := map[string]string{"profile": c.Provider, "path": c.Path}
	for _, key := range []string{"type", "host", "port", "user", "username", "url", "vendor", "account", "endpoint", "region", "provider", "project_number", "project_id", "root_folder_id", "team_drive", "drive_id", "drive_type", "site_id", "site_url", "hostname", "root_id"} {
		if key == "account" && c.Provider == "b2" {
			continue
		}
		identity[key] = config[key]
	}
	encoded, _ := json.Marshal(identity)
	hash := fmt.Sprintf("%x", sha256.Sum256(encoded))
	if c.Recovery && (c.TargetHash == "" || c.TargetHash != hash) {
		return "", fmt.Errorf("destination differs from the pending cache; sign in to the original account and library before recovery")
	}
	return hash, nil
}

func (s *Service) pending(ctx context.Context, c Connection) (int, error) {
	s.mu.Lock()
	rpc := s.rpc
	s.mu.Unlock()
	uploads, _, err := readCacheStats(ctx, rpc, c)
	if err != nil {
		return 0, err
	}
	cached, err := s.cachedPending(ctx, c)
	if err != nil {
		return 0, err
	}
	return max(cached, uploads.Active+uploads.Queued, uploads.CacheErrors), nil
}

func (s *Service) Unmount(id string) error {
	s.mu.Lock()
	i := s.index(id)
	if i < 0 {
		s.mu.Unlock()
		return fmt.Errorf("connection not found")
	}
	c := s.store.Connections[i]
	if s.busy[id] {
		s.mu.Unlock()
		return fmt.Errorf("operation in progress")
	}
	if !s.mounted[id] {
		s.mu.Unlock()
		return fmt.Errorf("drive is not mounted; use Connect to recover pending files")
	}
	s.busy[id] = true
	delete(s.inspections, id)
	s.mu.Unlock()
	unmounted, err := s.drainAndUnmount(c)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.busy[id] = false
	delete(s.inspections, id)
	if unmounted {
		delete(s.mounted, id)
	}
	if err != nil {
		s.errors[id] = err.Error()
		return err
	}
	delete(s.mounted, id)
	delete(s.errors, id)
	i = s.index(id)
	s.store.Connections[i].Recovery = false
	if err = saveStore(s.dir, s.store); err != nil {
		s.store.Connections[i].Recovery = true
		return err
	}
	return nil
}

func (s *Service) drainAndUnmount(c Connection) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	for {
		pending, err := s.pending(ctx, c)
		if err != nil {
			return false, err
		}
		if pending == 0 {
			break
		}
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("there are open files or pending uploads; close files and wait for the network before trying again")
		case <-time.After(time.Second):
		}
	}
	if err := s.call(ctx, "mount/unmount", map[string]string{"mountPoint": c.Letter + ":"}, nil); err != nil {
		return false, fmt.Errorf("could not unmount; close open files and try again")
	}
	// Shutdown removes the VFS from RC, so inspect durable metadata after unmount.
	pending, err := s.cachedPending(ctx, c)
	if err != nil {
		return true, err
	}
	if pending > 0 {
		return true, fmt.Errorf("the drive was unmounted with pending files; use Recover before exiting")
	}
	return true, nil
}

func (s *Service) Exit() error {
	s.mu.Lock()
	if s.exiting {
		s.mu.Unlock()
		return fmt.Errorf("exit in progress")
	}
	if s.session != nil && !s.session.Complete {
		s.mu.Unlock()
		return fmt.Errorf("complete or cancel setup before exiting")
	}
	s.exiting = true
	success := false
	defer func() {
		if !success {
			s.mu.Lock()
			s.exiting = false
			s.mu.Unlock()
		}
	}()
	ids := []string{}
	for id, active := range s.mounted {
		if active {
			ids = append(ids, id)
		}
	}
	s.mu.Unlock()
	for _, id := range ids {
		if err := s.Unmount(id); err != nil {
			return err
		}
	}
	s.mu.Lock()
	for _, busy := range s.busy {
		if busy {
			s.mu.Unlock()
			return fmt.Errorf("wait for operations before exiting")
		}
	}
	connections := append([]Connection{}, s.store.Connections...)
	for _, c := range connections {
		if c.Recovery {
			s.mu.Unlock()
			return fmt.Errorf("recover connection %s before exiting safely", c.Name)
		}
	}
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	for _, c := range connections {
		pending, err := s.cachedPending(ctx, c)
		if err != nil {
			return fmt.Errorf("could not verify local cache before exiting; preserve the files")
		}
		if pending > 0 {
			return fmt.Errorf("recover connection %s before exiting safely", c.Name)
		}
	}
	success = true
	if s.quit != nil {
		s.quit()
	}
	return nil
}
