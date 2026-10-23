package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Folder struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func listFolders(ctx context.Context, rpc RPC, c Connection, path string) ([]Folder, error) {
	c.Path = path
	if err := validateConnection(&c); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var result struct {
		List []struct {
			Name, Path string
			IsDir      bool
		}
	}
	if err := callRPC(ctx, rpc, "operations/list", map[string]any{"fs": "sd_" + c.Remote + ":", "remote": path, "opt": map[string]bool{"dirsOnly": true, "recurse": false}}, &result); err != nil {
		return nil, fmt.Errorf("could not access this folder; check the destination and account permissions")
	}
	if result.List == nil {
		return nil, fmt.Errorf("the service did not return a valid folder list")
	}
	folders := []Folder{}
	for _, item := range result.List {
		if !item.IsDir {
			continue
		}
		full := item.Path
		// Only immediate children of the requested path may become picker targets.
		prefix := ""
		if path != "" {
			prefix = strings.TrimRight(path, "/") + "/"
		}
		if !strings.HasPrefix(full, prefix) {
			continue
		}
		leaf := strings.TrimPrefix(full, prefix)
		if leaf == "" || leaf == "." || leaf == ".." || strings.Contains(leaf, "/") {
			continue
		}
		candidate := c
		candidate.Path = full
		if validateConnection(&candidate) != nil {
			continue
		}
		folders = append(folders, Folder{Name: item.Name, Path: full})
	}
	sort.Slice(folders, func(i, j int) bool { return strings.ToLower(folders[i].Name) < strings.ToLower(folders[j].Name) })
	return folders, nil
}

func (s *Service) ListConfigFolders(id, path string) ([]Folder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss := s.session
	if ss == nil || ss.ID != id || ss.closing || ss.Busy || !ss.Authenticated || ss.Complete {
		return nil, fmt.Errorf("sign in before choosing a folder")
	}
	if ss.inPlace && path != ss.connection.Path {
		return nil, fmt.Errorf("keep the destination during recovery")
	}
	c, rpc := ss.connection, ss.rpc
	s.mu.Unlock()
	folders, err := listFolders(ss.ctx, rpc, c, path)
	s.mu.Lock()
	if s.session != ss || ss.closing || ss.Complete || ss.connection != c {
		return nil, fmt.Errorf("setup changed; choose the folder again")
	}
	return folders, err
}

func (s *Service) ConfirmConfig(id, path, letter string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss := s.session
	if ss == nil || ss.ID != id || ss.closing || ss.Busy || !ss.Authenticated {
		return "", fmt.Errorf("setup is not ready")
	}
	if ss.Complete {
		if letter != ss.connection.Letter {
			i := s.index(ss.connection.ID)
			if i < 0 {
				return "", fmt.Errorf("connection not found")
			}
			c := s.store.Connections[i]
			c.Letter = letter
			if err := s.updateConnection(c); err != nil {
				return "", err
			}
			ss.connection = s.store.Connections[i]
		}
		return ss.connection.ID, nil
	}
	if ss.inPlace && (path != ss.connection.Path || letter != ss.connection.Letter) {
		return "", fmt.Errorf("keep the destination during recovery")
	}
	c, rpc := ss.connection, ss.rpc
	candidate := c
	candidate.Path, candidate.Letter = path, letter
	if err := validateConnection(&candidate); err != nil {
		return "", err
	}
	if err := s.availableLetter(candidate); err != nil {
		return "", err
	}
	ss.Busy = true
	s.mu.Unlock()
	_, err := listFolders(ss.ctx, rpc, c, path)
	s.mu.Lock()
	ss.Busy = false
	if s.session != ss || ss.closing || ss.connection != c {
		return "", fmt.Errorf("setup changed; choose the folder again")
	}
	if err != nil {
		return "", err
	}
	if err := s.availableLetter(candidate); err != nil {
		return "", err
	}
	ss.connection = candidate
	if err := s.commitConfig(ss); err != "" {
		if !ss.Complete {
			ss.connection = c
		}
		ss.Error = err
		return "", fmt.Errorf("%s", err)
	}
	ss.Error = ""
	return ss.connection.ID, nil
}

// Caller holds s.mu; recheck after provider calls because Windows letters can change.
func (s *Service) availableLetter(c Connection) error {
	for _, other := range s.store.Connections {
		if other.ID != c.ID && other.Letter == c.Letter {
			return fmt.Errorf("letter already reserved")
		}
	}
	if s.driveUsed(c.Letter) {
		return fmt.Errorf("letter %s: is already in use", c.Letter)
	}
	return nil
}

func SetFilePicker(s *Service, picker func(string) (string, error)) { s.pickFile = picker }

