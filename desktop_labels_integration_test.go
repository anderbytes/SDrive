package main

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
	"sdrive/internal/core"
	"sdrive/internal/platform"
)

func TestNetworkDriveDisplaysOnlyConnectionTitleInExplorer(t *testing.T) {
	requireNativeMount(t)
	f := newIntegrationFixture(t, true)
	s, _ := openIntegrationService(t, f.profile, nil)
	c := configureIntegration(t, s, f.url, f.letter)
	isolatedConnectionWorkers(t, s, f.profile)
	c.Name, c.MountMode = "Library (SDrive)", "network"
	if err := s.UpdateConnection(c); err != nil {
		t.Fatal(err)
	}
	core.SetDriveLabels(s, func(connections []core.Connection) error {
		drives := []platform.DriveLabel{}
		for _, c := range connections {
			drives = append(drives, platform.DriveLabel{Letter: c.Letter, Name: c.Name})
		}
		return platform.SyncDriveLabels(f.profile, drives)
	})
	t.Cleanup(func() {
		if s.Snapshot().Connections[0].Mounted {
			if err := s.Unmount(c.ID); err != nil {
				t.Error(err)
			}
		}
		if err := platform.SyncDriveLabels(f.profile, nil); err != nil {
			t.Error(err)
		}
	})
	if err := s.Mount(c.ID); err != nil {
		t.Fatal(err)
	}
	if warning := s.Snapshot().DesktopWarning; warning != "" {
		t.Fatal(warning)
	}
	command := fmt.Sprintf(`$s = New-Object -ComObject Shell.Application; $s.Namespace(17).Items() | Where-Object Path -eq '%s:\' | ForEach-Object { $_.Name }`, f.letter)
	output, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", command).CombinedOutput()
	if err != nil {
		t.Fatalf("read Explorer name: %v: %s", err, output)
	}
	expected := fmt.Sprintf("%s (%s:)", c.Name, f.letter)
	if name := strings.TrimSpace(string(output)); name != expected {
		t.Fatalf("Explorer name = %q, want %q", name, expected)
	}
	if err := s.Unmount(c.ID); err != nil {
		t.Fatal(err)
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Explorer\MountPoints2\##server#`+c.Name, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer key.Close()
	for _, value := range []string{"_LabelFromReg", "SDriveLabelOwner", "SDriveLabelInstall", "SDriveLabel"} {
		if _, _, err := key.GetStringValue(value); err != registry.ErrNotExist {
			t.Fatalf("disconnect retained label metadata %s: %v", value, err)
		}
	}
}
