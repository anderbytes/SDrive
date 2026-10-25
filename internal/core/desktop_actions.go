package core

// Installed before publishing the service; tests inject isolated desktop operations.
func SetDriveIcons(s *Service, sync func([]Connection) error) { s.syncDriveIcons = sync }

func (s *Service) reconcileDesktopIcons() {
	s.desktopMu.Lock()
	defer s.desktopMu.Unlock()
	s.mu.Lock()
	callback := s.syncDriveIcons
	if callback == nil || s.settingsError != "" {
		s.mu.Unlock()
		return
	}
	drives := []Connection{}
	if s.store.Settings.ShowServiceIcons {
		for _, c := range s.store.Connections {
			if c.MountFolder == "" && s.mounted[c.ID] {
				drives = append(drives, c)
			}
		}
	}
	s.mu.Unlock()
	err := callback(drives)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.desktopWarning = "Explorer service icons could not be updated. Existing customization may take precedence."
	} else {
		s.desktopWarning = ""
	}
}

func (s *Service) mountDesktopActions(id string) {
	s.reconcileDesktopIcons()
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
