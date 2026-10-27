package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"sdrive/internal/i18n"
	"sdrive/internal/platform"
)

const portableLimit = 4 << 20

// Portable definitions deliberately have no credential or recovery fields.
type PortableConnection struct {
	HiddenFileVisibility  string            `json:"hiddenFileVisibility,omitempty"`
	OpenMountAfterConnect bool              `json:"openExplorerAfterConnect,omitempty"`
	CardStyle             string            `json:"cardStyle,omitempty"`
	IconImage             string            `json:"iconImage,omitempty"`
	Name                  string            `json:"name"`
	Provider              string            `json:"provider"`
	Path                  string            `json:"path"`
	Letter                string            `json:"letter"`
	MountMode             string            `json:"mountMode"`
	MountFolder           string            `json:"mountFolder"`
	VirtualCapacityGiB    int64             `json:"virtualCapacityGiB,omitempty"`
	DirectoryCacheSeconds int               `json:"directoryCacheSeconds"`
	ReadOnly              bool              `json:"readOnly"`
	AutoMount             bool              `json:"autoMount"`
	AuthMethod            string            `json:"authMethod"`
	SessionOnly           bool              `json:"sessionOnly"`
	UploadLimit           int64             `json:"uploadLimit"`
	DownloadLimit         int64             `json:"downloadLimit"`
	UseProxy              bool              `json:"useProxy"`
	Browser               string            `json:"browser"`
	PrivateBrowser        bool              `json:"privateBrowser"`
	ConnectTimeout        int               `json:"connectTimeout"`
	IdleTimeout           int               `json:"idleTimeout"`
	AlternateURL          string            `json:"alternateURL"`
	Transport             string            `json:"transport"`
	Values                map[string]string `json:"values"`
}

type PortablePreferences struct {
	Theme            string `json:"theme"`
	Language         string `json:"language"`
	CacheGiB         int    `json:"cacheGiB"`
	CacheMaxAgeHours *int   `json:"cacheMaxAgeHours,omitempty"`
	WriteBackSeconds *int   `json:"writeBackSeconds,omitempty"`
	ShowServiceIcons bool   `json:"showServiceIcons"`
}

// Read the retired stripe preference without exporting it or relaxing other fields.
func (p *PortablePreferences) UnmarshalJSON(data []byte) error {
	type preferences PortablePreferences
	value := struct {
		*preferences
		LegacyStripe json.RawMessage `json:"driveStripeColorSource"`
	}{preferences: (*preferences)(p)}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(&value)
}

type PortableFile struct {
	Format      string               `json:"format"`
	Version     int                  `json:"version"`
	Connections []PortableConnection `json:"connections"`
	Preferences PortablePreferences  `json:"preferences"`
}

type ImportDraft struct {
	ID         string             `json:"id"`
	Definition PortableConnection `json:"definition"`
}

type ImportPreview struct {
	ID       string       `json:"id"`
	Document PortableFile `json:"document"`
}

func (d PortableConnection) connection() Connection {
	return Connection{HiddenFileVisibility: d.HiddenFileVisibility, OpenMountAfterConnect: d.OpenMountAfterConnect, CardStyle: d.CardStyle, IconImage: d.IconImage, Name: d.Name, Provider: d.Provider, Path: d.Path, Letter: d.Letter, MountMode: d.MountMode, MountFolder: d.MountFolder, DirectoryCacheSeconds: d.DirectoryCacheSeconds, VirtualCapacityGiB: d.VirtualCapacityGiB, ReadOnly: d.ReadOnly, AutoMount: d.AutoMount, AuthMethod: d.AuthMethod, SessionOnly: d.SessionOnly, UploadLimit: d.UploadLimit, DownloadLimit: d.DownloadLimit, UseProxy: d.UseProxy, Browser: d.Browser, PrivateBrowser: d.PrivateBrowser, ConnectTimeout: d.ConnectTimeout, IdleTimeout: d.IdleTimeout, AlternateURL: d.AlternateURL}
}

