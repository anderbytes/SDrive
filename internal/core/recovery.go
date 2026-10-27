package core

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const recoveryMarker = "settings.recovery"

func (s *Service) backupStore() (Store, error) {
	b, err := os.ReadFile(filepath.Join(s.dir, "settings.backup.json"))
	if err != nil {
		return Store{}, err
	}
	return decodeStore(b)
}

func generatedRemote(name string) bool {
	if len(name) != 35 || !strings.HasPrefix(name, "sd_") {
		return false
	}
	_, err := hex.DecodeString(name[3:])
	return err == nil
}

func (s *Service) listRemotes() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var result struct{ Remotes []string }
	if err := s.call(ctx, "config/listremotes", map[string]any{}, &result); err != nil || result.Remotes == nil {
		return nil, fmt.Errorf("could not verify saved credentials; cleanup is suspended")
	}
	for i, name := range result.Remotes {
		result.Remotes[i] = strings.TrimSuffix(name, ":")
	}
	return result.Remotes, nil
}

// Presence, not Dirty metadata, establishes what must be preserved during settings recovery.
func (s *Service) cacheRemotes() (map[string]bool, error) {
	if err := s.cacheReady(); err != nil {
		return nil, err
	}
	remotes := map[string]bool{}
	if info, err := os.Lstat(s.dataPath()); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return nil, fmt.Errorf("unexpected recovery cache entry; preserve the data directory")
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("could not inspect recovery cache; preserve the data directory")
	}
	for _, kind := range []string{"vfs", "vfsMeta"} {
		root := filepath.Join(s.dataPath(), kind)
		if info, err := os.Lstat(root); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return nil, fmt.Errorf("unexpected recovery cache entry; preserve the data directory")
		} else if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("could not inspect recovery cache; preserve the data directory")
		}
		entries, err := os.ReadDir(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("could not inspect recovery cache; preserve the data directory")
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("unexpected recovery cache entry; preserve the data directory")
			}
			remotes[entry.Name()] = true
		}
	}
	return remotes, nil
}

func (s *Service) initializeRecovery(loadErr error) {
	if _, err := os.Stat(filepath.Join(s.dir, recoveryMarker)); !os.IsNotExist(err) {
		s.recoveryHold = true
	}
	if loadErr != nil {
		s.settingsError = "Could not load local settings. Preserve the data directory and restore a valid backup."
		s.incompatibleSettings = errors.Is(loadErr, errSettingsVersion)
		if s.incompatibleSettings {
			s.settingsError = errSettingsVersion.Error()
		}
		s.store = defaultStore()
		return
	}
	if _, err := os.Stat(filepath.Join(s.dir, "settings.json")); !os.IsNotExist(err) {
		return
	}
	// Missing settings cannot turn an established profile into an empty first-run profile.
	if _, err := os.Stat(filepath.Join(s.dir, "proxy.enc")); !os.IsNotExist(err) {
		s.settingsError = "Local settings are missing. Preserve the data directory and restore a valid backup."
		return
	}
	if entries, err := os.ReadDir(filepath.Join(s.dir, "connections")); (err == nil && len(entries) > 0) || (err != nil && !os.IsNotExist(err)) {
		s.settingsError = "Local settings are missing. Preserve the data directory and restore a valid backup."
		return
	}
	_, backupErr := os.Stat(filepath.Join(s.dir, "settings.backup.json"))
	cache, cacheErr := s.cacheRemotes()
	if !os.IsNotExist(backupErr) || cacheErr != nil || len(cache) > 0 || s.recoveryHold {
		s.settingsError = "Local settings are missing. Preserve the data directory and restore a valid backup."
		return
	}
	if s.rpc != nil {
		remotes, err := s.listRemotes()
		if err == nil && len(remotes) == 0 {
			return
		}
		s.settingsError = "Could not confirm a new profile; existing credentials must be preserved."
		return
	}
	if config, err := os.ReadFile(filepath.Join(s.dir, "rclone.conf")); !os.IsNotExist(err) && (err != nil || len(config) > 0) {
		s.settingsError = "Could not confirm a new profile; existing credentials must be preserved."
	}
}

func (s *Service) persistenceReady() error {
	if s.settingsError != "" {
		return fmt.Errorf("restore valid settings before changing connections or preferences")
	}
	return s.cacheReady()
}

// StartEngine does not initialize keys or configuration for an unreadable settings file.
func StartEngine(s *Service) {
	if s.persistenceReady() == nil {
		if _, err := os.Stat(filepath.Join(s.dir, "settings.json")); os.IsNotExist(err) {
			// Commit the first profile before creating engine files, so reopening is unambiguous.
			if err = saveStore(s.dir, s.store); err != nil {
				s.settingsError = "Could not save initial settings. Preserve the data directory and restart SDrive."
				return
			}
		}
		_ = s.RestartEngine()
	}
}

