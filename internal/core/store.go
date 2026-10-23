package core

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sdrive/internal/i18n"
	"strings"
)

type Connection struct {
	ID         string `json:"id"`
	Remote     string `json:"remote"`
	Name       string `json:"name"`
	Provider   string `json:"provider"`
	Path       string `json:"path"`
	Letter     string `json:"letter"`
	ReadOnly   bool   `json:"readOnly"`
	AutoMount  bool   `json:"autoMount"`
	Recovery   bool   `json:"recovery"`
	TargetHash string `json:"targetHash"`
}
type Settings struct {
	Language  string `json:"language"`
	AutoStart bool   `json:"autoStart"`
	CacheGiB  int    `json:"cacheGiB"`
}
type Store struct {
	Connections []Connection `json:"connections"`
	Settings    Settings     `json:"settings"`
}

func loadStore(dir string) (Store, error) {
	s := Store{Connections: []Connection{}, Settings: Settings{CacheGiB: 2, Language: "en"}}
	b, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("invalid local settings; preserve settings.json for recovery")
	}
	if !i18n.Valid(s.Settings.Language) {
		s.Settings.Language = "en"
	}
	if s.Settings.CacheGiB < 1 || s.Settings.CacheGiB > 1024 {
		return s, fmt.Errorf("invalid cache limit in settings")
	}
	ids := map[string]bool{}
	letters := map[string]bool{}
	for i := range s.Connections {
		c := &s.Connections[i]
		if err = validateConnection(c); err != nil {
			return s, fmt.Errorf("invalid connection in settings: %w", err)
		}
		for _, value := range []string{c.ID, c.Remote} {
			decoded, e := hex.DecodeString(value)
			if e != nil || len(decoded) != 16 {
				return s, fmt.Errorf("invalid identifier in settings")
			}
		}
		if ids[c.ID] || letters[c.Letter] {
			return s, fmt.Errorf("duplicate connection or drive letter in settings")
		}
		ids[c.ID] = true
		letters[c.Letter] = true
	}
	return s, nil
}

func saveStore(dir string, s Store) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dir, "settings.json"), b)
}

func atomicWrite(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".sdrive-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return replaceFile(temp, path)
}

func validateConnection(c *Connection) error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" || len(c.Name) > 80 {
		return fmt.Errorf("name must contain between 1 and 80 characters")
	}
	c.Letter = strings.ToUpper(strings.TrimSpace(c.Letter))
	if len(c.Letter) != 1 || c.Letter[0] < 'D' || c.Letter[0] > 'Z' {
		return fmt.Errorf("choose a letter between D and Z")
	}
	if strings.ContainsAny(c.Name+c.Path, "\x00\r\n") || strings.ContainsAny(c.Path, "\\:") || strings.HasPrefix(c.Path, "/") {
		return fmt.Errorf("enter a relative remote path, separated by /")
	}
	for _, part := range strings.Split(c.Path, "/") {
		if part == ".." {
			return fmt.Errorf("path must not contain ..")
		}
	}
	_, err := provider(c.Provider)
	return err
}