func portableDefinition(c Connection) PortableConnection {
	return PortableConnection{HiddenFileVisibility: c.HiddenFileVisibility, OpenMountAfterConnect: c.OpenMountAfterConnect, CardStyle: c.CardStyle, IconImage: c.IconImage, Name: c.Name, Provider: c.Provider, Path: c.Path, Letter: c.Letter, MountMode: c.MountMode, MountFolder: c.MountFolder, DirectoryCacheSeconds: c.DirectoryCacheSeconds, VirtualCapacityGiB: c.VirtualCapacityGiB, ReadOnly: c.ReadOnly, AutoMount: c.AutoMount, AuthMethod: c.AuthMethod, SessionOnly: c.SessionOnly, UploadLimit: c.UploadLimit, DownloadLimit: c.DownloadLimit, UseProxy: c.UseProxy, Browser: c.Browser, PrivateBrowser: c.PrivateBrowser, ConnectTimeout: c.ConnectTimeout, IdleTimeout: c.IdleTimeout, AlternateURL: c.AlternateURL, Values: map[string]string{}}
}

func portablePreferences(settings Settings) PortablePreferences {
	return PortablePreferences{settings.Theme, settings.Language, settings.CacheGiB, settings.CacheMaxAgeHours, settings.WriteBackSeconds, settings.ShowServiceIcons}
}

func (p PortablePreferences) settings(local Settings) Settings {
	local.Theme = p.Theme
	if local.Theme == "" {
		local.Theme = "system"
	}
	local.Language, local.CacheGiB = p.Language, p.CacheGiB
	local.CacheMaxAgeHours, local.WriteBackSeconds = p.CacheMaxAgeHours, p.WriteBackSeconds
	local.ShowServiceIcons = p.ShowServiceIcons
	return local
}

func portableField(providerID, key string) bool {
	p, err := provider(providerID)
	if err != nil {
		return false
	}
	// File paths, access-key identifiers and user-owned OAuth credentials are reentered locally.
	for _, field := range p.Fields {
		if field.Key != key || field.Secret || field.File {
			continue
		}
		switch key {
		case "user", "username", "host", "port", "url", "domain", "provider", "region", "endpoint", "project_number", "root_folder_id", "shared_with_me", "export_formats", "hostname", "disable_epsv", "disable_mlsd":
			return true
		case "account":
			return providerID == "azureblob"
		}
	}
	return false
}

func validatePortableConnection(d PortableConnection) error {
	c := d.connection()
	if err := validatePortableMountTarget(c); err != nil {
		return err
	}
	c.Letter, c.MountFolder, c.MountMode = platform.PortableValidationTarget()
	if err := validateConnection(&c); err != nil {
		return err
	}
	if d.Transport != "" && (d.Provider != "ftp" || (d.Transport != "explicit" && d.Transport != "implicit" && d.Transport != "plain")) {
		return fmt.Errorf("Invalid portable configuration.")
	}
	p, _ := provider(d.Provider)
	for key, value := range d.Values {
		if !portableField(d.Provider, key) || len(value) > 8192 || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("Invalid portable configuration.")
		}
		if value == "" {
			continue
		}
		for _, field := range p.Fields {
			if field.Key == key && len(field.Choices) > 0 {
				valid := false
				for _, choice := range field.Choices {
					if value == choice {
						valid = true
						break
					}
				}
				if !valid {
					return fmt.Errorf("Invalid portable configuration.")
				}
			}
		}
		if key == "url" || key == "endpoint" {
			u, err := url.Parse(value)
			if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return fmt.Errorf("Portable addresses must not contain credentials, query parameters or fragments.")
			}
		}
	}
	return nil
}

