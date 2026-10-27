package core

import (
	"context"
	"runtime"
	"time"

	"sdrive/internal/platform"
)

type AboutInfo struct {
	Platform        string `json:"platform"`
	OSVersion       string `json:"osVersion"`
	AppVersion      string `json:"appVersion"`
	ReleaseDate     string `json:"releaseDate"`
	RcloneVersion   string `json:"rcloneVersion"`
	WindowsVersion  string `json:"windowsVersion"`
	WinFspVersion   string `json:"winFspVersion"`
	WebView2Version string `json:"webView2Version"`
	Architecture    string `json:"architecture"`
}

func SetReleaseDate(s *Service, date string) { s.releaseDate = date }

// Desktop callbacks are installed before the service is published to the bridge.
func SetDesktopStatus(s *Service, visible func() bool, changed func(uint64, View), version string) {
	s.desktopVisible, s.snapshotChanged, s.appVersion = visible, changed, version
}

func (s *Service) DesktopVisible() bool {
	s.mu.Lock()
	visible := s.desktopVisible
	s.mu.Unlock()
	return visible == nil || visible()
}

func (s *Service) About() AboutInfo {
	if s.accessAllowed() != nil {
		return AboutInfo{}
	}
	s.mu.Lock()
	rpc, version := s.rpc, s.appVersion
	s.mu.Unlock()
	windowsVersion, winFsp, webView := platform.ComponentVersions()
	info := AboutInfo{Platform: platform.OS, OSVersion: windowsVersion, AppVersion: version, ReleaseDate: s.releaseDate, WindowsVersion: windowsVersion, WinFspVersion: winFsp, WebView2Version: webView, Architecture: runtime.GOARCH}
	if platform.OS != "windows" {
		info.WindowsVersion = ""
	}
	if rpc != nil && rpc.Alive() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var result struct {
			Version string `json:"version"`
		}
		if rpc.Call(ctx, "core/version", nil, &result) == nil {
			info.RcloneVersion = result.Version
		}
	}
	return info
}
