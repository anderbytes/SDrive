package core

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"sdrive/internal/i18n"
	"sdrive/internal/platform"
)

type engineMount struct{ Fs, MountPoint string }

func readMounts(ctx context.Context, rpc RPC) ([]engineMount, error) {
	var result struct{ MountPoints *[]engineMount }
	if err := callRPC(ctx, rpc, "mount/listmounts", map[string]any{}, &result); err != nil || result.MountPoints == nil {
		return nil, fmt.Errorf("could not verify mounted drives; preserve the cache and try again")
	}
	seen := map[string]bool{}
	for _, m := range *result.MountPoints {
		point := mountKey(m.MountPoint)
		letter := len(point) == 2 && point[0] >= 'A' && point[0] <= 'Z' && point[1] == ':'
		_, pathErr := platform.LocalPath(m.MountPoint)
		if (!letter && pathErr != nil) || m.Fs == "" || seen[point] {
			return nil, fmt.Errorf("could not verify mounted drives; preserve the cache and try again")
		}
		seen[point] = true
	}
	return *result.MountPoints, nil
}

func (s *Service) Mount(id string) (resultErr error) {
	defer func() { s.diagnostics.Operation("mount", resultErr) }()
	s.mu.Lock()
	defer s.mu.Unlock()
	record, owned := false, false
	defer func() {
		if owned {
			s.busy[id] = false
			delete(s.operations, id)
			s.snapshotID++
			delete(s.inspections, id)
		}
		if record && resultErr != nil {
			s.recordConnectionError(id, resultErr)
			s.snapshotID++
		}
	}()
	i := s.index(id)
	if i < 0 {
		return fmt.Errorf("connection not found")
	}
	if worker := s.workers[s.store.Connections[i].Remote]; worker != nil && !worker.Alive() && !s.busy[id] {
		s.mounted[id] = false
		s.stopConnection(s.store.Connections[i])
	}
	if s.mounted[id] || s.busy[id] {
		return fmt.Errorf("connection already mounted or busy")
	}
	record = true
	if err := s.persistenceReady(); err != nil {
		return err
	}
	if s.exiting {
		return fmt.Errorf("exit in progress")
	}
	c := s.store.Connections[i]
	if s.recoveryHold {
		cache, err := s.cacheRemotes()
		if err != nil {
			return err
		}
		if cache["sd_"+c.Remote] && (!c.Recovery || c.TargetHash == "") {
			return fmt.Errorf("the restored connection cannot prove the cache destination; preserve the data directory for manual recovery")
		}
	}
	if s.session != nil && s.session.connection.ID == id && !s.session.Complete {
		return fmt.Errorf("complete or cancel reconfiguration")
	}
	if !s.winFsp() {
		return fmt.Errorf("run the SDrive installer again to prepare virtual drives")
	}
	if err := s.availableMount(c); err != nil {
		return err
	}
	for otherID, active := range s.busy {
		if active && otherID != id {
			other := s.index(otherID)
			if other >= 0 && mountTargetsOverlap(s.store.Connections[other], c) {
				return fmt.Errorf("letter in use by another operation")
			}
		}
	}
	if err := os.MkdirAll(s.dataPath(), 0700); err != nil {
		return err
	}
	free, err := platform.FreeBytes(s.dataPath())
	if err != nil || free < 5<<30 {
		return fmt.Errorf("keep at least 5 GiB free on the cache disk")
	}
	previous := c.Recovery
	s.busy[id] = true
	owned = true
	if c.SessionOnly && s.workers[c.Remote] == nil && s.startConnection != nil {
		return fmt.Errorf("Sign in again before connecting this session-only drive.")
	}
	if worker := s.workers[c.Remote]; worker != nil && !worker.Alive() {
		s.stopConnection(c)
	}
	if err := s.ensureConnection(c); err != nil {
		return err
	}
	rpc := s.connectionRPC(c)
	s.busy[id] = true
	owned = true
	s.operations[id] = "validating"
	s.snapshotID++
	delete(s.inspections, id)
	s.mu.Unlock()
	ctxTarget, cancelTarget := context.WithTimeout(context.Background(), 45*time.Second)
	var config map[string]string
	err = callRPC(ctxTarget, rpc, "config/get", map[string]string{"name": "sd_" + c.Remote}, &config)
	selectedURL := ""
	if err == nil {
		selectedURL, err = s.verifyDestination(ctxTarget, &c, config, false)
	}
	if c.PrimaryURL != "" && config != nil {
		config["url"] = c.PrimaryURL
	}
	cancelTarget()
	s.mu.Lock()
	i = s.index(id)
	if err == nil && (i < 0 || s.connectionRPC(c) != rpc || s.exiting) {
		err = fmt.Errorf("connection operation changed; try again")
	}
	if err != nil {
		return err
	}
	if err = oauthParametersForConnection(c, config); err != nil {
		return err
	}
	targetHash, err := checkDestination(c, config)
	if err != nil {
		return err
	}
	previousHash := c.TargetHash
	s.store.Connections[i].TargetHash = targetHash
	s.store.Connections[i].Recovery = true
	if err = saveStore(s.dir, s.store); err != nil {
		s.store.Connections[i].Recovery = previous
		s.store.Connections[i].TargetHash = previousHash
		return err
	}
	s.operations[id] = "mounting"
	s.snapshotID++
	delete(s.inspections, id)
	options := mountOptions(c, s.store.Settings)
	if err = s.availableMount(c); err != nil {
		return err
	}
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if selectedURL != "" && selectedURL != config["url"] {
		var result ConfigResult
		err = callRPC(ctx, rpc, "config/update", map[string]any{"name": "sd_" + c.Remote, "parameters": map[string]string{"url": selectedURL}, "opt": map[string]bool{"nonInteractive": true, "noObscure": true}}, &result)
	}
	if err == nil {
		err = callRPC(ctx, rpc, "mount/mount", options, nil)
	}
	var actual []engineMount
	var checkErr error
	if err != nil {
		checkCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
		actual, checkErr = readMounts(checkCtx, rpc)
		done()
	}
	s.mu.Lock()
	i = s.index(id)
	delete(s.inspections, id)
	if err != nil {
		checkCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		if checkErr == nil {
			active := false
			for _, mount := range actual {
				if mountKey(mount.MountPoint) == mountKey(mountPoint(c)) && mount.Fs == remote(c) {
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
	delete(s.localizedErrors, id)
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
	identity := destinationFields(c, config)
	encoded, _ := json.Marshal(identity)
	hash := fmt.Sprintf("%x", sha256.Sum256(encoded))
	if c.Recovery && (c.TargetHash == "" || c.TargetHash != hash) {
		return "", fmt.Errorf("destination differs from the pending cache; sign in to the original account and library before recovery")
	}
	return hash, nil
}

func (s *Service) pending(ctx context.Context, c Connection) (int, error) {
	s.mu.Lock()
	rpc := s.connectionRPC(c)
	s.mu.Unlock()
	return s.pendingOn(ctx, rpc, c)
}

func (s *Service) pendingOn(ctx context.Context, rpc RPC, c Connection) (int, error) {
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

func (s *Service) Unmount(id string) (resultErr error) {
	defer func() { s.diagnostics.Operation("unmount", resultErr) }()
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
	s.operations[id] = "checkingUploads"
	s.snapshotID++
	delete(s.inspections, id)
	s.mu.Unlock()
	unmounted, err := s.drainAndUnmount(c)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.busy[id] = false
	delete(s.operations, id)
	s.snapshotID++
	delete(s.inspections, id)
	if unmounted {
		delete(s.mounted, id)
	}
	if err != nil {
		s.recordConnectionError(id, err)
		return err
	}
	delete(s.mounted, id)
	delete(s.errors, id)
	delete(s.localizedErrors, id)
	i = s.index(id)
	s.store.Connections[i].Recovery = false
	if err = saveStore(s.dir, s.store); err != nil {
		s.store.Connections[i].Recovery = true
		return err
	}
	s.stopConnection(c)
	return nil
}

func (s *Service) drainAndUnmount(c Connection) (unmounted bool, resultErr error) {
	defer func() { s.diagnostics.Operation("cache", resultErr) }()
	s.mu.Lock()
	rpc := s.connectionRPC(c)
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	for {
		pending, err := s.pendingOn(ctx, rpc, c)
		if err != nil {
			return false, err
		}
		if pending == 0 {
			break
		}
		s.mu.Lock()
		s.operations[c.ID] = "waitingUploads"
		s.snapshotID++
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("there are open files or pending uploads; close files and wait for the network before trying again")
		case <-time.After(time.Second):
		}
	}
	s.mu.Lock()
	s.operations[c.ID] = "unmounting"
	s.snapshotID++
	s.mu.Unlock()
	if err := callRPC(ctx, rpc, "mount/unmount", map[string]string{"mountPoint": mountPoint(c)}, nil); err != nil {
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

func (s *Service) Exit() (resultErr error) {
	defer func() { s.diagnostics.Operation("exit", resultErr) }()
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
			return i18n.NewMessage("recovery.required_before_exit", map[string]string{"name": c.Name})
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
			return i18n.NewMessage("recovery.required_before_exit", map[string]string{"name": c.Name})
		}
	}
	success = true
	if s.quit != nil {
		s.quit()
	}
	return nil
}

func destinationFields(c Connection, config map[string]string) map[string]string {
	identity := map[string]string{"profile": c.Provider, "path": c.Path}
	for _, key := range []string{"type", "host", "port", "user", "username", "url", "vendor", "account", "endpoint", "region", "provider", "project_number", "project_id", "root_folder_id", "team_drive", "drive_id", "drive_type", "site_id", "site_url", "hostname", "root_id"} {
		if key == "account" && c.Provider == "b2" {
			continue
		}
		identity[key] = config[key]
	}
	if c.PrimaryURL != "" {
		identity["url"] = c.PrimaryURL
	}
	return identity
}
