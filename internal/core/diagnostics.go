package core

import (
	"fmt"
	"strings"

	"sdrive/internal/diagnostics"
)

// Installed before publishing the service; no profile data enters the collector.
func SetDiagnostics(s *Service, log *diagnostics.Collector) {
	s.diagnostics = log
	result := "ok"
	kind := "none"
	if s.settingsError != "" || s.recoveryHold {
		result = "failed"
		kind = "unknown"
	}
	log.Record("profile", result, kind, nil)
}

func (s *Service) Diagnostics() string {
	info := s.About()
	s.mu.Lock()
	log := s.diagnostics
	engineAlive := s.rpc != nil && s.rpc.Alive()
	connections, mounted, recovery := len(s.store.Connections), 0, 0
	for _, c := range s.store.Connections {
		if s.mounted[c.ID] {
			mounted++
		}
		if c.Recovery {
			recovery++
		}
	}
	settingsUnavailable, hold := s.settingsError != "", s.recoveryHold
	s.mu.Unlock()
	var b strings.Builder
	b.WriteString("SDrive diagnostic report v1\nNo credentials, account names, destinations or file contents are included.\n")
	for _, item := range []struct{ name, version string }{
		{"SDrive", info.AppVersion}, {"rclone (running)", info.RcloneVersion},
		{"Windows", info.WindowsVersion}, {"WinFsp (installed)", info.WinFspVersion}, {"WebView2 (installed)", info.WebView2Version},
	} {
		fmt.Fprintf(&b, "%s: %s\n", item.name, diagnostics.Version(item.version))
	}
	arch := "unavailable"
	if info.Architecture == "amd64" || info.Architecture == "arm64" || info.Architecture == "386" {
		arch = info.Architecture
	}
	fmt.Fprintf(&b, "Architecture: %s\nEngine running: %t; settings unavailable: %t; recovery protection: %t.\nConnections: %d; mounted: %d; recovery required: %d.\n", arch, engineAlive, settingsUnavailable, hold, connections, mounted, recovery)
	b.WriteString(log.Report())
	return b.String()
}
