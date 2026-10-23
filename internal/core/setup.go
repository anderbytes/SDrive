package core

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Folder struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func (s *Service) listFolders(c Connection, path string) ([]Folder, error) {
	c.Path = path
	if err := validateConnection(&c); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var result struct {
		List []struct {
			Name, Path string
			IsDir      bool
		}
	}
	if err := s.call(ctx, "operations/list", map[string]any{"fs": "sd_" + c.Remote + ":", "remote": path, "opt": map[string]bool{"dirsOnly": true, "recurse": false}}, &result); err != nil {
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
	if ss == nil || ss.ID != id || ss.Busy || !ss.Authenticated || ss.Complete {
		return nil, fmt.Errorf("sign in before choosing a folder")
	}
	if ss.inPlace && path != ss.connection.Path {
		return nil, fmt.Errorf("keep the destination during recovery")
	}
	return s.listFolders(ss.connection, path)
}

func (s *Service) ConfirmConfig(id, path string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss := s.session
	if ss == nil || ss.ID != id || ss.Busy || !ss.Authenticated {
		return "", fmt.Errorf("setup is not ready")
	}
	if ss.Complete {
		return ss.connection.ID, nil
	}
	if ss.inPlace && path != ss.connection.Path {
		return "", fmt.Errorf("keep the destination during recovery")
	}
	if _, err := s.listFolders(ss.connection, path); err != nil {
		return "", err
	}
	ss.connection.Path = path
	if err := s.commitConfig(ss); err != "" {
		ss.Error = err
		return "", fmt.Errorf("%s", err)
	}
	return ss.connection.ID, nil
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
	if !ok && (strings.Contains(name, "2fa") || strings.Contains(name, "otp") || strings.Contains(name, "verification") || strings.Contains(name, "mfa")) {
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
