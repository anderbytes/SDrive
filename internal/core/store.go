package core

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sdrive/internal/i18n"
	"sdrive/internal/platform"
	"strings"
	"unicode/utf8"
)

type Connection struct {
	HiddenFileVisibility  string `json:"hiddenFileVisibility,omitempty"`
	OpenMountAfterConnect bool   `json:"openExplorerAfterConnect,omitempty"`
	CardStyle             string `json:"cardStyle,omitempty"`
	IconImage             string `json:"iconImage,omitempty"`
	ID                    string `json:"id"`
	Remote                string `json:"remote"`
	Name                  string `json:"name"`
	Provider              string `json:"provider"`
	Path                  string `json:"path"`
	Letter                string `json:"letter"`
	MountMode             string `json:"mountMode,omitempty"`
	MountFolder           string `json:"mountFolder,omitempty"`
	VirtualCapacityGiB    int64  `json:"virtualCapacityGiB,omitempty"`
	DirectoryCacheSeconds int    `json:"directoryCacheSeconds,omitempty"`
	ReadOnly              bool   `json:"readOnly"`
	AutoMount             bool   `json:"autoMount"`
	Recovery              bool   `json:"recovery"`
	TargetHash            string `json:"targetHash"`
	AuthMethod            string `json:"authMethod,omitempty"`
	SessionOnly           bool   `json:"sessionOnly,omitempty"`
	UploadLimit           int64  `json:"uploadLimit,omitempty"`
	DownloadLimit         int64  `json:"downloadLimit,omitempty"`
	UseProxy              bool   `json:"useProxy,omitempty"`
	Browser               string `json:"browser,omitempty"`
	PrivateBrowser        bool   `json:"privateBrowser,omitempty"`
	ConnectTimeout        int    `json:"connectTimeout,omitempty"`
	IdleTimeout           int    `json:"idleTimeout,omitempty"`
	AlternateURL          string `json:"alternateURL,omitempty"`
	ResourceID            string `json:"resourceID,omitempty"`
	PrimaryURL            string `json:"primaryURL,omitempty"`
	Binding               string `json:"binding,omitempty"`
}
type Settings struct {
	Density          string `json:"density"`
	Theme            string `json:"theme"`
	ShowServiceIcons bool   `json:"showServiceIcons"`
	Language         string `json:"language"`
	AutoStart        bool   `json:"autoStart"`
	CacheGiB         int    `json:"cacheGiB"`
	CacheMaxAgeHours *int   `json:"cacheMaxAgeHours,omitempty"`
	WriteBackSeconds *int   `json:"writeBackSeconds,omitempty"`
}
type Store struct {
	AccessProtected bool          `json:"accessProtected,omitempty"`
	ImportDrafts    []ImportDraft `json:"importDrafts,omitempty"`
	Connections     []Connection  `json:"connections"`
	Settings        Settings      `json:"settings"`
}

var errSettingsVersion = errors.New("unsupported settings format; preserve the data directory and use a compatible SDrive version")

func defaultStore() Store {
	return Store{Connections: []Connection{}, Settings: Settings{CacheGiB: 2, Language: "en", Theme: "system", Density: "balanced"}}
}

func loadStore(dir string) (Store, error) {
	b, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if os.IsNotExist(err) {
		return defaultStore(), nil
	}
	if err != nil {
		return defaultStore(), err
	}
	return decodeStore(b)
}

