package platform

import (
	"fmt"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

func TestDriveLabelsUpdateAndCleanupOnlyOwnedUnchangedValues(t *testing.T) {
	root := fmt.Sprintf(`Software\SDrive\Tests\DriveLabels-%d`, time.Now().UnixNano())
	t.Cleanup(func() { removeIconTestKey(t, root) })
	const mount = "##server#Library"
	set := func(value string) {
		t.Helper()
		key, _, err := registry.CreateKey(registry.CURRENT_USER, root+`\`+mount, registry.SET_VALUE)
		if err != nil {
			t.Fatal(err)
		}
		defer key.Close()
		if err := key.SetStringValue("_LabelFromReg", value); err != nil {
			t.Fatal(err)
		}
		if err := key.SetStringValue("Unrelated", "preserve"); err != nil {
			t.Fatal(err)
		}
	}
	check := func(profile, exe, desired, expected string) {
		t.Helper()
		wanted := map[string]string{}
		if desired != "" {
			wanted[mount] = desired
		}
		if _, err := reconcileDriveLabels(root, profile, exe, wanted); err != nil {
			t.Fatal(err)
		}
		key, err := registry.OpenKey(registry.CURRENT_USER, root+`\`+mount, registry.QUERY_VALUE)
		if err != nil {
			t.Fatal(err)
		}
		defer key.Close()
		got, _, _ := key.GetStringValue("_LabelFromReg")
		if got != expected {
			t.Fatalf("label = %q, want %q", got, expected)
		}
		if value, _, _ := key.GetStringValue("Unrelated"); value != "preserve" {
			t.Fatal("unrelated metadata changed")
		}
	}
	set("User label")
	check("profile", "app.exe", "Library", "User label")
	key, _ := registry.OpenKey(registry.CURRENT_USER, root+`\`+mount, registry.SET_VALUE)
	if err := key.DeleteValue("_LabelFromReg"); err != nil {
		t.Fatal(err)
	}
	key.Close()
	check("profile", "app.exe", "Library", "Library")
	check("profile", "app.exe", "Library", "Library")
	check("other", "app.exe", "", "Library")
	check("", "other.exe", "", "Library")
	check("profile", "app.exe", "Renamed", "Renamed")
	set("Later user label")
	check("profile", "app.exe", "", "Later user label")
	key, _ = registry.OpenKey(registry.CURRENT_USER, root+`\`+mount, registry.SET_VALUE)
	if err := key.DeleteValue("_LabelFromReg"); err != nil {
		t.Fatal(err)
	}
	key.Close()
	check("profile", "app.exe", "Library", "Library")
	check("profile", "app.exe", "", "")
	check("profile", "app.exe", "Library", "Library")
	check("", "app.exe", "", "")
}
