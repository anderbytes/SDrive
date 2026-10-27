//go:build linux

package platform

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxMountFolderOwnershipPreservesExistingContents(t *testing.T) {
	root := t.TempDir()
	profile := filepath.Join(root, "profile")
	os.Mkdir(profile, 0700)
	path := filepath.Join(root, "mounted")
	if FolderMounted(root) {
		t.Fatal("ordinary directory reported as a mount")
	}
	if err := PrepareMountFolder(profile, "connection", path); err != nil {
		t.Fatal(err)
	}
	if err := CheckMountFolder(profile, "another", path); err == nil {
		t.Fatal("another connection claimed the folder")
	}
	if err := PrepareMountFolder(profile, "connection", path); err != nil {
		t.Fatal("owned empty folder cannot recover", err)
	}
	file := filepath.Join(path, "keep.txt")
	os.WriteFile(file, []byte("keep"), 0600)
	if err := ReleaseMountFolder(profile, "connection", path); err == nil {
		t.Fatal("nonempty folder was removed")
	}
	if b, err := os.ReadFile(file); err != nil || string(b) != "keep" {
		t.Fatal("existing contents changed")
	}
	os.Remove(file)
	if err := ReleaseMountFolder(profile, "connection", path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("owned empty folder remains")
	}
	os.Mkdir(path, 0700)
	if err := CheckMountFolder(profile, "connection", path); err == nil {
		t.Fatal("unowned folder accepted")
	}
}
func TestLinuxLocalPathsAndDirectoryIdentityRejectLinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "data")
	os.Mkdir(target, 0700)
	alias := filepath.Join(root, "link")
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"relative", "/tmp/../private", "C:\\Mount", "/tmp/invalid\npath"} {
		if _, err := LocalPath(path); err == nil {
			t.Fatalf("accepted %q", path)
		}
	}
	if _, err := DirectoryIdentity(alias); err == nil {
		t.Fatal("link accepted")
	}
	if _, err := DirectoryIdentity(target); err != nil {
		t.Fatal(err)
	}
}
func TestLinuxSecretServiceKeysNeverReplaceMissingOrLockedKeys(t *testing.T) {
	oldGet, oldSet := keyGet, keySet
	t.Cleanup(func() { keyGet, keySet = oldGet, oldSet })
	records := map[string]string{}
	keyGet = func(service, user string) (string, error) {
		value, ok := records[user]
		if !ok {
			return "", errors.New("missing")
		}
		return value, nil
	}
	keySet = func(service, user, value string) error { records[user] = value; return nil }
	dir := t.TempDir()
	key, err := Key(dir)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Key(dir)
	if err != nil || key != again {
		t.Fatal("key changed", err)
	}
	sealed, err := ProtectData(dir, []byte("synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := UnprotectData(dir, sealed)
	if err != nil || !bytes.Equal(plain, []byte("synthetic")) {
		t.Fatal("round trip failed", err)
	}
	sealed[len(sealed)-1] ^= 1
	if _, err = UnprotectData(dir, sealed); err == nil {
		t.Fatal("tampered record accepted")
	}
	for reference := range records {
		delete(records, reference)
	}
	if _, err = Key(dir); err == nil || len(records) != 0 {
		t.Fatal("lost key was replaced")
	}
	lost := t.TempDir()
	os.WriteFile(filepath.Join(lost, "proxy.enc"), []byte("preserve"), 0600)
	if _, err = Key(lost); err == nil {
		t.Fatal("missing key accepted with encrypted proxy")
	}
	keySet = func(string, string, string) error { return errors.New("locked") }
	blocked := t.TempDir()
	if _, err = Key(blocked); err == nil {
		t.Fatal("locked keyring accepted")
	}
	if err = CheckKey(blocked); !os.IsNotExist(err) {
		t.Fatal("failed creation published a reference")
	}
}
func TestLinuxProfileLockAndAutostart(t *testing.T) {
	dir := t.TempDir()
	release, err := SingleInstance(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SingleInstance(dir); !errors.Is(err, ErrInstanceRunning) {
		t.Fatal("second owner accepted", err)
	}
	release()
	release, err = SingleInstance(dir)
	if err != nil {
		t.Fatal(err)
	}
	release()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := AutoStart(true); err != nil {
		t.Fatal(err)
	}
	if ok, err := AutoStartMatches(true); err != nil || !ok {
		t.Fatal("autostart did not match", err)
	}
	if err := AutoStart(false); err != nil {
		t.Fatal(err)
	}
	if ok, err := AutoStartMatches(false); err != nil || !ok {
		t.Fatal("autostart remains", err)
	}
}
func TestLinuxSparseAllocationAndAtomicReplace(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sparse")
	f, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(1 << 30); err != nil {
		t.Fatal(err)
	}
	f.Close()
	allocated, err := AllocatedBytes(file)
	if err != nil || allocated >= 1<<30 {
		t.Fatal("sparse allocation counted as logical size", allocated, err)
	}
	if err := AtomicWrite(file, []byte("replacement")); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(file); err != nil || string(b) != "replacement" {
		t.Fatal("replace failed", err)
	}
}
