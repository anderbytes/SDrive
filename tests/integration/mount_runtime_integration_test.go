//go:build windows

package integration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"golang.org/x/sys/windows"
	"sdrive/internal/core"
	"sdrive/internal/desktop"
)

func TestWindowsHiddenFileAttributes(t *testing.T) {
	requireNativeMount(t)
	for _, mode := range []string{"network", "local", "folder"} {
		t.Run(mode, func(t *testing.T) {
			f := newIntegrationFixture(t, true)
			s, _ := openIntegrationService(t, f.profile, nil)
			c := configureIntegration(t, s, f.url, f.letter)
			isolatedConnectionWorkers(t, s, f.profile)
			for _, name := range []string{".dot-file", "normal.txt"} {
				if err := os.WriteFile(filepath.Join(f.destination, "library", name), []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Mkdir(filepath.Join(f.destination, "library", ".dot-folder"), 0700); err != nil {
				t.Fatal(err)
			}
			c.MountMode = mode
			point := c.Letter + `:\`
			if mode == "folder" {
				c.Letter, c.MountMode, c.MountFolder = "", "local", filepath.Join(f.root, "mounted-folder")
				point = c.MountFolder
			}
			for _, readOnly := range []bool{false, true} {
				for _, shown := range []bool{true, false} {
					c.ShowHiddenFiles, c.ReadOnly = &shown, readOnly
					if err := s.UpdateConnection(c); err != nil {
						t.Fatal(err)
					}
					if err := s.Mount(c.ID); err != nil {
						t.Fatal(err)
					}
					for _, name := range []string{".dot-file", ".dot-folder", "normal.txt"} {
						path, err := windows.UTF16PtrFromString(filepath.Join(point, name))
						if err != nil {
							t.Fatal(err)
						}
						attributes, err := windows.GetFileAttributes(path)
						wantHidden := !shown && name != "normal.txt"
						if err != nil || (attributes&windows.FILE_ATTRIBUTE_HIDDEN != 0) != wantHidden {
							t.Fatalf("%s readOnly=%t shown=%t: attributes=%x error=%v", name, readOnly, shown, attributes, err)
						}
						if data, err := os.ReadDir(point); err != nil || len(data) != 3 {
							t.Fatal("hidden items must remain available to filesystem enumeration", err)
						}
					}
					if err := s.Unmount(c.ID); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}

func TestWindowsMountRuntimeModes(t *testing.T) {
	requireNativeMount(t)
	for _, mode := range []string{"network", "local", "folder"} {
		t.Run(mode, func(t *testing.T) {
			f := newIntegrationFixture(t, true)
			s, _ := openIntegrationService(t, f.profile, nil)
			c := configureIntegration(t, s, f.url, f.letter)
			isolatedConnectionWorkers(t, s, f.profile)
			cache := filepath.Join(f.root, "selected-cache")
			if err := os.Mkdir(cache, 0700); err != nil {
				t.Fatal(err)
			}
			if err := s.ChangeCacheLocation(cache); err != nil {
				t.Fatal(err)
			}
			c.MountMode, c.DirectoryCacheSeconds = mode, 1
			point := c.Letter + `:\`
			if mode == "folder" {
				c.Letter, c.MountMode, c.MountFolder = "", "local", filepath.Join(f.root, "mounted-folder")
				point = c.MountFolder
			}
			if err := s.UpdateConnection(c); err != nil {
				t.Fatal(err)
			}
			settings := s.Snapshot().Settings
			age, delay := 48, 1
			settings.CacheMaxAgeHours, settings.WriteBackSeconds = &age, &delay
			if err := s.SaveSettings(settings); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(f.destination, "library", "remote.txt"), []byte("remote data"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := s.Mount(c.ID); err != nil {
				t.Fatal(err)
			}
			view := s.Snapshot().Connections[0]
			if !view.Mounted || !view.Recovery {
				t.Fatal("mount reconciliation failed")
			}
			volumePath, err := windows.UTF16PtrFromString(point + `\`)
			if err != nil {
				t.Fatal(err)
			}
			expectedType := uint32(windows.DRIVE_FIXED)
			if mode == "network" {
				expectedType = windows.DRIVE_REMOTE
			}
			if windows.GetDriveType(volumePath) != expectedType {
				t.Fatal("Windows drive presentation differs from the selected mode")
			}
			if mode != "network" {
				sd, err := windows.GetNamedSecurityInfo(point, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
				if err != nil {
					t.Fatal(err)
				}
				if regexp.MustCompile(`\(A;[^;]*;[^;]*[1-9a-fA-F][^;]*;[^;]*;[^;]*;WD\)`).MatchString(sd.String()) {
					t.Fatal("local mount grants Everyone access")
				}
			}
			beforeGet := f.downloads.Load()
			if _, err := os.ReadDir(point); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(point, "remote.txt")); err != nil {
				t.Fatal(err)
			}
			if f.downloads.Load() != beforeGet {
				t.Fatal("listing or file properties downloaded content")
			}
			if b, err := os.ReadFile(filepath.Join(point, "remote.txt")); err != nil || string(b) != "remote data" {
				t.Fatal("remote file read failed")
			}
			if f.downloads.Load() <= beforeGet {
				t.Fatal("fixture did not observe the content read")
			}
			f.blocked.Store(true)
			if err := os.WriteFile(filepath.Join(point, "pending.txt"), []byte("pending data"), 0600); err != nil {
				t.Fatal(err)
			}
			waitIntegration(t, "pending metadata on selected cache disk", 10*time.Second, func() bool {
				v := s.Snapshot().Connections[0]
				return v.Pending != nil && *v.Pending > 0
			})
			if err := os.WriteFile(filepath.Join(f.destination, "library", "external.txt"), []byte("external data"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := s.RefreshDirectoryCache(c.ID); err != nil {
				t.Fatal(err)
			}
			if b, err := os.ReadFile(filepath.Join(point, "external.txt")); err != nil || string(b) != "external data" {
				t.Fatal("manual external refresh failed")
			}
			if b, err := os.ReadFile(filepath.Join(point, "pending.txt")); err != nil || string(b) != "pending data" {
				t.Fatal("refresh lost local pending content")
			}
			if err := os.Rename(filepath.Join(f.destination, "library", "external.txt"), filepath.Join(f.destination, "library", "renamed.txt")); err != nil {
				t.Fatal(err)
			}
			waitIntegration(t, "directory lifetime refresh", 10*time.Second, func() bool { _, err := os.Stat(filepath.Join(point, "renamed.txt")); return err == nil })
			assertIntegrationAbsent(t, filepath.Join(point, "external.txt"))
			if err := os.Remove(filepath.Join(f.destination, "library", "renamed.txt")); err != nil {
				t.Fatal(err)
			}
			if err := s.RefreshDirectoryCache(c.ID); err != nil {
				t.Fatal(err)
			}
			assertIntegrationAbsent(t, filepath.Join(point, "renamed.txt"))
			if s.ChangeCacheLocation(t.TempDir()) == nil {
				t.Fatal("active cache transition accepted")
			}
			hold, err := os.Create(filepath.Join(point, "open-file.txt"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = hold.Close() })
			if _, err := hold.WriteString("open file data"); err != nil {
				_ = hold.Close()
				t.Fatal(err)
			}
			if err := hold.Sync(); err != nil {
				_ = hold.Close()
				t.Fatal(err)
			}
			unmounted := make(chan error, 1)
			go func() { unmounted <- s.Unmount(c.ID) }()
			waitIntegration(t, "disconnect waiting for the open file", 10*time.Second, func() bool {
				v := s.Snapshot().Connections[0]
				return v.Mounted && v.Recovery && v.Operation == "waitingUploads"
			})
			if err := hold.Close(); err != nil {
				t.Fatal(err)
			}
			f.blocked.Store(false)
			waitIntegration(t, "pending upload completed", 30*time.Second, func() bool {
				b, err := os.ReadFile(filepath.Join(f.destination, "library", "pending.txt"))
				return err == nil && string(b) == "pending data"
			})
			select {
			case err := <-unmounted:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(35 * time.Second):
				t.Fatal("disconnect did not finish after releasing the file")
			}
			if mode == "folder" {
				if _, err := os.Lstat(point); !os.IsNotExist(err) {
					t.Fatal("folder mountpoint was not released")
				}
			}
			core.CloseConnectionEngines(s)
			reopened, _ := openIntegrationService(t, f.profile, nil)
			isolatedConnectionWorkers(t, reopened, f.profile)
			if core.CachePath(reopened) != cache || reopened.Snapshot().Connections[0].MountFolder != c.MountFolder {
				t.Fatal("reopening lost the cache or mount destination")
			}
			if mode == "folder" {
				if err := os.Mkdir(point, 0700); err != nil {
					t.Fatal(err)
				}
				collision := filepath.Join(point, "keep.txt")
				if err := os.WriteFile(collision, []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
				if reopened.Mount(c.ID) == nil {
					t.Fatal("native folder collision accepted")
				}
				if b, err := os.ReadFile(collision); err != nil || string(b) != "preserve" {
					t.Fatal("folder collision damaged content")
				}
				if err := os.Remove(collision); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(point); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Mkdir(filepath.Join(f.destination, "library", "empty-folder"), 0700); err != nil {
				t.Fatal(err)
			}
			c.ReadOnly = true
			if err := reopened.UpdateConnection(c); err != nil {
				t.Fatal(err)
			}
			if err := reopened.Mount(c.ID); err != nil {
				t.Fatal(err)
			}
			uploads, moves, deletes := f.uploads.Load(), f.moves.Load(), f.deletes.Load()
			file := filepath.Join(point, "remote.txt")
			for _, operation := range []struct {
				name string
				run  func() error
			}{
				{"delete file", func() error { return os.Remove(file) }},
				{"delete directory", func() error { return os.Remove(filepath.Join(point, "empty-folder")) }},
				{"rename", func() error { return os.Rename(file, filepath.Join(point, "forbidden-rename.txt")) }},
				{"overwrite", func() error { return os.WriteFile(file, []byte("must not upload"), 0600) }},
				{"create file", func() error {
					return os.WriteFile(filepath.Join(point, "forbidden.txt"), []byte("must not upload"), 0600)
				}},
				{"create directory", func() error { return os.Mkdir(filepath.Join(point, "forbidden-folder"), 0700) }},
			} {
				if err := operation.run(); !errors.Is(err, os.ErrPermission) {
					t.Fatalf("read-only %s must return permission denied immediately, got %v", operation.name, err)
				}
			}
			for _, refresh := range []bool{false, true} {
				if refresh {
					if err := reopened.RefreshDirectoryCache(c.ID); err != nil {
						t.Fatal(err)
					}
				}
				if b, err := os.ReadFile(file); err != nil || string(b) != "remote data" {
					t.Fatalf("read-only operations changed mounted content (refresh=%t): %v", refresh, err)
				}
				if entries, err := os.ReadDir(filepath.Join(point, "empty-folder")); err != nil || len(entries) != 0 {
					t.Fatalf("read-only directory was removed or is inaccessible (refresh=%t): %v", refresh, err)
				}
				for _, name := range []string{"forbidden.txt", "forbidden-rename.txt", "forbidden-folder"} {
					if _, err := os.Stat(filepath.Join(point, name)); !os.IsNotExist(err) {
						t.Fatalf("read-only operation left %s: %v", name, err)
					}
				}
			}
			assertIntegrationAbsent(t, filepath.Join(f.destination, "library", "forbidden.txt"))
			if err := reopened.Unmount(c.ID); err != nil {
				t.Fatal(err)
			}
			if b, err := os.ReadFile(filepath.Join(f.destination, "library", "remote.txt")); err != nil || string(b) != "remote data" {
				t.Fatal("read-only operations changed remote content")
			}
			if entries, err := os.ReadDir(filepath.Join(f.destination, "library", "empty-folder")); err != nil || len(entries) != 0 {
				t.Fatal("read-only operations changed remote directory")
			}
			if f.uploads.Load() != uploads || f.moves.Load() != moves || f.deletes.Load() != deletes {
				t.Fatal("read-only operations reached the remote as mutations")
			}
			c.ReadOnly = false
			if err := reopened.UpdateConnection(c); err != nil {
				t.Fatal(err)
			}
			if err := reopened.Mount(c.ID); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, []byte("updated data"), 0600); err != nil {
				t.Fatal(err)
			}
			waitIntegration(t, "writable replacement uploaded", 15*time.Second, func() bool {
				b, err := os.ReadFile(filepath.Join(f.destination, "library", "remote.txt"))
				return err == nil && string(b) == "updated data"
			})
			renamed := filepath.Join(point, "writable-renamed.txt")
			if err := os.Rename(file, renamed); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(renamed); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(point, "empty-folder")); err != nil {
				t.Fatal(err)
			}
			if err := reopened.Unmount(c.ID); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"remote.txt", "writable-renamed.txt", "empty-folder"} {
				assertIntegrationAbsent(t, filepath.Join(f.destination, "library", name))
			}
			t.Logf("synthetic browsing observations: %d HTTP requests, %d content reads", f.requests.Load(), f.downloads.Load())
		})
	}
}

func TestConnectionWorkerUsesSelectedCache(t *testing.T) {
	if os.Getenv("SDRIVE_TEST_ENGINE") != "1" {
		t.Skip("opt-in: SDRIVE_TEST_ENGINE=1")
	}
	f := newIntegrationFixture(t, false)
	s, _ := openIntegrationService(t, f.profile, nil)
	c := configureIntegration(t, s, f.url, "")
	var worker core.RPC
	core.SetConnectionEngine(s, func(c core.Connection, proxy string) (core.RPC, error) {
		var err error
		worker, err = desktop.StartConnectionEngine(f.profile, core.CachePath(s), c, proxy, nil)
		return worker, err
	})
	t.Cleanup(func() { core.CloseConnectionEngines(s) })
	cache := filepath.Join(f.root, "selected-cache")
	if err := os.Mkdir(cache, 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.ChangeCacheLocation(cache); err != nil {
		t.Fatal(err)
	}
	if err := s.TestConnection(c.ID); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var paths map[string]any
	if err := worker.Call(ctx, "config/paths", map[string]any{}, &paths); err != nil {
		t.Fatal(err)
	}
	if paths["cache"] != cache {
		t.Fatal("worker cache root differs")
	}
}
