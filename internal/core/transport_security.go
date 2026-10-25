package core

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// TransportSecurityView describes transport protection, not encryption at rest.
type TransportSecurityView struct {
	State     string     `json:"state"`
	Protocol  string     `json:"protocol"`
	Source    string     `json:"source"`
	CheckedAt *time.Time `json:"checkedAt"`
}

type transportInspection struct {
	connection Connection
	rpc        RPC
	at         time.Time
	view       TransportSecurityView
}

func (s *Service) sampleTransportSecurity(ctx context.Context, c Connection, rpc RPC) TransportSecurityView {
	unknown := TransportSecurityView{State: "unknown"}
	s.mu.Lock()
	i := s.index(c.ID)
	if i < 0 || s.store.Connections[i] != c || s.busy[c.ID] || s.connectionRPC(c) != rpc || !s.mounted[c.ID] || rpc == nil || !rpc.Alive() {
		s.mu.Unlock()
		return unknown
	}
	if previous := s.transportInspections[c.ID]; previous != nil && previous.connection == c && previous.rpc == rpc && time.Since(previous.at) < 30*time.Second {
		view := previous.view
		s.mu.Unlock()
		if view.State == "" {
			return unknown
		}
		return view
	}
	if s.transportInspections == nil {
		s.transportInspections = map[string]*transportInspection{}
	}
	inspection := &transportInspection{connection: c, rpc: rpc, at: time.Now()}
	s.transportInspections[c.ID] = inspection
	s.mu.Unlock()
	checkCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	view := readTransportSecurity(checkCtx, rpc, c)
	view.CheckedAt = &inspection.at
	s.mu.Lock()
	defer s.mu.Unlock()
	i = s.index(c.ID)
	if s.transportInspections[c.ID] != inspection || i < 0 || s.store.Connections[i] != c || s.connectionRPC(c) != rpc || !rpc.Alive() || !s.mounted[c.ID] || s.busy[c.ID] {
		return unknown
	}
	inspection.view = view
	return view
}

func readTransportSecurity(ctx context.Context, rpc RPC, c Connection) TransportSecurityView {
	unknown := TransportSecurityView{State: "unknown"}
	p, err := provider(c.Provider)
	if err != nil {
		return unknown
	}
	var config map[string]string
	if callRPC(ctx, rpc, "config/get", map[string]string{"name": "sd_" + c.Remote}, &config) != nil || config["type"] != p.Backend {
		return unknown
	}
	for key := range config {
		if strings.HasPrefix(key, "override.") {
			return unknown
		}
	}
	view := TransportSecurityView{State: "encrypted", Protocol: "https", Source: "configuration"}
	switch p.ID {
	case "smb":
		var response struct {
			Result *struct {
				Encrypted *bool `json:"encrypted"`
			} `json:"result"`
		}
		if callRPC(ctx, rpc, "backend/command", map[string]string{"fs": remote(c), "command": "transport-security"}, &response) != nil || response.Result == nil || response.Result.Encrypted == nil {
			return unknown
		}
		view.Protocol, view.Source = "smb", "negotiated"
		if !*response.Result.Encrypted {
			view.State = "unencrypted"
		}
	case "sftp":
		view.Protocol, view.Source = "ssh", "protocol"
	case "ftp":
		mode, err := ftpTransport(config)
		if err != nil {
			return unknown
		}
		view.Protocol = "ftps"
		if mode == "plain" {
			view.State, view.Protocol = "unencrypted", "ftp"
		}
	case "mega":
		secure := false
		if config["use_https"] != "" {
			secure, err = strconv.ParseBool(config["use_https"])
			if err != nil {
				return unknown
			}
		}
		if !secure {
			view.State, view.Protocol = "unencrypted", "http"
		}
	case "nextcloud", "owncloud", "webdav":
		return transportURL(config["url"])
	case "s3":
		if config["endpoint"] != "" {
			return transportURL(config["endpoint"])
		}
	case "azureblob":
		emulator, envAuth := false, false
		for key, value := range map[string]*bool{"use_emulator": &emulator, "env_auth": &envAuth} {
			if config[key] != "" {
				*value, err = strconv.ParseBool(config[key])
				if err != nil {
					return unknown
				}
			}
		}
		// Azure prioritizes environment, emulator and account keys ahead of a SAS URL.
		if !envAuth && !emulator && !(config["account"] != "" && config["key"] != "") && config["sas_url"] != "" {
			return transportURL(config["sas_url"])
		}
		if config["endpoint"] != "" {
			return transportURL(config["endpoint"])
		}
		if emulator && !envAuth {
			view.State, view.Protocol = "unencrypted", "http"
		}
	default:
		view.Source = "protocol"
	}
	return view
}

func transportURL(value string) TransportSecurityView {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return TransportSecurityView{State: "unknown"}
	}
	state := "encrypted"
	if u.Scheme == "http" {
		state = "unencrypted"
	}
	return TransportSecurityView{State: state, Protocol: u.Scheme, Source: "configuration"}
}
