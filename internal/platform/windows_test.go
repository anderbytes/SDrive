package platform

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"unsafe"
)

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
