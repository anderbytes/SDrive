package core

// DisconnectProgress is an internal desktop notification, not a bridge response.
type DisconnectProgress struct {
	Sequence uint64
	ID       string
	Name     string
	Stage    string
}

// SetDisconnectProgress is installed before publication. The callback must not
// wait for the desktop thread or call back into the service.
func SetDisconnectProgress(s *Service, changed func(DisconnectProgress)) {
	s.disconnectChanged = changed
}

// Caller holds s.mu; notifications preserve the order of lifecycle transitions.
func (s *Service) notifyDisconnect(c Connection, stage string) {
	if s.disconnectChanged != nil {
		s.disconnectSequence++
		s.disconnectChanged(DisconnectProgress{Sequence: s.disconnectSequence, ID: c.ID, Name: c.Name, Stage: stage})
	}
}
