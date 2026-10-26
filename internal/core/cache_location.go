package core

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sdrive/internal/platform"
)

const cacheLocationFile = "cache-location.json"
const cacheOwnerFile = ".sdrive-cache-owner"

type cacheLocation struct {
	Path     string `json:"path"`
	Owner    string `json:"owner"`
	Identity string `json:"identity"`
}

func readCacheLocation(dir string) (cacheLocation, error) {
	var location cacheLocation
	f, err := os.Open(filepath.Join(dir, cacheLocationFile))
	if err != nil {
		return location, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil {
		return location, err
	}
	if len(b) > 8192 || json.Unmarshal(b, &location) != nil || location.Path == "" || len(location.Owner) != 32 || location.Identity == "" {
		return location, fmt.Errorf("Cache location metadata is invalid. Preserve the profile and cache.")
	}
	return location, nil
}

func verifyCacheLocation(location cacheLocation) error {
	path, err := platform.LocalPath(location.Path)
	if err != nil {
		return fmt.Errorf("The selected cache is unavailable. Restore its original disk and directory.")
	}
	identity, err := platform.DirectoryIdentity(path)
	if err != nil || identity != location.Identity {
		return fmt.Errorf("The selected cache is unavailable or has changed identity. Restore its original disk and directory.")
	}
	marker := filepath.Join(path, cacheOwnerFile)
	info, err := os.Lstat(marker)
	if err != nil || !info.Mode().IsRegular() || info.Size() != 32 {
		return fmt.Errorf("Cache ownership could not be verified. Preserve the profile and cache.")
	}
	b, err := os.ReadFile(marker)
	if err != nil || string(b) != location.Owner {
		return fmt.Errorf("Cache ownership could not be verified. Preserve the profile and cache.")
	}
	return nil
}

func writeCacheLocation(dir string, location cacheLocation) error {
	b, err := json.Marshal(location)
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dir, cacheLocationFile), b)
}

// Before version 3 is saved, bind the existing cache without moving its contents.
func ensureCacheLocation(dir string) error {
	if location, err := readCacheLocation(dir); err == nil {
		return verifyCacheLocation(location)
	} else if !os.IsNotExist(err) {
		return err
	}
	for _, name := range []string{"settings.json", "settings.backup.json"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		var version struct{ Version int }
		if err == nil && json.Unmarshal(b, &version) == nil && version.Version >= 3 {
			return fmt.Errorf("Cache location metadata is missing. Restore it before changing the profile.")
		}
	}
	path := filepath.Join(dir, "cache")
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	identity, err := platform.DirectoryIdentity(path)
	if err != nil {
		return err
	}
	if err = platform.ProtectDir(path); err != nil {
		return err
	}
	owner := newID()
	marker := filepath.Join(path, cacheOwnerFile)
	// An interrupted initial binding may leave its owner marker before the manifest.
	if info, err := os.Lstat(marker); err == nil {
		if !info.Mode().IsRegular() || info.Size() != 32 {
			return fmt.Errorf("Cache ownership could not be verified. Preserve the profile and cache.")
		}
		b, err := os.ReadFile(marker)
		if err != nil || len(b) != 32 {
			return fmt.Errorf("Cache ownership could not be verified. Preserve the profile and cache.")
		}
		owner = string(b)
	} else if !os.IsNotExist(err) {
		return err
	} else if err = atomicWrite(marker, []byte(owner)); err != nil {
		return err
	}
	return writeCacheLocation(dir, cacheLocation{path, owner, identity})
}

func (s *Service) cacheReady() error {
	location, err := readCacheLocation(s.dir)
	if os.IsNotExist(err) {
		// Legacy profiles have not yet committed a cache binding.
		for _, name := range []string{"settings.json", "settings.backup.json"} {
			b, e := os.ReadFile(filepath.Join(s.dir, name))
			var version struct{ Version int }
			if e == nil && json.Unmarshal(b, &version) == nil && version.Version >= 3 {
				return fmt.Errorf("Cache location metadata is missing. Restore it before changing the profile.")
			}
		}
		return nil
	}
	if err != nil {
		return err
	}
	if location.Path != s.dataPath() {
		return fmt.Errorf("Cache location changed outside this session. Restart SDrive and preserve both directories.")
	}
	return verifyCacheLocation(location)
}

// CachePath is the same authoritative directory used by recovery and worker startup.
func CachePath(s *Service) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dataPath()
}

func (s *Service) ChangeCacheLocation(path string) error {
	return s.changeCacheLocation(path, false)
}

func (s *Service) RestoreDefaultCacheLocation() (string, error) {
	path := filepath.Join(s.dir, "cache")
	if err := s.changeCacheLocation(path, true); err != nil {
		return "", err
	}
	return path, nil
}

// Only the fixed default may reuse an owner marker and empty directory skeleton.
func cacheDestinationOwner(path string, restoreDefault bool) (string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", nil
	}
	if !restoreDefault {
		return "", fmt.Errorf("Choose an empty dedicated cache directory.")
	}
	owner := ""
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = filepath.WalkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			_, err := platform.DirectoryIdentity(current)
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if current != filepath.Join(path, cacheOwnerFile) || !info.Mode().IsRegular() || info.Size() != 32 {
			return fmt.Errorf("The default cache contains files or unknown entries. Preserve it before restoring its location.")
		}
		b, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		if _, err := hex.DecodeString(string(b)); err != nil || len(b) != 32 {
			return fmt.Errorf("Cache ownership could not be verified. Preserve the profile and cache.")
		}
		owner = string(b)
		return nil
	})
	if err != nil {
		return "", err
	}
	if owner == "" {
		return "", fmt.Errorf("Cache ownership could not be verified. Preserve the profile and cache.")
	}
	return owner, nil
}

