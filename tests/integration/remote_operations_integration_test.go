//go:build windows

package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func shellFileOperation(t *testing.T, operation, source, destination string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	// Paths are process-scoped data, never interpolated as PowerShell source.
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-STA", "-Command", `$ErrorActionPreference='Stop'; $shell=New-Object -ComObject Shell.Application; $folder=$shell.NameSpace($env:SDRIVE_SHELL_DESTINATION); if($null -eq $folder){throw 'Disposable shell destination unavailable'}; if($env:SDRIVE_SHELL_OPERATION -eq 'copy'){$folder.CopyHere($env:SDRIVE_SHELL_SOURCE,1556)}elseif($env:SDRIVE_SHELL_OPERATION -eq 'copyRename'){$folder.CopyHere($env:SDRIVE_SHELL_SOURCE,1564)}else{$folder.MoveHere($env:SDRIVE_SHELL_SOURCE,1556)}`)
	cmd.Env = append(os.Environ(), "SDRIVE_SHELL_SOURCE="+source, "SDRIVE_SHELL_DESTINATION="+destination, "SDRIVE_SHELL_OPERATION="+operation)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Run(); err != nil {
		t.Fatalf("disposable Windows Shell operation failed: %v", err)
	}
}

func TestMountedRemoteOperationsMeasureExistingWindowsShellBehavior(t *testing.T) {
	requireNativeMount(t)
	f := newIntegrationFixture(t, true)
	remote := filepath.Join(f.destination, "library")
	if err := os.Mkdir(filepath.Join(remote, "target"), 0700); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 2<<20)
	for i := range data {
		data[i] = byte(i % 251)
	}
	for _, name := range []string{"rename.dat", "copy.dat", "failure.dat", "conflict.dat", "pending-copy.dat", "cross.dat"} {
		if err := os.WriteFile(filepath.Join(remote, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	expected, err := integrationHash(filepath.Join(remote, "copy.dat"))
	if err != nil {
		t.Fatal(err)
	}
	s, _ := openIntegrationService(t, f.profile, nil)
	c := configureIntegration(t, s, f.url, f.letter)
	isolatedConnectionWorkers(t, s, f.profile)
	if err := s.Mount(c.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.blocked.Store(false); f.failMoves.Store(false); _ = s.Unmount(c.ID) })
	root := c.Letter + `:\`
	beforeGET, beforePUT, beforeMOVE := f.downloads.Load(), f.uploads.Load(), f.moves.Load()
	if err := os.Rename(filepath.Join(root, "rename.dat"), filepath.Join(root, "renamed.dat")); err != nil {
		t.Fatal(err)
	}
	assertIntegrationFile(t, filepath.Join(remote, "renamed.dat"), expected)
	if f.downloads.Load() != beforeGET || f.uploads.Load() != beforePUT || f.moves.Load() <= beforeMOVE {
		t.Fatal("same-backend rename did not use remote MOVE without content transfer")
	}
	beforeGET, beforePUT, beforeMOVE = f.downloads.Load(), f.uploads.Load(), f.moves.Load()
	shellFileOperation(t, "move", filepath.Join(root, "renamed.dat"), filepath.Join(root, "target"))
	assertIntegrationFile(t, filepath.Join(remote, "target", "renamed.dat"), expected)
	if _, err := os.Stat(filepath.Join(remote, "renamed.dat")); !os.IsNotExist(err) {
		t.Fatal("successful shell move left source")
	}
	if f.downloads.Load() != beforeGET || f.uploads.Load() != beforePUT || f.moves.Load() <= beforeMOVE {
		t.Fatal("Windows Shell same-drive move did not use remote MOVE")
	}
	beforeGET, beforePUT, beforeCOPY := f.downloads.Load(), f.uploads.Load(), f.copies.Load()
	shellFileOperation(t, "copy", filepath.Join(root, "copy.dat"), filepath.Join(root, "target"))
	assertIntegrationFile(t, filepath.Join(remote, "target", "copy.dat"), expected)
	assertIntegrationFile(t, filepath.Join(remote, "copy.dat"), expected)
	if f.downloads.Load() <= beforeGET || f.uploads.Load() <= beforePUT || f.copies.Load() != beforeCOPY {
		t.Fatal("Windows Shell copy baseline changed; reassess server-side copy support")
	}
	t.Log("Windows Shell copy downloaded/uploaded content; same-drive rename/move used remote MOVE without content transfer")
	if err := os.WriteFile(filepath.Join(root, "target", "conflict.dat"), []byte("preserve original conflict target"), 0600); err != nil {
		t.Fatal(err)
	}
	oldHash, err := integrationHash(filepath.Join(root, "target", "conflict.dat"))
	if err != nil {
		t.Fatal(err)
	}
	assertIntegrationFile(t, filepath.Join(remote, "target", "conflict.dat"), oldHash)
	beforeEntries, err := os.ReadDir(filepath.Join(root, "target"))
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, entry := range beforeEntries {
		known[entry.Name()] = true
	}
	shellFileOperation(t, "copyRename", filepath.Join(root, "conflict.dat"), filepath.Join(root, "target"))
	var collisionName string
	waitIntegration(t, "Windows Shell collision rename", 20*time.Second, func() bool {
		entries, err := os.ReadDir(filepath.Join(root, "target"))
		if err != nil {
			return false
		}
		for _, entry := range entries {
			if !known[entry.Name()] {
				collisionName = entry.Name()
				return true
			}
		}
		return false
	})
	assertIntegrationFile(t, filepath.Join(remote, "target", collisionName), expected)
	assertIntegrationFile(t, filepath.Join(remote, "target", "conflict.dat"), oldHash)
	assertIntegrationFile(t, filepath.Join(remote, "conflict.dat"), expected)
	f.blocked.Store(true)
	shellFileOperation(t, "copy", filepath.Join(root, "pending-copy.dat"), filepath.Join(root, "target"))
	waitIntegration(t, "rejected copy upload retained in cache", 15*time.Second, func() bool {
		select {
		case <-f.upload:
			return true
		default:
			return false
		}
	})
	assertIntegrationFile(t, filepath.Join(remote, "pending-copy.dat"), expected)
	assertIntegrationFile(t, filepath.Join(root, "target", "pending-copy.dat"), expected)
	view := s.Snapshot().Connections[0]
	if view.Pending == nil || *view.Pending == 0 || !view.Recovery {
		t.Fatal("failed upload lost pending recovery protection")
	}
	f.blocked.Store(false)
	assertIntegrationFile(t, filepath.Join(remote, "target", "pending-copy.dat"), expected)
	other := newIntegrationFixture(t, true)
	peer, _ := openIntegrationService(t, other.profile, nil)
	peerConnection := configureIntegration(t, peer, other.url, other.letter)
	peerConnection.Name = "Remote copy target"
	if err := peer.UpdateConnection(peerConnection); err != nil {
		t.Fatal(err)
	}
	isolatedConnectionWorkers(t, peer, other.profile)
	if err := peer.Mount(peerConnection.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.Unmount(peerConnection.ID) })
	shellFileOperation(t, "copy", filepath.Join(root, "cross.dat"), peerConnection.Letter+`:\`)
	assertIntegrationFile(t, filepath.Join(other.destination, "library", "cross.dat"), expected)
	assertIntegrationFile(t, filepath.Join(remote, "cross.dat"), expected)
	if other.uploads.Load() == 0 || other.copies.Load() != 0 {
		t.Fatal("cross-backend copy baseline changed")
	}
	t.Log("Collision rename preserved the existing target; rejected uploads retained the source/cache and recovered; cross-backend copy preserved source/target content")
	// An explicit remote failure must preserve the sole valid source.
	f.failMoves.Store(true)
	if err := os.Rename(filepath.Join(root, "failure.dat"), filepath.Join(root, "failed.dat")); err == nil {
		t.Fatal("injected remote MOVE failure succeeded")
	}
	f.failMoves.Store(false)
	assertIntegrationFile(t, filepath.Join(remote, "failure.dat"), expected)
	if _, err := os.Stat(filepath.Join(remote, "failed.dat")); !os.IsNotExist(err) {
		t.Fatal("failed move produced unexpected destination")
	}
	beforeDELETE, beforeGET := f.deletes.Load(), f.downloads.Load()
	if err := os.Remove(filepath.Join(root, "target", "copy.dat")); err != nil {
		t.Fatal(err)
	}
	if f.deletes.Load() <= beforeDELETE || f.downloads.Load() != beforeGET {
		t.Fatal("delete did not use remote DELETE without download")
	}
}
