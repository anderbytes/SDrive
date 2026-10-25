package platform

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/png"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

func TestServiceIconsAreValidAndDistinct(t *testing.T) {
	seen := map[string]bool{}
	for provider, badge := range serviceBadges {
		for _, r := range badge.label {
			if _, ok := badgeGlyphs[r]; !ok {
				t.Fatal("missing service glyph", provider, r)
			}
		}
		icon, err := serviceIcon(provider)
		if err != nil || len(icon) < 22 || binary.LittleEndian.Uint16(icon[2:4]) != 1 || binary.LittleEndian.Uint32(icon[18:22]) != 22 {
			t.Fatal("invalid ICO", provider, err)
		}
		im, err := png.Decode(bytes.NewReader(icon[22:]))
		if err != nil || im.Bounds().Dx() != 32 {
			t.Fatal("invalid image", provider, err)
		}
		if seen[string(icon)] {
			t.Fatal("services share indistinguishable icons", provider)
		}
		seen[string(icon)] = true
	}
}

func TestDriveIconOwnershipPreservesOtherCustomization(t *testing.T) {
	root := fmt.Sprintf(`Software\SDrive\Tests\DriveIcons-%d`, time.Now().UnixNano())
	t.Cleanup(func() { removeIconTestKey(t, root) })
	sync := func(profile, exe string, wanted map[string]string, uninstall bool) {
		t.Helper()
		if err := reconcileDriveIcons(registry.CURRENT_USER, root, profile, exe, wanted, uninstall); err != nil {
			t.Fatal(err)
		}
	}
	path := root + `\S\DefaultIcon`
	get := func() string {
		t.Helper()
		key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
		if err != nil {
			return ""
		}
		defer key.Close()
		v, _, _ := key.GetStringValue("")
		return v
	}
	set := func(value string) {
		t.Helper()
		key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
		if err != nil {
			t.Fatal(err)
		}
		defer key.Close()
		if err := key.SetStringValue("", value); err != nil {
			t.Fatal(err)
		}
	}
	set("user-custom.ico")
	sync("profile", "install.exe", map[string]string{"S": "service.ico"}, false)
	if get() != "user-custom.ico" {
		t.Fatal("existing customization overwritten")
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	if err := key.DeleteValue(""); err != nil {
		t.Fatal(err)
	}
	key.Close()
	sync("profile", "install.exe", map[string]string{"S": "service.ico"}, false)
	if get() != "service.ico" {
		t.Fatal("owned icon not installed")
	}
	sync("other-profile", "install.exe", nil, false)
	if get() != "service.ico" {
		t.Fatal("another profile removed owned icon")
	}
	sync("profile", "install.exe", map[string]string{"S": "next-service.ico"}, false)
	if get() != "next-service.ico" {
		t.Fatal("owned icon not updated")
	}
	set("new-user-custom.ico")
	sync("profile", "install.exe", nil, false)
	if get() != "new-user-custom.ico" {
		t.Fatal("later user customization removed")
	}
	key, _ = registry.OpenKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	_ = key.DeleteValue("")
	key.Close()
	sync("profile", "install.exe", map[string]string{"S": "service.ico"}, false)
	sync("", "other-install.exe", nil, true)
	if get() != "service.ico" {
		t.Fatal("unrelated installation removed icon")
	}
	sync("", "install.exe", nil, true)
	if get() != "" {
		t.Fatal("uninstall left owned icon")
	}
	if key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE); err == nil {
		key.Close()
		t.Fatal("cleanup left an empty owned icon key")
	}
}

func removeIconTestKey(t *testing.T, path string) {
	t.Helper()
	key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.ENUMERATE_SUB_KEYS)
	if err == registry.ErrNotExist {
		return
	}
	if err != nil {
		t.Error(err)
		return
	}
	names, err := key.ReadSubKeyNames(-1)
	key.Close()
	if err != nil {
		t.Error(err)
		return
	}
	for _, name := range names {
		removeIconTestKey(t, path+`\`+name)
	}
	if err := registry.DeleteKey(registry.CURRENT_USER, path); err != nil {
		t.Error(err)
	}
}
