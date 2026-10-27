package core

import (
	"encoding/json"
	"fmt"
)

func hiddenFileVisibility(c Connection) string {
	if c.HiddenFileVisibility == "" {
		return "system"
	}
	return c.HiddenFileVisibility
}

// Old false meant Explorer-controlled attributes, never forced exclusion.
func migrateDrivePreferences(data []byte, portable bool) ([]byte, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	key := "settings"
	if portable {
		key = "preferences"
	}
	var preferences map[string]json.RawMessage
	if err := json.Unmarshal(document[key], &preferences); err != nil {
		return nil, err
	}
	open := false
	if raw, ok := preferences["openExplorerAfterConnect"]; ok {
		if err := json.Unmarshal(raw, &open); err != nil {
			return nil, err
		}
	}
	delete(preferences, "openExplorerAfterConnect")
	document[key], _ = json.Marshal(preferences)
	migrate := func(raw json.RawMessage) (json.RawMessage, error) {
		var c map[string]json.RawMessage
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, err
		}
		if c == nil {
			return nil, fmt.Errorf("invalid connection")
		}
		if _, ok := c["hiddenFileVisibility"]; !ok {
			show := true
			if value, exists := c["showHiddenFiles"]; exists {
				if err := json.Unmarshal(value, &show); err != nil {
					return nil, err
				}
			}
			mode := "show"
			if !show {
				mode = "system"
			}
			c["hiddenFileVisibility"], _ = json.Marshal(mode)
		}
		delete(c, "showHiddenFiles")
		if _, ok := c["openExplorerAfterConnect"]; !ok {
			c["openExplorerAfterConnect"], _ = json.Marshal(open)
		}
		return json.Marshal(c)
	}
	var connections []json.RawMessage
	if err := json.Unmarshal(document["connections"], &connections); err != nil {
		return nil, err
	}
	for i, c := range connections {
		value, err := migrate(c)
		if err != nil {
			return nil, err
		}
		connections[i] = value
	}
	document["connections"], _ = json.Marshal(connections)
	if raw, ok := document["importDrafts"]; ok {
		var drafts []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &drafts); err != nil {
			return nil, err
		}
		for _, draft := range drafts {
			value, err := migrate(draft["definition"])
			if err != nil {
				return nil, err
			}
			draft["definition"] = value
		}
		document["importDrafts"], _ = json.Marshal(drafts)
	}
	if portable {
		document["version"] = json.RawMessage("2")
	}
	return json.Marshal(document)
}

func (s *Service) ReorderConnections(ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	if err := s.persistenceReady(); err != nil {
		return err
	}
	if s.exiting || s.recoveryHold {
		return fmt.Errorf("Connection order cannot be changed right now.")
	}
	if len(ids) != len(s.store.Connections) {
		return fmt.Errorf("The drive list changed. Try again.")
	}
	ordered := make([]Connection, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		i := s.index(id)
		if i < 0 || seen[id] {
			return fmt.Errorf("The drive list changed. Try again.")
		}
		seen[id] = true
		ordered = append(ordered, s.store.Connections[i])
	}
	previous := s.store.Connections
	s.store.Connections = ordered
	if err := saveStore(s.dir, s.store); err != nil {
		s.store.Connections = previous
		return err
	}
	s.snapshotID++
	return nil
}