func (s *Service) SelectCredentialFile(kind string) (string, error) {
	if kind != "key_file" && kind != "service_account_file" {
		return "", fmt.Errorf("invalid file type")
	}
	if s.pickFile == nil {
		return "", fmt.Errorf("file picker unavailable")
	}
	return s.pickFile(kind)
}

func localizeQuestion(q *Question) {
	labels := map[string][2]string{
		"config_type":              {"Account type", "Choose your personal account or workplace."},
		"config_driveid":           {"Library", "Select the drive or library to connect."},
		"config_site":              {"SharePoint site", "Select your organization's site."},
		"config_site_url":          {"SharePoint address", "Paste the site address provided by your organization."},
		"config_search_term":       {"Search for a site", "Enter part of your organization's site name."},
		"config_drive_ok":          {"Confirm library", "Connect the selected library?"},
		"config_team_drive":        {"Shared drive", "Select your organization's shared drive."},
		"config_team_drive_id":     {"Shared drive", "Select the shared drive."},
		"config_change_team_drive": {"Shared drive", "Change the shared drive?"},
		"config_mailbox_password":  {"Mailbox password", "Enter the additional mailbox password if your account uses a separate password."},
		"config_refresh_token":     {"Renew access", "Sign in again to renew authorization?"},
		"config_is_local":          {"Sign in through your browser", "Open this computer's browser to authorize access."},
	}
	label, ok := labels[q.Name]
	name := strings.ToLower(q.Name)
	if !ok && verificationQuestion(name) {
		label = [2]string{"Verification code", "Enter the authentication code requested by your account."}
		ok = true
	}
	if !ok {
		q.Name = "Unrecognized step"
		q.Help = "This service requested a step that this version of SDrive does not support yet. Cancel and report the service to project support."
		q.Unsupported = true
		q.DefaultStr = ""
		q.Examples = nil
		return
	}
	q.Name, q.Help = label[0], label[1]
	for i := range q.Examples {
		e := &q.Examples[i]
		switch strings.ToLower(e.Help) {
		case "yes", "true":
			e.Help = "Yes"
		case "no", "false":
			e.Help = "No"
		case "onedrive personal or business":
			e.Help = "OneDrive personal or business"
		case "sharepoint site":
			e.Help = "SharePoint site"
		case "search for a sharepoint site":
			e.Help = "Search for a SharePoint site"
		}
		if name == "config_type" {
			labels := map[string]string{"onedrive": "OneDrive personal or business", "sharepoint": "SharePoint root site", "url": "Enter SharePoint address", "search": "Search for a SharePoint site", "driveid": "Drive ID (advanced)", "siteid": "Site ID (advanced)", "path": "SharePoint path (advanced)"}
			if text, ok := labels[e.Value]; ok {
				e.Help = text
			} else {
				e.Help = "Advanced option: " + e.Value
			}
		}
	}
}

func verificationQuestion(name string) bool {
	name = strings.ToLower(name)
	for _, term := range []string{"2fa", "otp", "verification", "mfa"} {
		if strings.Contains(name, term) {
			return true
		}
	}
	return false
}

func sensitiveQuestion(name string) bool {
	name = strings.ToLower(name)
	if verificationQuestion(name) {
		return true
	}
	for _, term := range []string{"token", "password", "secret"} {
		if strings.Contains(name, term) {
			return true
		}
	}
	return false
}

func validateConfiguredRemote(p Provider, config map[string]string) error {
	invalid := fmt.Errorf("the engine did not save a valid account configuration; cancel and try again")
	if config == nil || config["type"] != p.Backend {
		return invalid
	}
	for _, f := range p.Fields {
		if f.Required && strings.TrimSpace(config[f.Key]) == "" {
			return invalid
		}
	}
	if p.OAuth && config["token"] == "" && !(p.ID == "gcs" && config["service_account_file"] != "") {
		return invalid
	}
	if p.ID == "azureblob" && config["key"] == "" && config["sas_url"] == "" {
		return invalid
	}
	if p.ID == "ftp" {
		if _, err := ftpTransport(config); err != nil {
			return invalid
		}
	}
	return nil
}

var errInvalidConfigResult = errors.New("invalid configuration result")

func setupFailure(err error) string {
	if errors.Is(err, errInvalidConfigResult) {
		return "The engine returned an invalid setup response. Cancel and try again."
	}
	var failure interface{ FailureKind() string }
	if errors.As(err, &failure) {
		switch failure.FailureKind() {
		case "network":
			return "Could not reach the storage service. Check your internet connection and try again."
		case "credentials":
			return "The service rejected your credentials. Check your account details and sign in again."
		case "permission":
			return "Your account does not have the required permission. Check access with your storage administrator."
		case "engine":
			return "Could not communicate with the local engine. Cancel setup and restart the engine."
		case "invalid":
			return "The engine returned an invalid setup response. Cancel and try again."
		}
	}
	return "Could not sign in. Check your credentials, internet connection and service permissions."
}
