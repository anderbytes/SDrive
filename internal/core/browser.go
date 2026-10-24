package core

import (
	"context"
	"fmt"
	"net/url"
	"sdrive/internal/platform"
	"time"
)

func (s *Service) Browsers() []platform.Browser { return platform.InstalledBrowsers() }

func validAuthorizationURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "http" && u.Host == "127.0.0.1:53682" && u.Path == "/auth" && u.User == nil && u.Fragment == "" && len(u.Query()) == 1 && len(u.Query()["state"]) == 1 && u.Query().Get("state") != "" && len(value) <= 8192
}

func (s *Service) launchAuthorization(ctx context.Context, id string, rpc RPC) {
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		var status struct {
			Status string `json:"status"`
			URL    string `json:"authUrl"`
		}
		poll, done := context.WithTimeout(ctx, time.Second)
		err := callRPC(poll, rpc, "config/oauthstatus", map[string]any{}, &status)
		done()
		if err != nil || status.Status != "running" {
			continue
		}
		s.mu.Lock()
		ss := s.session
		if ss == nil || ss.ID != id || ss.closing || ss.browserOpened {
			s.mu.Unlock()
			return
		}
		ss.browserOpened = true
		c, open := ss.connection, s.open
		s.mu.Unlock()
		if !validAuthorizationURL(status.URL) {
			err = fmt.Errorf("The engine returned an invalid sign-in address.")
		} else if c.Browser != "" {
			err = platform.OpenSignIn(c.Browser, c.PrivateBrowser, status.URL)
		} else if c.PrivateBrowser {
			err = fmt.Errorf("Select a browser to use a private window.")
		} else if open == nil {
			err = fmt.Errorf("Browser launch is unavailable.")
		} else {
			err = open(status.URL)
			if err != nil {
				err = fmt.Errorf("Could not launch the selected browser.")
			}
		}
		if err != nil {
			s.mu.Lock()
			if s.session == ss && !ss.closing {
				ss.browserError = err.Error()
				ss.Error = err.Error()
				ss.cancel()
			}
			s.mu.Unlock()
		}
		return
	}
}
