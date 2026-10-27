package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestIndependentMountedClientsDoNotShareExclusiveFileHandles(t *testing.T) {
	requireNativeMount(t)
	f := newIntegrationFixture(t, true)
	if err := os.WriteFile(filepath.Join(f.destination, "library", "shared.txt"), []byte("unchanged disposable content"), 0600); err != nil {
		t.Fatal(err)
	}
	first, _ := openIntegrationService(t, f.profile, nil)
	c1 := configureIntegration(t, first, f.url, f.letter)
	isolatedConnectionWorkers(t, first, f.profile)
	if err := first.Mount(c1.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Unmount(c1.ID) })
	profile := filepath.Join(f.root, "other-client")
	second, _ := openIntegrationService(t, profile, nil)
	c2 := configureIntegration(t, second, f.url, "")
	c2.Name = "Shared editing peer"
	if err := second.UpdateConnection(c2); err != nil {
		t.Fatal(err)
	}
	isolatedConnectionWorkers(t, second, profile)
	if err := second.Mount(c2.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Unmount(c2.ID) })
	t.Logf("Disposable mount letters: %s and %s", c1.Letter, c2.Letter)
	waitIntegration(t, "second mount file visibility", 5*time.Second, func() bool { _, err := os.Stat(c2.Letter + `:\shared.txt`); return err == nil })
	openExclusive := func(letter string) (windows.Handle, error) {
		path, err := windows.UTF16PtrFromString(letter + `:\shared.txt`)
		if err != nil {
			return windows.InvalidHandle, err
		}
		return windows.CreateFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	}
	h1, err := openExclusive(c1.Letter)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h1)
	if duplicate, err := openExclusive(c1.Letter); err == nil {
		windows.CloseHandle(duplicate)
		t.Fatal("same mount ignored local exclusive sharing")
	}
	h2, err := openExclusive(c2.Letter)
	if err != nil {
		t.Fatalf("baseline changed: second client no longer acquires an independent handle: %v", err)
	}
	defer windows.CloseHandle(h2)
	t.Log("Two isolated clients acquired exclusive local handles simultaneously: distributed editing protection is unsupported")
}
