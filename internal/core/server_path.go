package core

import (
	"context"
	"encoding/json"
	"net"
	"net/url"
	"strings"
	"time"
)

type serverPathInspection struct {
	remote, provider, path, primary string
	value                           string
	at                              time.Time
	ready                           bool
}

// Only public destination fields reach the UI; credentials never leave the local reader.
func serverPath(c Connection, config map[string]string) string {
	base := ""
	switch c.Provider {
	case "webdav", "nextcloud", "owncloud":
		base = config["url"]
		if c.PrimaryURL != "" {
			base = c.PrimaryURL
		}
	case "s3":
		base = config["endpoint"]
	case "azureblob":
		base = config["sas_url"]
		if account := config["account"]; account != "" && !strings.ContainsAny(account, "/:@?#\\") {
			base = "https://" + account + ".blob.core.windows.net"
		}
	case "sftp", "ftp", "smb":
		host := config["host"]
		if strings.ContainsAny(host, "/\\@?#\x00\r\n") {
			return c.Path
		}
		if port := config["port"]; host != "" && port != "" {
			if strings.Trim(port, "0123456789") != "" {
				return c.Path
			}
			host = net.JoinHostPort(strings.Trim(host, "[]"), port)
		}
		if host == "" {
			return c.Path
		}
		if c.Provider == "smb" {
			return `\\` + host + `\` + strings.ReplaceAll(c.Path, "/", `\`)
		}
		return host + "/" + c.Path
	}
	if base == "" || strings.ContainsAny(base, "\x00\r\n") {
		return c.Path
	}
	if !strings.Contains(base, "://") {
		base = "https://" + base
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return c.Path
	}
	u.User, u.RawQuery, u.Fragment, u.RawFragment = nil, "", "", ""
	u.ForceQuery = false
	u.Path = strings.TrimRight(u.Path, "/") + "/" + c.Path
	u.RawPath = ""
	return u.String()
}

func (s *Service) connectionServerPath(ctx context.Context, c Connection) string {
	s.mu.Lock()
	if s.serverPaths == nil {
		s.serverPaths = map[string]*serverPathInspection{}
	}
	if cached := s.serverPaths[c.ID]; cached != nil && cached.remote == c.Remote && cached.provider == c.Provider && cached.path == c.Path && cached.primary == c.PrimaryURL && (cached.ready || time.Since(cached.at) < 30*time.Second) {
		value := cached.value
		s.mu.Unlock()
		return value
	}
	i := s.index(c.ID)
	if i < 0 || s.store.Connections[i] != c || s.access.locked || s.busy[c.ID] || s.exiting || (s.session != nil && s.session.connection.ID == c.ID) {
		s.mu.Unlock()
		return c.Path
	}
	entry := &serverPathInspection{remote: c.Remote, provider: c.Provider, path: c.Path, primary: c.PrimaryURL, value: c.Path, at: time.Now()}
	s.serverPaths[c.ID] = entry
	needsConfig := c.Provider == "webdav" || c.Provider == "nextcloud" || c.Provider == "owncloud" || c.Provider == "s3" || c.Provider == "azureblob" || c.Provider == "sftp" || c.Provider == "ftp" || c.Provider == "smb"
	if !needsConfig {
		entry.ready = true
		s.mu.Unlock()
		return c.Path
	}
	if !c.SessionOnly {
		if err := s.loadConnectionRPC(c); err != nil {
			s.mu.Unlock()
			return c.Path
		}
	}
	rpc, epoch := s.connectionRPC(c), s.access.epoch
	s.mu.Unlock()
	var config map[string]string
	var err error
	if c.SessionOnly && c.Binding != "" {
		err = json.Unmarshal([]byte(c.Binding), &config)
	} else {
		readCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()
		err = callRPC(readCtx, rpc, "config/get", map[string]string{"name": "sd_" + c.Remote}, &config)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i = s.index(c.ID)
	p, _ := provider(c.Provider)
	if err == nil && config["type"] == p.Backend && s.serverPaths[c.ID] == entry && !s.access.locked && s.access.epoch == epoch && i >= 0 && s.store.Connections[i] == c && s.connectionRPC(c) == rpc {
		entry.value, entry.ready = serverPath(c, config), true
	}
	return entry.value
}
