package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"sdrive/internal/core"
	"sdrive/internal/platform"
)

func TestServiceWebDAVIntegration(t *testing.T) {
	f := newIntegrationFixture(t, false)
	f.service, f.rpc = openIntegrationService(t, f.profile, nil)
	c := configureIntegration(t, f.service, f.url, "")
	if err := f.service.TestConnection(c.ID); err != nil {
		t.Fatal(err)
	}
	// Check the writable fixture and upload gate even on machines without WinFsp.
	for _, blocked := range []bool{true, false} {
		f.blocked.Store(blocked)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		request, err := http.NewRequestWithContext(ctx, http.MethodPut, f.url+"/library/probe.txt", bytes.NewBufferString("synthetic probe"))
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(request)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if blocked {
			if response.StatusCode != http.StatusServiceUnavailable {
				t.Fatal("upload gate did not block the write")
			}
			select {
			case <-f.upload:
			default:
				t.Fatal("upload gate did not signal the blocked write")
			}
			assertIntegrationAbsent(t, filepath.Join(f.destination, "library", "probe.txt"))
		} else if response.StatusCode < 200 || response.StatusCode >= 300 {
			t.Fatal("local WebDAV rejected the disposable write")
		}
	}
	expected := fmt.Sprintf("%x", sha256.Sum256([]byte("synthetic probe")))
	assertIntegrationFile(t, filepath.Join(f.destination, "library", "probe.txt"), expected)
	integrationCall(t, f.rpc, "operations/copyfile", map[string]string{"srcFs": "sd_" + c.Remote + ":" + c.Path, "srcRemote": "probe.txt", "dstFs": f.root, "dstRemote": "download.txt"}, nil)
	assertIntegrationFile(t, filepath.Join(f.root, "download.txt"), expected)
	f.rpc.Close()
	reopened, _ := openIntegrationService(t, f.profile, nil)
	v := reopened.Snapshot()
	if len(v.Connections) != 1 || v.Connections[0].Connection != c || v.Connections[0].Mounted {
		t.Fatal("reopening changed the saved disposable connection")
	}
	if err := reopened.TestConnection(c.ID); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsMountLifecycle(t *testing.T) {
	f := newIntegrationFixture(t, true)
	f.service, f.rpc = openIntegrationService(t, f.profile, nil)
	c := configureIntegration(t, f.service, f.url, f.letter)
	if err := f.service.Mount(c.ID); err != nil {
		t.Fatal(err)
	}
	v := f.service.Snapshot().Connections[0]
	if !v.Mounted || !v.Recovery || v.TargetHash == "" {
		t.Fatal("mount did not persist recovery protection")
	}
	mount := c.Letter + `:\`
	largeHash := ""
	for _, file := range []struct {
		name string
		size int64
	}{{"empty.txt", 0}, {"Café 日本 😀.bin", 64 << 20}} {
		path := filepath.Join(mount, file.name)
		out, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		// A repeating synthetic block bounds memory use while checking large files.
		block := bytes.Repeat([]byte("test-content-"), 8192)
		hash := sha256.New()
		for remaining := file.size; remaining > 0; {
			n := min(remaining, int64(len(block)))
			written, err := io.MultiWriter(out, hash).Write(block[:n])
			if err != nil {
				_ = out.Close()
				t.Fatal(err)
			}
			remaining -= int64(written)
		}
		if err = out.Close(); err != nil {
			t.Fatal(err)
		}
		expected := fmt.Sprintf("%x", hash.Sum(nil))
		assertIntegrationFile(t, path, expected)
		assertIntegrationFile(t, filepath.Join(f.destination, "library", file.name), expected)
		if file.size == 0 {
			assertPinnedCacheSchema(t, f.rpc, c, file.name)
		}
		if file.size > 0 {
			largeHash = expected
		}
	}
	name, renamed := "Café 日本 😀.bin", "renamed.bin"
	if err := os.Rename(filepath.Join(mount, name), filepath.Join(mount, renamed)); err != nil {
		t.Fatal(err)
	}
	assertIntegrationAbsent(t, filepath.Join(f.destination, "library", name))
	assertIntegrationFile(t, filepath.Join(f.destination, "library", renamed), largeHash)
	replacement := []byte("replacement, shorter than the original")
	if err := os.WriteFile(filepath.Join(mount, renamed), replacement, 0600); err != nil {
		t.Fatal(err)
	}
	assertIntegrationFile(t, filepath.Join(f.destination, "library", renamed), fmt.Sprintf("%x", sha256.Sum256(replacement)))
	if err := os.Remove(filepath.Join(mount, renamed)); err != nil {
		t.Fatal(err)
	}
	assertIntegrationAbsent(t, filepath.Join(f.destination, "library", renamed))
	// Explicit invalidation does not certify automatic remote change detection.
	for _, data := range []string{"external creation", "external replacement"} {
		if err := os.WriteFile(filepath.Join(f.destination, "library", "external.txt"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		integrationCall(t, f.rpc, "vfs/forget", map[string]string{"fs": "sd_" + c.Remote + ":" + c.Path}, nil)
		assertIntegrationFile(t, filepath.Join(mount, "external.txt"), fmt.Sprintf("%x", sha256.Sum256([]byte(data))))
	}
	if err := os.Remove(filepath.Join(f.destination, "library", "external.txt")); err != nil {
		t.Fatal(err)
	}
	integrationCall(t, f.rpc, "vfs/forget", map[string]string{"fs": "sd_" + c.Remote + ":" + c.Path}, nil)
	assertIntegrationAbsent(t, filepath.Join(mount, "external.txt"))
	if err := f.service.Unmount(c.ID); err != nil {
		t.Fatal(err)
	}
	assertIntegrationUnmounted(t, f, c)
	f.rpc.Close()
	f.service, f.rpc = openIntegrationService(t, f.profile, nil)
	clean := f.service.Snapshot().Connections[0]
	if clean.Recovery || clean.Mounted || clean.ID != c.ID || clean.TargetHash != v.TargetHash {
		t.Fatal("clean unmount was not persisted")
	}
	c = clean.Connection
	c.ReadOnly = true
	if err := f.service.UpdateConnection(c); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Mount(c.ID); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mount, "blocked.txt"), []byte("blocked"), 0600); err == nil {
		t.Fatal("read-only mount accepted writes")
	}
	if err := f.service.Unmount(c.ID); err != nil {
		t.Fatal(err)
	}
	assertIntegrationUnmounted(t, f, c)
}

func assertIntegrationAbsent(t *testing.T, path string) {
	t.Helper()
	waitIntegration(t, "deleted or renamed file", 20*time.Second, func() bool { _, err := os.Stat(path); return os.IsNotExist(err) })
}

func assertIntegrationUnmounted(t *testing.T, f *integrationFixture, c core.Connection) {
	t.Helper()
	waitIntegration(t, "released fixture mount", 10*time.Second, func() bool {
		if c.MountFolder != "" {
			_, err := os.Lstat(c.MountFolder)
			return os.IsNotExist(err)
		}
		return !platform.DriveUsed(c.Letter)
	})
	v := f.service.Snapshot().Connections[0]
	if v.Mounted || v.Recovery || v.Pending == nil || *v.Pending != 0 || integrationDirtyAt(t, core.CachePath(f.service)) {
		t.Fatal("safe disconnect retained a mount, recovery protection or pending data")
	}
}

func TestWindowsMountOpenFiles(t *testing.T) {
	f := newIntegrationFixture(t, true)
	var quit atomic.Bool
	f.service, f.rpc = openIntegrationService(t, f.profile, func() { quit.Store(true) })
	c := configureIntegration(t, f.service, f.url, f.letter)
	if err := f.service.Mount(c.ID); err != nil {
		t.Fatal(err)
	}
	f.blocked.Store(true)
	file, err := os.Create(filepath.Join(c.Letter+`:\`, "held.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err = io.WriteString(file, "initial"); err != nil {
		t.Fatal(err)
	}
	if err = file.Sync(); err != nil {
		t.Fatal(err)
	}
	waitIntegration(t, "durable pending open file", 10*time.Second, func() bool { return integrationDirty(t, f.profile) })
	done := make(chan error, 1)
	finished := make(chan struct{})
	go func() { done <- f.service.Unmount(c.ID); close(finished) }()
	t.Cleanup(func() {
		select {
		case <-finished:
		case <-time.After(40 * time.Second):
			t.Error("disconnect worker survived its deadline")
		}
	})
	waitIntegration(t, "disconnect waiting for uploads", 10*time.Second, func() bool {
		return f.service.Snapshot().Connections[0].Operation == "waitingUploads"
	})
	if _, err = io.WriteString(file, " and late write"); err != nil {
		t.Fatal(err)
	}
	if err = file.Sync(); err != nil {
		t.Fatal(err)
	}
	if err = f.service.Exit(); err == nil || quit.Load() {
		t.Fatal("safe exit passed an active disconnect")
	}
	select {
	case err := <-done:
		t.Fatalf("disconnect completed with a dirty open handle: %v", err)
	default:
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	f.blocked.Store(false)
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(45 * time.Second):
		t.Fatal("disconnect did not finish after closing the handle")
	}
	assertIntegrationFile(t, filepath.Join(f.destination, "library", "held.txt"), fmt.Sprintf("%x", sha256.Sum256([]byte("initial and late write"))))
	assertIntegrationUnmounted(t, f, c)
	if err = f.service.Exit(); err != nil || !quit.Load() {
		t.Fatal("clean exit failed after closing the file")
	}
}

func TestWindowsMountPendingRecovery(t *testing.T) {
	requireNativeMount(t)
	for _, mode := range []string{"network", "local", "folder"} {
		t.Run(mode, func(t *testing.T) { verifyInterruptedMountRecovery(t, mode) })
	}
}

func verifyInterruptedMountRecovery(t *testing.T, mode string) {
	t.Helper()
	f := newIntegrationFixture(t, true)
	f.blocked.Store(true)
	cache := filepath.Join(f.root, "selected-cache")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMountRecoveryChild$", "-test.timeout=80s")
	child.Env = append(os.Environ(), "SDRIVE_TEST_RECOVERY_ROOT="+f.root, "SDRIVE_TEST_RECOVERY_URL="+f.url, "SDRIVE_TEST_RECOVERY_LETTER="+f.letter, "SDRIVE_TEST_RECOVERY_MODE="+mode)
	// Raw child output is discarded; retained evidence is synthetic state and content.
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() { _ = child.Wait(); close(exited) }()
	t.Cleanup(func() {
		_ = child.Process.Kill()
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			t.Error("isolated child did not terminate")
		}
	})
	var saved core.Connection
	waitIntegration(t, "child pending-write checkpoint", 45*time.Second, func() bool {
		select {
		case <-exited:
			t.Fatal("isolated child exited before checkpoint")
		default:
		}
		data, err := os.ReadFile(filepath.Join(f.root, "checkpoint.json"))
		return err == nil && json.Unmarshal(data, &saved) == nil
	})
	select {
	case <-f.upload:
	case <-time.After(15 * time.Second):
		t.Fatal("pending upload did not reach the controlled destination")
	}
	if !integrationDirtyAt(t, cache) {
		t.Fatal("interruption checkpoint has no durable pending data")
	}
	expected := fmt.Sprintf("%x", sha256.Sum256([]byte("durable interrupted content")))
	backingFile := filepath.Join(cache, "vfs", "sd_"+saved.Remote, saved.Path, "recover.txt")
	assertIntegrationFile(t, backingFile, expected)
	assertIntegrationAbsent(t, filepath.Join(f.destination, "library", "recover.txt"))
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("interrupted child did not exit")
	}
	waitIntegration(t, "interrupted mount release", 10*time.Second, func() bool {
		if saved.MountFolder != "" {
			_, err := os.Lstat(saved.MountFolder)
			return os.IsNotExist(err)
		}
		return !platform.DriveUsed(f.letter)
	})
	if !integrationDirtyAt(t, cache) {
		t.Fatal("process interruption lost pending metadata")
	}
	assertIntegrationFile(t, backingFile, expected)
	f.service, f.rpc = openIntegrationService(t, f.profile, nil)
	isolatedConnectionWorkers(t, f.service, f.profile)
	v := f.service.Snapshot().Connections[0]
	if v.Connection != saved || !v.Recovery || v.TargetHash == "" || v.Mounted {
		t.Fatal("reopen lost the original recovery destination")
	}
	if err := f.service.Exit(); err == nil {
		t.Fatal("exit accepted interrupted pending data")
	}
	f.blocked.Store(false)
	if err := f.service.Mount(saved.ID); err != nil {
		t.Fatal(err)
	}
	assertIntegrationFile(t, filepath.Join(f.destination, "library", "recover.txt"), expected)
	if err := f.service.Unmount(saved.ID); err != nil {
		t.Fatal(err)
	}
	assertIntegrationUnmounted(t, f, saved)
}

// Only the parent selects this helper, which owns its engine's kill-on-close job.
func TestMountRecoveryChild(t *testing.T) {
	root := os.Getenv("SDRIVE_TEST_RECOVERY_ROOT")
	if root == "" || os.Getenv("SDRIVE_TEST_MOUNT") != "1" {
		t.Skip("isolated interruption helper")
	}
	base, err := filepath.Abs(filepath.Join(".tools", "integration"))
	if err != nil || filepath.Dir(root) != base || !platform.WinFspInstalled() {
		t.Fatal("invalid isolated mount helper environment")
	}
	s, _ := openIntegrationService(t, filepath.Join(root, "client"), nil)
	mode := os.Getenv("SDRIVE_TEST_RECOVERY_MODE")
	if mode != "network" && mode != "local" && mode != "folder" {
		t.Fatal("invalid isolated recovery mode")
	}
	cache := filepath.Join(root, "selected-cache")
	if err := os.Mkdir(cache, 0700); err != nil {
		t.Fatal(err)
	}
	isolatedConnectionWorkers(t, s, filepath.Join(root, "client"))
	if err := s.ChangeCacheLocation(cache); err != nil {
		t.Fatal(err)
	}
	c := configureIntegration(t, s, os.Getenv("SDRIVE_TEST_RECOVERY_URL"), os.Getenv("SDRIVE_TEST_RECOVERY_LETTER"))
	c.MountMode = mode
	point := c.Letter + `:\`
	if mode == "folder" {
		c.Letter, c.MountMode, c.MountFolder = "", "local", filepath.Join(root, "mounted-folder")
		point = c.MountFolder
	}
	if err := s.UpdateConnection(c); err != nil {
		t.Fatal(err)
	}
	if err := s.Mount(c.ID); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(point, "recover.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.WriteString(file, "durable interrupted content"); err != nil {
		t.Fatal(err)
	}
	if err = file.Sync(); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	waitIntegration(t, "durable interrupted file", 10*time.Second, func() bool { return integrationDirtyAt(t, cache) })
	checkpoint, err := json.Marshal(s.Snapshot().Connections[0].Connection)
	if err != nil {
		t.Fatal(err)
	}
	if err = platform.AtomicWrite(filepath.Join(root, "checkpoint.json"), checkpoint); err != nil {
		t.Fatal(err)
	}
	<-time.After(70 * time.Second)
	t.Fatal("parent did not interrupt isolated helper")
}
