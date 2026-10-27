package core

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/png"
	"strings"
)

type DriveAppearance struct {
	CardStyle string `json:"cardStyle,omitempty"`
	IconImage string `json:"iconImage,omitempty"`
}

//go:embed drive_palette.json
var drivePaletteJSON []byte

var drivePalette [][3]string

func init() {
	if err := json.Unmarshal(drivePaletteJSON, &drivePalette); err != nil {
		panic(err)
	}
}

// Legacy RGB values are converted only while reading existing configuration.
func normalizeLegacyCardStyle(style string) string {
	if len(style) != 7 || style[0] != '#' {
		return style
	}
	rgb, err := hex.DecodeString(style[1:])
	if err != nil {
		return style
	}
	if max(rgb[0], rgb[1], rgb[2])-min(rgb[0], rgb[1], rgb[2]) <= 24 {
		return "gray"
	}
	nearest, distance := "", 3*255*255+1
	for _, entry := range drivePalette {
		if entry[2] == "" {
			continue
		}
		candidate, _ := hex.DecodeString(entry[2][1:])
		squared := 0
		for channel, value := range rgb {
			delta := int(value) - int(candidate[channel])
			squared += delta * delta
		}
		if squared < distance {
			nearest, distance = entry[0], squared
		}
	}
	return nearest
}

func validateDriveAppearance(a DriveAppearance) error {
	valid := false
	for _, entry := range drivePalette {
		if entry[0] == a.CardStyle {
			valid = true
			break
		}
	}
	if !valid {
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
