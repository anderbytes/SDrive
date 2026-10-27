//go:build windows

package platform

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestAllocatedBytesIncludesSparseFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache-file")
	if err := os.WriteFile(path, []byte("cached content"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := AllocatedBytes(path); err != nil {
		t.Fatal(err)
	}
	if _, err := AllocatedBytes(path + "-missing"); err == nil {
		t.Fatal("missing file reported as zero")
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var returned uint32
	if err = windows.DeviceIoControl(windows.Handle(f.Fd()), windows.FSCTL_SET_SPARSE, nil, 0, nil, 0, &returned, nil); err != nil {
		t.Skipf("sparse files unsupported: %v", err)
	}
	if err = f.Truncate(64 << 20); err != nil {
		t.Fatal(err)
	}
	used, err := AllocatedBytes(path)
	if err != nil || used >= 64<<20 {
		t.Fatalf("sparse allocation = %d, %v", used, err)
	}
}

func TestDPAPIKeyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := ProtectDir(dir); err != nil {
		t.Fatal(err)
	}
	first, err := Key(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Key(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first) < 40 {
		t.Fatal("key did not persist")
	}
	disk, err := os.ReadFile(filepath.Join(dir, "key.dpapi"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(disk, []byte(first)) {
		t.Fatal("key in plain text")
	}
}

func TestKeyFailurePreservesExistingData(t *testing.T) {
	for _, scenario := range []string{"empty key", "corrupt key", "missing key with config", "unreadable config", "key destination"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "key.dpapi")
			var original []byte
			switch scenario {
			case "empty key", "corrupt key":
				if scenario == "corrupt key" {
					original = []byte("damaged encrypted key")
				}
				if err := os.WriteFile(path, original, 0600); err != nil {
					t.Fatal(err)
				}
			case "missing key with config":
				if err := os.WriteFile(filepath.Join(dir, "rclone.conf"), []byte("encrypted credentials"), 0600); err != nil {
					t.Fatal(err)
				}
			case "unreadable config":
				if err := os.Mkdir(filepath.Join(dir, "rclone.conf"), 0700); err != nil {
					t.Fatal(err)
				}
			case "key destination":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := Key(dir); err == nil {
				t.Fatal("unsafe key regeneration allowed")
			}
			if scenario == "empty key" || scenario == "corrupt key" {
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(after, original) {
					t.Fatalf("existing key changed: %v", err)
				}
			}
			if scenario == "missing key with config" || scenario == "unreadable config" {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("key regenerated over existing configuration")
				}
			}
		})
	}
}

func TestAtomicWriteFailureAndExclusiveCreation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "key")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, promote := range []func(string, string) error{
		func(src, dst string) error {
			data, err := os.ReadFile(src)
			if err != nil || string(data) != "new" {
				t.Fatal("incomplete temporary file promoted")
			}
			return errors.New("interrupted promotion")
		},
		createFile,
	} {
		if atomicWrite(path, []byte("new"), promote) == nil {
			t.Fatal("existing key overwritten")
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "old" {
			t.Fatalf("previous key lost: %v", err)
		}
		partials, _ := filepath.Glob(filepath.Join(dir, ".sdrive-*"))
		if len(partials) != 0 {
			t.Fatalf("partial files leaked: %v", partials)
		}
	}
}

func TestConcurrentKeyCreationUsesOnePersistentKey(t *testing.T) {
	dir := t.TempDir()
	start := make(chan struct{})
	keys := make([]string, 4)
	errs := make([]error, len(keys))
	var workers sync.WaitGroup
	for i := range keys {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			keys[i], errs[i] = Key(dir)
		}()
	}
	close(start)
	workers.Wait()
	for i, key := range keys {
		if errs[i] != nil || key == "" || key != keys[0] {
			t.Fatalf("concurrent key creation disagreed: %v", errs[i])
		}
	}
	persisted, err := Key(dir)
	if err != nil || persisted != keys[0] {
		t.Fatal("persistent key differs from issued keys")
	}
}

func TestPrerequisiteInstallerRejectsUnverifiedPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-an-installer.msi")
	if err := os.WriteFile(path, []byte("invalid payload"), 0600); err != nil {
		t.Fatal(err)
	}
	if code := InstallPrerequisite(Prerequisite{File: "winfsp.msi"}, path); code != 13 {
		t.Fatalf("unverified installer accepted: %d", code)
	}
	if compatibleWinFsp(path) {
		t.Fatal("non-DLL detected as compatible driver")
	}
	if unsafe.Sizeof(shellExecuteInfo{}) != 112 || unsafe.Offsetof(shellExecuteInfo{}.Process) != 104 {
		t.Fatal("invalid ShellExecuteEx x64 ABI")
	}
	code := PrerequisiteStatus()
	if code != 0 && code != 10 && code != 20 && code != 30 {
		t.Fatalf("invalid installer flags: %d", code)
	}
}
func TestAtomicReplace(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "new"), filepath.Join(dir, "old")
	_ = os.WriteFile(src, []byte("new"), 0600)
	_ = os.WriteFile(dst, []byte("old"), 0600)
	if err := Replace(src, dst); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "new" {
		t.Fatal("replacement failed")
	}
}
