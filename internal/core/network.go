package core

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sdrive/internal/platform"
	"strings"
)

func webDAV(id string) bool { return id == "webdav" || id == "nextcloud" || id == "owncloud" }
func proxySupported(id string) bool {
	switch id {
	case "drive", "onedrive", "sharepoint", "dropbox", "box", "pcloud", "webdav", "nextcloud", "owncloud", "s3", "b2", "azureblob", "gcs":
		return true
	}
	return false
}

func validateNetwork(c Connection) error {
	invalid := func(key, text string) error { return &FieldError{Field: key, Message: text} }
	if c.UploadLimit < 0 || c.DownloadLimit < 0 || c.UploadLimit > 1<<40 || c.DownloadLimit > 1<<40 {
		return invalid("bandwidth", "Bandwidth must be between 0 and 1 TiB/s.")
	}
	if (c.UploadLimit != 0 || c.DownloadLimit != 0) && !proxySupported(c.Provider) {
		return invalid("bandwidth", "Independent bandwidth limits are supported for HTTP storage services only.")
	}
	if c.UseProxy && !proxySupported(c.Provider) {
		return invalid("useProxy", "Proxy routing is not supported for this service.")
	}
	if (c.ConnectTimeout != 0 || c.IdleTimeout != 0) && !webDAV(c.Provider) {
		return invalid("connectTimeout", "Timeout controls are available for WebDAV connections only.")
	}
	if c.ConnectTimeout < 0 || c.ConnectTimeout > 3600 || c.IdleTimeout < 0 || c.IdleTimeout > 3600 {
		return invalid("connectTimeout", "Timeout must be between 0 and 3600 seconds.")
	}
	switch c.Browser {
	case "", "edge", "chrome", "firefox":
	default:
		return invalid("browser", "Select an installed browser.")
	}
	if c.PrivateBrowser && c.Browser == "" {
		return invalid("browser", "Select a browser to use a private window.")
	}
	switch c.AuthMethod {
	case "", "oauth", "serviceAccount":
	default:
		return invalid("authMethod", "Select a supported authentication method.")
	}
	if c.AuthMethod == "serviceAccount" && c.Provider != "gcs" {
		return invalid("authMethod", "Service-account selection is available for Google Cloud Storage only.")
	}
	if c.SessionOnly {
		if c.AutoMount {
			return invalid("autoMount", "Session-only connections require manual sign-in.")
		}
		if !webDAV(c.Provider) && c.Provider != "drive" && c.Provider != "onedrive" && c.Provider != "sharepoint" {
			return invalid("sessionOnly", "Session-only sign-in requires a verifiable destination for this service.")
		}
	}
	if c.AlternateURL != "" {
		u, err := url.Parse(c.AlternateURL)
		if !webDAV(c.Provider) || err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(c.AlternateURL, "\x00\r\n") || len(c.AlternateURL) > 8192 {
			return invalid("alternateURL", "Enter an HTTPS WebDAV address without credentials, query or fragment.")
		}
	}
	if len(c.ResourceID) > 2048 || strings.ContainsAny(c.ResourceID, "\x00\r\n") {
		return invalid("alternateURL", "Invalid verified destination identity.")
	}
	return nil
}

type ProxyConfig struct {
	Address     string `json:"address"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	HasPassword bool   `json:"hasPassword"`
}

func (p ProxyConfig) URL() string {
	if p.Address == "" {
		return ""
	}
	u, _ := url.Parse(p.Address)
	if u == nil {
		return ""
	}
	if p.Username != "" {
		u.User = url.UserPassword(p.Username, p.Password)
	}
	return u.String()
}
func proxyCipher(dir string) (cipher.AEAD, error) {
	if _, err := os.Stat(filepath.Join(dir, "proxy.enc")); err == nil {
		if _, err = os.Stat(filepath.Join(dir, "key.dpapi")); err != nil {
			return nil, fmt.Errorf("Could not unlock proxy settings.")
		}
	}
	key, err := platform.Key(dir)
	if err != nil {
		return nil, err
	}
	raw, err := base64.RawStdEncoding.DecodeString(key)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func (s *Service) readProxy() (ProxyConfig, error) {
	var p ProxyConfig
	b, err := os.ReadFile(filepath.Join(s.dir, "proxy.enc"))
	if os.IsNotExist(err) {
		return p, nil
	}
	if err != nil {
		return p, fmt.Errorf("Could not unlock proxy settings.")
	}
	gcm, err := proxyCipher(s.dir)
	if err != nil {
		return p, err
	}
	if len(b) < gcm.NonceSize() {
		return p, fmt.Errorf("Could not unlock proxy settings.")
	}
	raw, err := gcm.Open(nil, b[:gcm.NonceSize()], b[gcm.NonceSize():], []byte("sdrive-proxy-v1"))
	if err != nil || json.Unmarshal(raw, &p) != nil {
		return ProxyConfig{}, fmt.Errorf("Could not unlock proxy settings.")
	}
	return p, nil
}
func (s *Service) ProxySettings() (ProxyConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.readProxy()
	p.HasPassword = p.Password != ""
	p.Password = ""
	return p, err
}
func (s *Service) SaveProxy(p ProxyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.persistenceReady(); err != nil {
		return err
	}
	if s.exiting {
		return fmt.Errorf("exit in progress")
	}
	if p.Address != "" {
		u, err := url.Parse(p.Address)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(p.Address+p.Username+p.Password, "\x00\r\n") || len(p.Address+p.Username+p.Password) > 32768 {
			return fmt.Errorf("Enter an HTTP or HTTPS proxy address without embedded credentials.")
		}
		if p.Password == "" && p.HasPassword {
			old, err := s.readProxy()
			if err != nil {
				return err
			}
			if old.Address == p.Address && old.Username == p.Username {
				p.Password = old.Password
			}
		}
	} else {
		p.Username = ""
		p.Password = ""
	}
	gcm, err := proxyCipher(s.dir)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(s.dir, "proxy.enc"), gcm.Seal(nonce, nonce, raw, []byte("sdrive-proxy-v1")))
}
