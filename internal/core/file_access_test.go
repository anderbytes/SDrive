package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileAccessRestrictsSelectionToMountedRegularFiles(t *testing.T) {
	s := testService(t, nil)
	root := t.TempDir()
	c := testConnection()
	c.Letter, c.MountFolder = "", root
	s.store.Connections = []Connection{c}
	s.mounted[c.ID] = true
	path := filepath.Join(root, "selected.txt")
	if err := os.WriteFile(path, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	SetFileAccessPicker(s, func(string) (string, error) { return path, nil })
	view, err := s.InspectFileAccess(c.ID, "")
	if err != nil || view.Path != "selected.txt" {
		t.Fatalf("selection: %+v %v", view, err)
	}
	for _, relative := range []string{`..\outside.txt`, root, `selected.txt:stream`, "\x00", `.`} {
		if _, err := s.InspectFileAccess(c.ID, relative); err == nil {
			t.Fatalf("invalid selection accepted: %q", relative)
		}
	}
	SetFileAccessPicker(s, func(string) (string, error) { return "", nil })
	if view, err := s.InspectFileAccess(c.ID, ""); err != nil || view.State != "cancelled" {
		t.Fatal("picker cancellation failed")
	}
}

func TestFileAccessDiscardsPickerResultAfterApplicationLocks(t *testing.T) {
	s := testService(t, nil)
	root := t.TempDir()
	c := testConnection()
	c.Letter, c.MountFolder = "", root
	s.store.Connections = []Connection{c}
	s.mounted[c.ID] = true
	// Keep the synthetic mount out of persistent connection validation.
	s.store.Connections = nil
	enableTestAccess(t, s, AccessPolicy{})
	s.store.Connections = []Connection{c}
	path := filepath.Join(root, "selected.txt")
	_ = os.WriteFile(path, []byte("synthetic"), 0600)
	entered, release := make(chan struct{}), make(chan struct{})
	SetFileAccessPicker(s, func(string) (string, error) { close(entered); <-release; return path, nil })
	done := make(chan error, 1)
	go func() { _, err := s.InspectFileAccess(c.ID, ""); done <- err }()
	<-entered
	if _, err := s.InspectFileAccess(c.ID, "selected.txt"); err == nil {
		t.Fatal("overlapping query accepted")
	}
	_ = s.Lock()
	close(release)
	if err := <-done; err == nil {
		t.Fatal("locked picker result exposed")
	}
}