func decodeStore(b []byte) (Store, error) {
	s := defaultStore()
	var shape struct {
		Version     json.RawMessage `json:"version"`
		Connections json.RawMessage `json:"connections"`
		Settings    *struct {
			AutoStart *bool `json:"autoStart"`
			CacheGiB  *int  `json:"cacheGiB"`
		} `json:"settings"`
	}
	invalid := fmt.Errorf("invalid local settings; preserve settings.json for recovery")
	if json.Unmarshal(b, &shape) != nil {
		return s, invalid
	}
	version := 0
	if shape.Version != nil {
		if bytes.Equal(bytes.TrimSpace(shape.Version), []byte("null")) || json.Unmarshal(shape.Version, &version) != nil || version < 1 || version > 6 {
			return s, errSettingsVersion
		}
	}
	if version < 6 {
		var migrationErr error
		b, migrationErr = migrateDrivePreferences(b, false)
		if migrationErr != nil {
			return s, invalid
		}
	}
	connections := bytes.TrimSpace(shape.Connections)
	if len(connections) == 0 || connections[0] != '[' || shape.Settings == nil || shape.Settings.AutoStart == nil || shape.Settings.CacheGiB == nil {
		return s, invalid
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, invalid
	}
	var err error
	if !validDensity(s.Settings.Density) {
		s.Settings.Density = "balanced"
	}
	if !validTheme(s.Settings.Theme) {
		s.Settings.Theme = "system"
	}
	if !i18n.Valid(s.Settings.Language) {
		s.Settings.Language = "en"
	}
	if s.Settings.CacheGiB < 1 || s.Settings.CacheGiB > 1024 {
		return s, fmt.Errorf("invalid cache limit in settings")
	}
	if err := validateCacheDurations(s.Settings); err != nil {
		return s, err
	}
	ids := map[string]bool{}
	destinations := map[string]bool{}
	for i := range s.Connections {
		c := &s.Connections[i]
		c.CardStyle = normalizeLegacyCardStyle(c.CardStyle)
		if err = validateConnection(c); err != nil {
			return s, fmt.Errorf("invalid connection in settings: %w", err)
		}
		for _, value := range []string{c.ID, c.Remote} {
			decoded, e := hex.DecodeString(value)
			if e != nil || len(decoded) != 16 {
				return s, fmt.Errorf("invalid identifier in settings")
			}
		}
		if ids[c.ID] || destinations[mountKey(mountPoint(*c))] {
			return s, fmt.Errorf("duplicate connection or mount destination in settings")
		}
		ids[c.ID] = true
		destinations[mountKey(mountPoint(*c))] = true
		for j := 0; j < i; j++ {
			other := s.Connections[j]
			if mountTargetsOverlap(*c, other) {
				return s, fmt.Errorf("overlapping mount destinations in settings")
			}
		}
	}
	for i := range s.ImportDrafts {
		draft := &s.ImportDrafts[i]
		draft.Definition.CardStyle = normalizeLegacyCardStyle(draft.Definition.CardStyle)
		if len(draft.ID) != 32 || ids[draft.ID] {
			return s, invalid
		}
		if _, err := hex.DecodeString(draft.ID); err != nil {
			return s, invalid
		}
		ids[draft.ID] = true
		if err := validatePortableConnection(draft.Definition); err != nil {
			return s, invalid
		}
	}
	return s, nil
}

func encodeStore(s Store) ([]byte, error) {
	if s.Connections == nil {
		s.Connections = []Connection{}
	}
	b, err := json.MarshalIndent(struct {
		Version int `json:"version"`
		Store
	}{Version: 6, Store: s}, "", "  ")
	if err != nil {
		return nil, err
	}
	if _, err = decodeStore(b); err != nil {
		return nil, err
	}
	return b, nil
}

func saveStore(dir string, s Store) error {
	b, err := encodeStore(s)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "settings.json")
	previous, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		if _, err = decodeStore(previous); err != nil {
			return err
		}
	} else {
		previous = b
	}
	if err = ensureCacheLocation(dir); err != nil {
		return err
	}
	if err = atomicWrite(filepath.Join(dir, "settings.backup.json"), previous); err != nil {
		return fmt.Errorf("could not preserve settings backup: %w", err)
	}
	return atomicWrite(path, b)
}

func atomicWrite(path string, data []byte) error { return platform.AtomicWrite(path, data) }

func validateConnection(c *Connection) error {
	if c.HiddenFileVisibility != "" && c.HiddenFileVisibility != "system" && c.HiddenFileVisibility != "show" && c.HiddenFileVisibility != "hide" {
		return &FieldError{Field: "hiddenFileVisibility", Message: "Select a valid hidden file visibility mode."}
	}
	if err := validateDriveAppearance(DriveAppearance{c.CardStyle, c.IconImage}); err != nil {
		return err
	}
	if err := validateNetwork(*c); err != nil {
		return err
	}
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" || utf8.RuneCountInString(c.Name) > 80 {
		return &FieldError{Field: "name", Message: "name must contain between 1 and 80 characters"}
	}
	if strings.ContainsAny(c.Name, "\x00\r\n") {
		return &FieldError{Field: "name", Message: "name must not contain NUL or line breaks"}
	}
	if err := validateMountTarget(c); err != nil {
		return err
	}
	if strings.ContainsAny(c.Path, "\x00\r\n\\:") || strings.HasPrefix(c.Path, "/") {
		return &FieldError{Field: "path", Message: "enter a relative remote path, separated by /"}
	}
	for _, part := range strings.Split(c.Path, "/") {
		if part == ".." {
			return &FieldError{Field: "path", Message: "path must not contain .."}
		}
	}
	_, err := provider(c.Provider)
	return err
}