// Portable previews accept either operating system; committing still requires a local target.
func validatePortableMountTarget(c Connection) error {
	invalid := fmt.Errorf("Invalid portable configuration.")
	if c.MountMode != "" && c.MountMode != "network" && c.MountMode != "local" {
		return invalid
	}
	if c.MountFolder == "" {
		letter := strings.ToUpper(strings.TrimSpace(c.Letter))
		if len(letter) != 1 || letter[0] < 'D' || letter[0] > 'Z' {
			return invalid
		}
		return nil
	}
	if c.Letter != "" || c.MountMode != "local" || strings.ContainsAny(c.MountFolder, "\x00\r\n") {
		return invalid
	}
	unix := strings.HasPrefix(c.MountFolder, "/") && c.MountFolder != "/"
	windows := len(c.MountFolder) > 3 && ((c.MountFolder[0] >= 'a' && c.MountFolder[0] <= 'z') || (c.MountFolder[0] >= 'A' && c.MountFolder[0] <= 'Z')) && c.MountFolder[1] == ':' && (c.MountFolder[2] == '\\' || c.MountFolder[2] == '/')
	if !unix && !windows {
		return invalid
	}
	for _, part := range strings.Split(strings.ReplaceAll(c.MountFolder, `\`, "/"), "/") {
		if part == "." || part == ".." {
			return invalid
		}
	}
	if (platform.OS == "linux" && unix) || (platform.OS == "windows" && windows) {
		return validateMountTarget(&c)
	}
	return nil
}

func validatePortableDocument(d PortableFile) error {
	b, err := json.Marshal(d)
	if err != nil || len(b) > portableLimit || d.Format != "sdrive-portable" || d.Version != 2 || d.Connections == nil || len(d.Connections) > 1000 {
		return fmt.Errorf("Unsupported or invalid portable configuration.")
	}
	s := d.Preferences.settings(defaultStore().Settings)
	if !validTheme(s.Theme) || !i18n.Valid(s.Language) || s.CacheGiB < 1 || s.CacheGiB > 1024 {
		return fmt.Errorf("Invalid portable preferences.")
	}
	if err := validateCacheDurations(s); err != nil {
		return err
	}
	for _, entry := range d.Connections {
		if err := validatePortableConnection(entry); err != nil {
			return err
		}
	}
	return nil
}

func normalizePortableDocument(d *PortableFile) {
	if d.Preferences.Theme == "" {
		d.Preferences.Theme = "system"
	}
	for i := range d.Connections {
		entry := &d.Connections[i]
		entry.CardStyle = normalizeLegacyCardStyle(entry.CardStyle)
		entry.Name = strings.TrimSpace(entry.Name)
		if entry.MountFolder == "" {
			entry.Letter = strings.ToUpper(strings.TrimSpace(entry.Letter))
		}
		if entry.MountFolder != "" {
			if path, err := platform.LocalPath(entry.MountFolder); err == nil {
				entry.MountFolder = path
			}
		}
	}
}

func decodePortable(b []byte) (PortableFile, error) {
	var d PortableFile
	if len(b) > portableLimit {
		return d, fmt.Errorf("Portable configuration exceeds the size limit.")
	}
	var header struct {
		Version int `json:"version"`
	}
	if json.Unmarshal(b, &header) != nil {
		return d, fmt.Errorf("Unsupported or invalid portable configuration.")
	}
	if header.Version == 1 {
		var err error
		b, err = migrateDrivePreferences(b, true)
		if err != nil {
			return d, fmt.Errorf("Unsupported or invalid portable configuration.")
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&d); err != nil {
		return d, fmt.Errorf("Unsupported or invalid portable configuration.")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return d, fmt.Errorf("Unsupported or invalid portable configuration.")
	}
	normalizePortableDocument(&d)
	return d, validatePortableDocument(d)
}

func SetPortableFilePicker(s *Service, pick func(save bool) (string, error)) { s.pickPortable = pick }

func (s *Service) ExportConfiguration() error {
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	if s.settingsError != "" {
		s.mu.Unlock()
		return fmt.Errorf("Restore valid settings before exporting configuration.")
	}
	connections := append([]Connection{}, s.store.Connections...)
	drafts := append([]ImportDraft{}, s.store.ImportDrafts...)
	d := PortableFile{Format: "sdrive-portable", Version: 2, Connections: []PortableConnection{}, Preferences: portablePreferences(s.store.Settings)}
	pick := s.pickPortable
	s.mu.Unlock()
	if pick == nil {
		return fmt.Errorf("Desktop interface unavailable.")
	}
	for _, c := range connections {
		fields, err := s.ConnectionFields(c.ID)
		if err != nil {
			return fmt.Errorf("Could not read connection definitions. Reauthenticate and retry export.")
		}
		entry := portableDefinition(c)
		entry.Transport = fields.Transport
		for key, value := range fields.Values {
			if value != "" && portableField(c.Provider, key) {
				entry.Values[key] = value
			}
		}
		d.Connections = append(d.Connections, entry)
	}
	for _, draft := range drafts {
		d.Connections = append(d.Connections, draft.Definition)
	}
	s.mu.Lock()
	unchanged := reflect.DeepEqual(connections, s.store.Connections) && reflect.DeepEqual(drafts, append([]ImportDraft{}, s.store.ImportDrafts...)) && reflect.DeepEqual(d.Preferences, portablePreferences(s.store.Settings))
	s.mu.Unlock()
	if !unchanged {
		return fmt.Errorf("Configuration changed during export. Try again.")
	}
	if err := validatePortableDocument(d); err != nil {
		return err
	}
	path, err := pick(true)
	if err != nil || path == "" {
		return err
	}
	s.mu.Lock()
	profile, cache := s.dir, s.dataPath()
	s.mu.Unlock()
	path, err = platform.LocalPath(path)
	if err != nil {
		return err
	}
	if _, err = platform.DirectoryIdentity(filepath.Dir(path)); err != nil {
		return err
	}
	for _, root := range []string{profile, cache} {
		if pathWithin(path, root) {
			return fmt.Errorf("Choose an export file outside the SDrive profile and cache.")
		}
	}
	if info, statErr := os.Lstat(path); statErr == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("Choose a regular local export file.")
	}
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	return atomicWrite(path, b)
}

func (s *Service) PreviewImport() (*ImportPreview, error) {
	if err := s.accessAllowed(); err != nil {
		return nil, err
	}
	if s.pickPortable == nil {
		return nil, fmt.Errorf("Desktop interface unavailable.")
	}
	path, err := s.pickPortable(false)
	if err != nil || path == "" {
		return nil, err
	}
	path, err = platform.LocalPath(path)
	if err != nil {
		return nil, err
	}
	if _, err = platform.DirectoryIdentity(filepath.Dir(path)); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("Could not read portable configuration.")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("Could not read portable configuration.")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, portableLimit+1))
	if err != nil {
		return nil, fmt.Errorf("Could not read portable configuration.")
	}
	d, err := decodePortable(b)
	if err != nil {
		return nil, err
	}
	preview := &ImportPreview{ID: newID(), Document: d}
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	s.importPreview = preview
	s.mu.Unlock()
	return preview, nil
}

func (s *Service) CommitImport(id string, document PortableFile, applyPreferences bool) error {
	if err := s.accessAllowed(); err != nil {
		return err
	}
	normalizePortableDocument(&document)
	if err := validatePortableDocument(document); err != nil {
		return err
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
	if s.recoveryHold || s.exiting || s.session != nil {
		return fmt.Errorf("Complete recovery and setup before importing configuration.")
	}
	if s.importPreview == nil || s.importPreview.ID != id {
		return fmt.Errorf("Preview the import again before saving.")
	}
	existing := append([]Connection{}, s.store.Connections...)
	for _, draft := range s.store.ImportDrafts {
		existing = append(existing, draft.Definition.connection())
	}
	for _, definition := range document.Connections {
		c := definition.connection()
		if err := validateConnection(&c); err != nil {
			return err
		}
		for _, other := range existing {
			if strings.EqualFold(strings.TrimSpace(c.Name), strings.TrimSpace(other.Name)) {
				return &FieldError{Field: "name", Message: "Resolve duplicate names before importing."}
			}
			if mountTargetsOverlap(c, other) {
				return &FieldError{Field: "destination", Message: "Resolve conflicting mount destinations before importing."}
			}
		}
		if err := s.availableMount(c); err != nil {
			return err
		}
		existing = append(existing, c)
	}
	previous := s.store
	s.store.ImportDrafts = append([]ImportDraft{}, s.store.ImportDrafts...)
	for _, definition := range document.Connections {
		s.store.ImportDrafts = append(s.store.ImportDrafts, ImportDraft{ID: newID(), Definition: definition})
	}
	if applyPreferences {
		s.store.Settings = document.Preferences.settings(s.store.Settings)
	}
	if err := saveStore(s.dir, s.store); err != nil {
		s.store = previous
		return err
	}
	s.importPreview = nil
	s.snapshotID++
	if applyPreferences && s.languageChanged != nil {
		s.languageChanged(s.store.Settings.Language)
	}
	return nil
}

func (s *Service) RemoveImportDraft(id string) error {
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	defer s.mu.Unlock()
	if err := s.persistenceReady(); err != nil {
		return err
	}
	if s.recoveryHold || s.exiting || (s.session != nil && s.session.importDraftID == id) {
		return fmt.Errorf("Complete recovery and setup before removing an imported definition.")
	}
	previous := s.store.ImportDrafts
	next := make([]ImportDraft, 0, len(previous))
	found := false
	for _, draft := range previous {
		if draft.ID == id {
			found = true
		} else {
			next = append(next, draft)
		}
	}
	if !found {
		return fmt.Errorf("Imported definition not found.")
	}
	s.store.ImportDrafts = next
	if err := saveStore(s.dir, s.store); err != nil {
		s.store.ImportDrafts = previous
		return err
	}
	s.snapshotID++
	return nil
}
