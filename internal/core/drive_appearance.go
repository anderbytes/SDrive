package core

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"
	"strings"
)

type DriveAppearance struct {
	CardStyle string `json:"cardStyle,omitempty"`
	IconImage string `json:"iconImage,omitempty"`
}

func validateDriveAppearance(a DriveAppearance) error {
	switch a.CardStyle {
	case "", "blue", "violet", "teal", "amber":
	default:
		return &FieldError{Field: "cardStyle", Message: "Select a supported card style."}
	}
	if a.IconImage == "" {
		return nil
	}
	invalid := &FieldError{Field: "iconImage", Message: "Choose a valid drive image up to 128 × 128 pixels and 128 KiB."}
	const prefix = "data:image/png;base64,"
	if len(a.IconImage) > 128<<10 || !strings.HasPrefix(a.IconImage, prefix) {
		return invalid
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(strings.TrimPrefix(a.IconImage, prefix))
	if err != nil {
		return invalid
	}
	config, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 128 || config.Height > 128 {
		return invalid
	}
	if _, err := png.Decode(bytes.NewReader(raw)); err != nil {
		return invalid
	}
	return nil
}

func (s *Service) UpdateDriveAppearance(id string, appearance DriveAppearance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	if err := s.persistenceReady(); err != nil {
		return err
	}
	if err := validateDriveAppearance(appearance); err != nil {
		return err
	}
	i := s.index(id)
	if i < 0 {
		return fmt.Errorf("connection not found")
	}
	if s.exiting || s.busy[id] || s.starting[s.store.Connections[i].Remote] || (s.session != nil && s.session.connection.ID == id) {
		return fmt.Errorf("connection operation in progress")
	}
	old := s.store.Connections[i]
	s.store.Connections[i].CardStyle, s.store.Connections[i].IconImage = appearance.CardStyle, appearance.IconImage
	if err := saveStore(s.dir, s.store); err != nil {
		s.store.Connections[i] = old
		return err
	}
	s.snapshotID++
	return nil
}
