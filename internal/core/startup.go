package core

import (
	"fmt"
	"os"
	"path/filepath"

	"sdrive/internal/platform"
)

// RestoreStartup applies only the saved Windows integration, without starting a service.
func RestoreStartup(dir string) error {
	return restoreStartup(dir, platform.AutoStartMatches, platform.AutoStart)
}

func restoreStartup(dir string, matches func(bool) (bool, error), update func(bool) error) error {
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	store, err := loadStore(dir)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, recoveryMarker)); !os.IsNotExist(err) {
		return fmt.Errorf("resolve settings recovery before restoring Windows startup")
	}
	for _, connection := range store.Connections {
		if connection.Recovery {
			return fmt.Errorf("resolve connection recovery before restoring Windows startup")
		}
	}
	ok, err := matches(store.Settings.AutoStart)
	if err != nil || ok {
		return err
	}
	if err := update(store.Settings.AutoStart); err != nil {
		return err
	}
	ok, err = matches(store.Settings.AutoStart)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("Windows startup still differs from saved preferences")
	}
	return nil
}

// ReconcileStartup runs after desktop callbacks are configured, never in New.
func ReconcileStartup(s *Service) error {
	return reconcileStartup(s, platform.AutoStartMatches)
}

func reconcileStartup(s *Service, matches func(bool) (bool, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	if s.settingsError != "" || s.recoveryHold {
		err = fmt.Errorf("resolve settings recovery before restoring Windows startup")
	} else {
		err = restoreStartup(s.dir, matches, s.setAutoStart)
	}
	if err != nil {
		s.startupWarning = "Windows startup may differ from saved preferences. Save preferences to reconcile it."
	} else {
		s.startupWarning = ""
	}
	return err
}
