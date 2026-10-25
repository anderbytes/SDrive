package core

import (
	"fmt"

	"sdrive/internal/i18n"
	"sdrive/internal/platform"
)

// Language reads only preferences, without querying drives or the engine.
func Language(s *Service) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store.Settings.Language
}

func SetLanguageChanged(s *Service, changed func(string)) { s.languageChanged = changed }

// SetAutoStart installs the desktop callback before publishing the service.
func SetAutoStart(s *Service, update func(bool) error) { s.autoStart = update }

func (s *Service) SaveSettings(settings Settings) error {
	if err := s.accessAllowed(); err != nil {
		return err
	}
	defer s.reconcileDesktopIcons()
	if err := validateCacheDurations(settings); err != nil {
		return err
	}
	if settings.Language == "" {
		settings.Language = "en"
	}
	if !i18n.Valid(settings.Language) {
		return fmt.Errorf("unsupported language")
	}
	if settings.CacheGiB < 1 || settings.CacheGiB > 1024 {
		return fmt.Errorf("cache must be between 1 and 1024 GiB per connection")
	}
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	defer s.mu.Unlock()
	if err := s.persistenceReady(); err != nil {
		return err
	}
	old := s.store.Settings
	if s.exiting {
		return fmt.Errorf("exit in progress")
	}
	if err := s.setAutoStart(settings.AutoStart); err != nil {
		return err
	}
	s.store.Settings = settings
	if err := saveStore(s.dir, s.store); err != nil {
		s.store.Settings = old
		if rollbackErr := s.setAutoStart(old.AutoStart); rollbackErr != nil {
			s.startupWarning = "Windows startup may differ from saved preferences. Save preferences to reconcile it."
			return fmt.Errorf("could not save preferences: %w; Windows startup rollback also failed: %v", err, rollbackErr)
		}
		return err
	}
	s.startupWarning = ""
	if s.languageChanged != nil {
		s.languageChanged(settings.Language)
	}
	return nil
}
func (s *Service) setAutoStart(enabled bool) error {
	if s.autoStart != nil {
		return s.autoStart(enabled)
	}
	return platform.AutoStart(enabled)
}