func (s *Service) RetryCleanup() error {
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	defer s.mu.Unlock()
	if err := s.persistenceReady(); err != nil {
		return err
	}
	if s.recoveryHold || s.session != nil || s.exiting {
		return fmt.Errorf("resolve recovery or setup before retrying cleanup")
	}
	return s.cleanup()
}

func (s *Service) cleanup() error {
	err := s.purgeOrphans()
	s.maintenanceError = ""
	if err != nil {
		s.maintenanceError = "Credential cleanup could not finish. Saved connections remain available; try cleanup again."
	}
	return err
}

func (s *Service) RestoreSettings() (resultErr error) {
	defer func() { s.diagnostics.Operation("recovery", resultErr) }()
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	defer s.mu.Unlock()
	if s.settingsError == "" || s.incompatibleSettings || s.session != nil || s.exiting {
		return fmt.Errorf("settings restoration is not available")
	}
	backup, err := s.backupStore()
	if err != nil {
		return fmt.Errorf("no valid settings backup is available; preserve the data directory")
	}
	cache, err := s.cacheRemotes()
	if err != nil {
		return err
	}
	for i := range backup.Connections {
		if cache["sd_"+backup.Connections[i].Remote] {
			backup.Connections[i].Recovery = true
		}
	}
	backup.AccessProtected = s.store.AccessProtected || s.access.record != nil
	data, err := encodeStore(backup)
	if err != nil {
		return err
	}
	if err = ensureCacheLocation(s.dir); err != nil {
		return err
	}
	if err = atomicWrite(filepath.Join(s.dir, recoveryMarker), []byte("Preserve credentials and cache until recovery is verified.\n")); err != nil {
		return fmt.Errorf("could not protect recovery; settings were not restored")
	}
	s.recoveryHold = true
	path := filepath.Join(s.dir, "settings.json")
	original, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("could not read damaged settings for preservation")
	}
	if err == nil {
		archive, err := os.CreateTemp(s.dir, "settings-damaged-*.json")
		if err != nil {
			return fmt.Errorf("could not archive damaged settings; restoration cancelled")
		}
		if err = archive.Chmod(0600); err == nil {
			_, err = archive.Write(original)
		}
		if err == nil {
			err = archive.Sync()
		}
		closeErr := archive.Close()
		if err != nil || closeErr != nil {
			return fmt.Errorf("could not archive damaged settings; restoration cancelled")
		}
	}
	if err = atomicWrite(path, data); err != nil {
		return fmt.Errorf("could not restore settings; original data and backup are preserved")
	}
	backup.AccessProtected = s.store.AccessProtected || s.access.record != nil
	s.store = backup
	s.inspections = nil
	s.settingsError = ""
	s.maintenanceError = ""
	s.startupWarning = "Restored startup preferences have not been applied. Save preferences to reconcile sign-in startup."
	if s.rpc == nil {
		s.engineError = "Restart the engine to recover restored connections."
	}
	if s.languageChanged != nil {
		s.languageChanged(backup.Settings.Language)
	}
	return nil
}

func (s *Service) VerifyRecovery() (resultErr error) {
	defer func() { s.diagnostics.Operation("recovery", resultErr) }()
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	defer s.mu.Unlock()
	if err := s.persistenceReady(); err != nil {
		return err
	}
	if !s.recoveryHold || s.session != nil || s.exiting {
		return fmt.Errorf("recovery verification is not available")
	}
	known := map[string]bool{}
	for _, c := range s.store.Connections {
		if c.Recovery || s.busy[c.ID] || s.mounted[c.ID] {
			return fmt.Errorf("recover and disconnect restored connections before completing settings recovery")
		}
		known["sd_"+c.Remote] = true
	}
	remotes, err := s.listRemotes()
	if err != nil {
		return err
	}
	available := map[string]bool{}
	for _, name := range remotes {
		available[name] = true
		if generatedRemote(name) && !known[name] {
			return fmt.Errorf("credentials absent from restored settings remain preserved; manual recovery is required")
		}
	}
	if err := s.isolatedRecoveryInventory(known, available); err != nil {
		return err
	}
	for name := range known {
		if !available[name] {
			return fmt.Errorf("credentials for a restored connection are missing; manual recovery is required")
		}
	}
	cache, err := s.cacheRemotes()
	if err != nil {
		return err
	}
	for name := range cache {
		if !known[name] || !available[name] {
			return fmt.Errorf("cache absent from restored settings remains preserved; manual recovery is required")
		}
	}
	if err = os.Remove(filepath.Join(s.dir, recoveryMarker)); err != nil {
		return fmt.Errorf("could not finish recovery; automatic operations remain suspended")
	}
	s.recoveryHold = false
	return nil
}
