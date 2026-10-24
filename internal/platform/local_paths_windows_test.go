package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalPathsRejectAmbiguousAndRemoteDestinations(t *testing.T) {
	for _, path := range []string{`relative\folder`, `C:relative`, `\\server\share\cache`, `\\?\C:\cache`, `C:\CON.txt`, `C:\trailing.`, `C:\trailing `, `C:\a\..\b`, `C:\cache:stream`, `C:\cache*`, "C:\\cache\x00"} {
		if _, err := LocalPath(path); err == nil {
			t.Fatalf("unsafe local destination accepted: %q", path)
		}
	}
	path := filepath.Join(t.TempDir(), "Café 日本")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if got, err := LocalPath(path); err != nil || got != path {
		t.Fatal("valid Unicode directory rejected")
	}
	first, err := DirectoryIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+"-original"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	second, err := DirectoryIdentity(path)
	if err != nil || first == second {
		t.Fatal("replaced directory retained its identity")
	}
}