func (s *Service) changeCacheLocation(path string, restoreDefault bool) error {
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	defer s.mu.Unlock()
	if err := s.persistenceReady(); err != nil {
		return err
	}
	if restoreDefault && strings.EqualFold(path, s.dataPath()) {
		return nil
	}
	if s.session != nil || s.recoveryHold || s.exiting {
		return fmt.Errorf("Disconnect all drives and finish setup or recovery before changing the cache.")
	}
	for _, inspection := range s.inspections {
		if !inspection.complete {
			return fmt.Errorf("Wait for the cache inspection before changing its location.")
		}
	}
	for _, active := range s.starting {
		if active {
			return fmt.Errorf("Wait for connection operations before changing the cache.")
		}
	}
	for _, c := range s.store.Connections {
		if s.mounted[c.ID] || s.busy[c.ID] || c.Recovery {
			return fmt.Errorf("Disconnect all drives and resolve pending files before changing the cache.")
		}
	}
	var err error
	path, err = platform.LocalPath(path)
	if err != nil {
		return err
	}
	if pathsOverlap(path, s.dir) && !(restoreDefault && strings.EqualFold(path, filepath.Join(s.dir, "cache"))) {
		return fmt.Errorf("Choose an empty cache directory outside the profile.")
	}
	for _, c := range s.store.Connections {
		if c.MountFolder != "" && pathsOverlap(path, c.MountFolder) {
			return fmt.Errorf("The cache must not overlap a mount folder.")
		}
	}
	if restoreDefault {
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			if _, err := platform.DirectoryIdentity(s.dir); err != nil {
				return err
			}
			if err := os.Mkdir(path, 0700); err != nil {
				return err
			}
		}
	}
	identity, err := platform.DirectoryIdentity(path)
	if err != nil {
		return err
	}
	owner, err := cacheDestinationOwner(path, restoreDefault)
	if err != nil {
		return err
	}
	free, err := platform.FreeBytes(path)
	if err != nil || free < 5<<30 {
		return fmt.Errorf("keep at least 5 GiB free on the cache disk")
	}
	checkCtx, checkCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer checkCancel()
	for _, worker := range s.workers {
		if worker.Alive() {
			mounts, e := readMounts(checkCtx, worker)
			if e != nil || len(mounts) != 0 {
				return fmt.Errorf("Mounted or unknown engine state prevents changing the cache.")
			}
		}
	}
	if s.rpc != nil && s.rpc.Alive() {
		mounts, e := readMounts(checkCtx, s.rpc)
		if e != nil || len(mounts) != 0 {
			return fmt.Errorf("Mounted or unknown engine state prevents changing the cache.")
		}
	}
	// Stop every worker first, then inspect durable metadata without a concurrent writer.
	for remote, worker := range s.workers {
		closeRPC(worker)
		delete(s.workers, remote)
	}
	if s.startConnection == nil && len(s.store.Connections) > 0 && s.rpc != nil {
		return fmt.Errorf("Restart SDrive with isolated connection engines before changing the cache.")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cache, err := s.cacheRemotes()
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, c := range s.store.Connections {
		known["sd_"+c.Remote] = true
		pending, err := s.cachedPending(ctx, c)
		if err != nil || pending != 0 {
			return fmt.Errorf("The cache is pending or unknown. Preserve it and recover files before changing its location.")
		}
	}
	for name := range cache {
		if !known[name] {
			return fmt.Errorf("Unknown cached connections must be recovered before changing the cache.")
		}
	}
	// Both settings generations must reject older apps before an external cache is activated.
	if err = saveStore(s.dir, s.store); err != nil {
		return err
	}
	if err = saveStore(s.dir, s.store); err != nil {
		return err
	}
	if err = platform.ProtectDir(path); err != nil {
		return err
	}
	if check, e := platform.DirectoryIdentity(path); e != nil || check != identity {
		return fmt.Errorf("The selected directory changed. Choose it again.")
	}
	checkOwner, err := cacheDestinationOwner(path, restoreDefault)
	if err != nil {
		return err
	}
	if checkOwner != owner {
		return fmt.Errorf("The selected directory changed. Choose it again.")
	}
	location := cacheLocation{Path: path, Owner: owner, Identity: identity}
	if owner == "" {
		location.Owner = newID()
		if err = atomicWrite(filepath.Join(path, cacheOwnerFile), []byte(location.Owner)); err != nil {
			return err
		}
	}
	if err = writeCacheLocation(s.dir, location); err != nil {
		return err
	}
	s.cacheRoot = path
	s.inspections = nil
	s.snapshotID++
	return nil
}

func SetDirectoryPicker(s *Service, pick func() (string, error)) { s.pickDirectory = pick }
func (s *Service) SelectDirectory() (string, error) {
	if err := s.accessAllowed(); err != nil {
		return "", err
	}
	if s.pickDirectory == nil {
		return "", fmt.Errorf("Directory selection is unavailable.")
	}
	return s.pickDirectory()
}
