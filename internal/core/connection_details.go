package core

import (
	"fmt"
	"net/url"
	"strings"
)

type ConnectionDetail struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Use the existing guarded reader, but never expose credential files or URL secrets.
func (s *Service) ConnectionDetails(id string) ([]ConnectionDetail, error) {
	s.mu.Lock()
	i := s.index(id)
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	if i < 0 {
		s.mu.Unlock()
		return nil, fmt.Errorf("connection not found")
	}
	c, epoch := s.store.Connections[i], s.access.epoch
	s.mu.Unlock()
	fields, err := s.ConnectionFields(id)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	i = s.index(id)
	if i < 0 || s.store.Connections[i] != c || s.access.epoch != epoch {
		return nil, fmt.Errorf("connection not found")
	}
	p, _ := provider(c.Provider)
	details := []ConnectionDetail{}
	for _, field := range p.Fields {
		value := fields.Values[field.Key]
		if value == "" {
			value = field.Default
		}
		if field.Secret || field.File || value == "" {
			continue
		}
		if strings.Contains(value, "://") || field.Key == "url" || field.Key == "endpoint" {
			u, err := url.Parse(value)
			if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
				continue
			}
			u.User, u.RawQuery, u.Fragment, u.RawFragment = nil, "", "", ""
			u.ForceQuery = false
			value = u.String()
		}
		details = append(details, ConnectionDetail{Label: field.Label, Value: value})
	}
	if fields.Transport != "" {
		details = append(details, ConnectionDetail{Label: "Access type", Value: fields.Transport})
	}
	if c.AlternateURL != "" {
		u, err := url.Parse(c.AlternateURL)
		if err == nil && u.Host != "" && u.Scheme == "https" {
			u.User, u.RawQuery, u.Fragment, u.RawFragment = nil, "", "", ""
			u.ForceQuery = false
			details = append(details, ConnectionDetail{Label: "Alternate HTTPS WebDAV address", Value: u.String()})
		}
	}
	return details, nil
}
