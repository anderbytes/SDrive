package core

import (
	"fmt"
	"sdrive/internal/platform"
	"strings"

	"sdrive/internal/diagnostics"
)

func SetDiagnosticReportPicker(s *Service, pick func() (string, error)) {
	s.pickDiagnosticReport = pick
}

// Export exactly the report the user reviewed; no new collection is performed here.
func (s *Service) ExportDiagnosticReport(report string) (bool, error) {
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return false, err
	}
	pick, epoch := s.pickDiagnosticReport, s.access.epoch
	s.mu.Unlock()
	if len(report) > 1<<20 || !strings.HasPrefix(report, "SDrive diagnostic report v1\n") {
		return false, fmt.Errorf("Invalid diagnostic report.")
	}
	if pick == nil {
		return false, fmt.Errorf("Diagnostic export is unavailable.")
	}
	path, err := pick()
	if err != nil {
		return false, fmt.Errorf("Could not export the diagnostic report. Try again.")
	}
	if path == "" {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return false, err
	}
	if s.access.epoch != epoch {
		return false, fmt.Errorf("Unlock SDrive before viewing diagnostics.")
	}
	if err := platform.AtomicWrite(path, []byte(report)); err != nil {
		return false, fmt.Errorf("Could not export the diagnostic report. Try again.")
	}
	return true, nil
}

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
	if s.accessAllowed() != nil {
		return "Unlock SDrive before viewing diagnostics."
	}
	info := s.About()
	s.mu.Lock()
	if s.access.locked {
		s.mu.Unlock()
		return "Unlock SDrive before viewing diagnostics."
	}
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
		{"Operating system", info.OSVersion}, {"WinFsp (installed)", info.WinFspVersion}, {"WebView2 (installed)", info.WebView2Version},
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
