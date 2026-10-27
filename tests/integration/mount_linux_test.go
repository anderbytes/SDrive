package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"sdrive/internal/core"
	"sdrive/internal/platform"
)

func TestLinuxMountLifecycleAndPendingRecovery(t *testing.T) {
	if os.Getenv("SDRIVE_TEST_MOUNT") != "1" {
		t.Skip("set SDRIVE_TEST_MOUNT=1 for real FUSE mounts")
	}
	if !platform.MountAvailable() {
		t.Fatal("FUSE3 and writable /dev/fuse are required")
	}
	f := newIntegrationFixture(t, false)
	service, rpc := openIntegrationService(t, f.profile, func() {})
	isolatedConnectionWorkers(t, service, f.profile)
	mount := filepath.Join(f.root, "mounted")
	session, err := service.BeginConfig(core.ConfigInput{Connection: core.Connection{Name: "Synthetic Linux WebDAV", Provider: "webdav", MountMode: "local", MountFolder: mount}, Values: map[string]string{"url": f.url}, AllowInsecure: true, Guided: true})
	if err != nil {
		t.Fatal(err)
	}
	waitIntegration(t, "Linux WebDAV authentication", 20*time.Second, func() bool {
		state, err := service.ConfigStatus(session)
		if err != nil || state.Error != "" {
			t.Fatal("authentication failed", err, state.Error)
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
	t.Cleanup(func() { f.blocked.Store(false); _ = service.Unmount(id) })
	if err = service.Mount(id); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(mount, "written.txt")
	remote := filepath.Join(f.destination, "library", "written.txt")
	if err = os.WriteFile(local, []byte("Linux mount write"), 0600); err != nil {
		t.Fatal(err)
	}
	waitIntegration(t, "Linux upload", 25*time.Second, func() bool { b, err := os.ReadFile(remote); return err == nil && string(b) == "Linux mount write" })
	if err = service.Unmount(id); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(mount); !os.IsNotExist(err) {
		t.Fatal("owned empty folder was not removed")
	}
	connection := service.Snapshot().Connections[0].Connection
	connection.ReadOnly = true
	if err = service.UpdateConnection(connection); err != nil {
		t.Fatal(err)
	}
	if err = service.Mount(id); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(local); err != nil || string(b) != "Linux mount write" {
		t.Fatal("mounted read failed", err)
	}
	for _, operation := range []func() error{func() error { return os.WriteFile(local, []byte("deny"), 0600) }, func() error { return os.Remove(local) }, func() error { return os.Rename(local, local+"-moved") }} {
		if err = operation(); err == nil {
			t.Fatal("read-only mount allowed mutation")
		}
	}
	if err = service.Unmount(id); err != nil {
		t.Fatal(err)
	}
	connection = service.Snapshot().Connections[0].Connection
	connection.ReadOnly = false
	if err = service.UpdateConnection(connection); err != nil {
		t.Fatal(err)
	}
	if err = service.Mount(id); err != nil {
		t.Fatal(err)
	}
	f.blocked.Store(true)
	if err = os.WriteFile(filepath.Join(mount, "pending.txt"), []byte("preserve pending"), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-f.upload:
	case <-time.After(20 * time.Second):
		t.Fatal("pending upload did not reach the fixture")
	}
	// An interrupted worker must not erase the durable cache or its recovery requirement.
	core.CloseConnectionEngines(service)
	rpc.Close()
	if err = service.Exit(); err == nil {
		t.Fatal("safe exit accepted an interrupted pending upload")
	}
	recovered, recoveryRPC := openIntegrationService(t, f.profile, func() {})
	isolatedConnectionWorkers(t, recovered, f.profile)
	t.Cleanup(func() { _ = recovered.Unmount(id); recoveryRPC.Close() })
	f.blocked.Store(false)
	if platform.FolderMounted(mount) {
		if err := exec.Command("fusermount3", "-u", mount).Run(); err != nil {
			t.Fatal("could not release the disposable interrupted FUSE mount", err)
		}
	}
	if err = recovered.Mount(id); err != nil {
		t.Fatal("recovery mount failed", err)
	}
	waitIntegration(t, "Linux pending recovery", 30*time.Second, func() bool {
		b, err := os.ReadFile(filepath.Join(f.destination, "library", "pending.txt"))
		return err == nil && string(b) == "preserve pending"
	})
	if err = recovered.Unmount(id); err != nil {
		t.Fatal(err)
	}
	if err = recovered.Exit(); err != nil {
		t.Fatal(err)
	}
}
