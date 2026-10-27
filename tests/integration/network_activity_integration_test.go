package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sdrive/internal/core"
)

func TestNetworkMountCountersIgnoreCachedReads(t *testing.T) {
	requireNativeMount(t)
	f := newIntegrationFixture(t, false)
	service, _ := openIntegrationService(t, f.profile, func() {})
	isolatedConnectionWorkers(t, service, f.profile)
	payload := []byte(strings.Repeat("network fixture\n", 4096))
	if err := os.WriteFile(filepath.Join(f.destination, "library", "remote.txt"), payload, 0600); err != nil {
		t.Fatal(err)
	}
	mount := filepath.Join(f.root, "network-mount")
	session, err := service.BeginConfig(core.ConfigInput{Connection: core.Connection{Name: "Synthetic network fixture", Provider: "webdav", MountMode: "local", MountFolder: mount}, Values: map[string]string{"url": f.url}, AllowInsecure: true, Guided: true})
	if err != nil {
		t.Fatal(err)
	}
	waitIntegration(t, "network authentication", 20*time.Second, func() bool {
		state, err := service.ConfigStatus(session)
		if err != nil || state.Error != "" {
			t.Fatal(err, state.Error)
		}
		return !state.Busy && state.Authenticated
	})
	id, err := service.ConfirmConfig(session, "library", core.MountTarget{Folder: mount, Mode: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.EndConfig(session); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Unmount(id) })
	if err = service.Mount(id); err != nil {
		t.Fatal(err)
	}
	if _, err = os.ReadDir(mount); err != nil {
		t.Fatal(err)
	}
	listing := service.Snapshot().Connections[0].Network
	if listing.State != "active" || listing.Sent == nil || *listing.Sent <= 0 || *listing.Received <= 0 || *listing.FilesReceived != 0 {
		t.Fatalf("metadata counters: %+v", listing)
	}
	if b, err := os.ReadFile(filepath.Join(mount, "remote.txt")); err != nil || string(b) != string(payload) {
		t.Fatal("mounted content", err)
	}
	read := service.Snapshot().Connections[0].Network
	if read.FilesReceived == nil || *read.FilesReceived != int64(len(payload)) {
		t.Fatalf("download counter: %+v", read)
	}
	downloads := f.downloads.Load()
	if _, err := os.ReadFile(filepath.Join(mount, "remote.txt")); err != nil {
		t.Fatal(err)
	}
	cached := service.Snapshot().Connections[0].Network
	if *cached.FilesReceived != *read.FilesReceived || f.downloads.Load() != downloads {
		t.Fatal("cached read counted as another download")
	}
	if err = os.WriteFile(filepath.Join(mount, "upload.txt"), payload, 0600); err != nil {
		t.Fatal(err)
	}
	waitIntegration(t, "network upload", 25*time.Second, func() bool {
		b, err := os.ReadFile(filepath.Join(f.destination, "library", "upload.txt"))
		return err == nil && string(b) == string(payload)
	})
	if err = service.Unmount(id); err != nil {
		t.Fatal(err)
	}
	stopped := service.Snapshot().Connections[0].Network
	if stopped.State != "disconnected" || *stopped.FilesSent != int64(len(payload)) || *stopped.FilesReceived != int64(len(payload)) {
		t.Fatalf("final counters: %+v", stopped)
	}
	if err = service.Mount(id); err != nil {
		t.Fatal(err)
	}
	reconnected := service.Snapshot().Connections[0].Network
	if reconnected.StartedAt.Equal(*stopped.StartedAt) || *reconnected.FilesSent != 0 || *reconnected.FilesReceived != 0 {
		t.Fatalf("reconnection retained counters: %+v", reconnected)
	}
	if err = service.Unmount(id); err != nil {
		t.Fatal(err)
	}
}
