package platform

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestDataDirLockChild(t *testing.T) {
	dir := os.Getenv("SDRIVE_LOCK_TEST_DIR")
	if dir == "" {
		return
	}
	unlock, err := lockDataDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	fmt.Println("LOCKED")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}

func TestDataDirOwnershipAcrossProcessesAndCrash(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestDataDirLockChild$")
	cmd.Env = append(os.Environ(), "SDRIVE_LOCK_TEST_DIR="+dir)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stdin.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	ready, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || ready != "LOCKED\n" {
		t.Fatalf("child lock: %q, %v", ready, err)
	}
	if unlock, err := lockDataDir(dir); err == nil {
		unlock()
		t.Fatal("second process admitted")
	}
	alias := t.TempDir()
	if err := os.Link(filepath.Join(dir, "instance.lock"), filepath.Join(alias, "instance.lock")); err != nil {
		t.Fatal(err)
	}
	if unlock, err := lockDataDir(alias); err == nil {
		unlock()
		t.Fatal("same file admitted through alias")
	}
	independent, err := lockDataDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	independent()
	path, _ := windows.UTF16PtrFromString(filepath.Join(dir, "instance.lock"))
	handle, err := windows.CreateFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err == nil {
		windows.CloseHandle(handle)
		t.Fatal("installer probe missed live owner")
	}
	if err != windows.ERROR_SHARING_VIOLATION {
		t.Fatalf("installer probe: %v", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	unlock, err := lockDataDir(dir)
	if err != nil {
		t.Fatal("crashed process retained lock:", err)
	}
	unlock()
}

func TestSingleInstanceKeepsLegacyInstallerMarker(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("USERNAME", "sdrive-test-"+filepath.Base(dir))
	unlock, err := SingleInstance(dir)
	if err != nil {
		t.Fatal(err)
	}
	name, _ := windows.UTF16PtrFromString(`Local\SDrive-` + os.Getenv("USERNAME"))
	handle, err := windows.OpenMutex(windows.SYNCHRONIZE, false, name)
	if err != nil {
		unlock()
		t.Fatal("legacy installer marker missing:", err)
	}
	windows.CloseHandle(handle)
	unlock()
	next, err := SingleInstance(dir)
	if err != nil {
		t.Fatal("instance marker retained after release:", err)
	}
	next()
}

func TestIndependentProfilesKeepSeparateOwnershipAndActivation(t *testing.T) {
	dir, other := t.TempDir(), t.TempDir()
	t.Setenv("USERNAME", "sdrive-test-"+filepath.Base(dir))
	first, err := SingleInstance(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer first()
	second, err := SingleInstance(other)
	if err != nil {
		t.Fatal("legacy marker blocked an independent profile", err)
	}
	defer second()
	id, err := InstanceID(dir)
	if err != nil {
		t.Fatal(err)
	}
	same, err := InstanceID(filepath.Join(dir, "."))
	if err != nil {
		t.Fatal(err)
	}
	different, err := InstanceID(other)
	if err != nil {
		t.Fatal(err)
	}
	if id != same || id == different {
		t.Fatal("profile activation identity is not scoped correctly")
	}
}
