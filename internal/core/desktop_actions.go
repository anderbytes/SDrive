package core

import "errors"

// Installed before publishing the service; tests inject isolated desktop operations.
func SetDriveIcons(s *Service, sync func([]Connection) error) { s.syncDriveIcons = sync }

func SetDriveLabels(s *Service, sync func([]Connection) error) { s.syncDriveLabels = sync }

func (s *Service) reconcileDesktopAppearance() {
	s.desktopMu.Lock()
	defer s.desktopMu.Unlock()
	s.mu.Lock()
	callback := s.syncDriveIcons
	labels := s.syncDriveLabels
	if (callback == nil && labels == nil) || s.settingsError != "" {
		s.mu.Unlock()
		return
	}
	drives := []Connection{}
	for _, c := range s.store.Connections {
		if c.MountFolder == "" && s.mounted[c.ID] {
			drives = append(drives, c)
		}
	}
	showIcons := s.store.Settings.ShowServiceIcons
	s.mu.Unlock()
	var err error
	if labels != nil {
		err = labels(drives)
	}
	if callback != nil {
		icons := drives
		if !showIcons {
			icons = nil
		}
		err = errors.Join(err, callback(icons))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.desktopWarning = "Explorer drive names or service icons could not be updated. Existing customization may take precedence."
	} else {
		s.desktopWarning = ""
	}
}

func (s *Service) mountDesktopActions(id string) {
	s.reconcileDesktopAppearance()
	s.mu.Lock()
	open := s.store.Settings.OpenExplorerAfterConnect
	s.mu.Unlock()
	if open {
		err := s.OpenExplorer(id)
		s.mu.Lock()
		s.explorerWarning = ""
		if err != nil {
			s.explorerWarning = "The drive connected, but Explorer could not be opened. Use Open in Explorer to retry."
		}
		s.mu.Unlock()
	}
}

func (s *Service) desktopWarnings() string {
	if s.desktopWarning != "" && s.explorerWarning != "" {
		return s.desktopWarning + "\n" + s.explorerWarning
	}
	return s.desktopWarning + s.explorerWarning
}
