package core

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestLinuxRejectsDriveLettersReportedByEngine(t *testing.T) {
	rpc := &fakeRPC{handle: func(context.Context, string, any) (any, error) {
		return map[string]any{"mountPoints": []map[string]string{{"Fs": "remote:", "MountPoint": "S:"}}}, nil
	}}
	if _, err := readMounts(context.Background(), rpc); err == nil {
		t.Fatal("Linux accepted a Windows mount point")
	}
}

func TestLinuxMountOptionsAndCaseSensitiveContainment(t *testing.T) {
	root := t.TempDir()
	c := Connection{ID: newID(), Name: "Cloud", Provider: "webdav", MountFolder: filepath.Join(root, "Cloud"), MountMode: "local"}
	if err := validateMountTarget(&c); err != nil {
		t.Fatal(err)
	}
	if mountTargetsOverlap(c, Connection{MountFolder: filepath.Join(root, "cloud")}) {
		t.Fatal("case-distinct folders overlap")
	}
	if !pathWithin(filepath.Join(c.MountFolder, "file"), c.MountFolder) || pathWithin(c.MountFolder+"-other", c.MountFolder) {
		t.Fatal("unsafe containment")
	}
	for _, readOnly := range []bool{false, true} {
		c.ReadOnly = readOnly
		options := mountOptions(c, defaultStore().Settings)
		mount := options["mountOpt"].(map[string]any)
		for _, key := range []string{"NetworkMode", "ExtraOptions", "VolumeName"} {
			if _, ok := mount[key]; ok {
				t.Fatal("Windows mount option leaked", key)
			}
		}
		if options["mountType"] != "mount2" || mount["AllowOther"] != false {
			t.Fatal("unsafe Linux mount options")
		}
	}
	c.Letter = "S"
	if err := validateMountTarget(&c); err == nil {
		t.Fatal("drive letter accepted")
	}
}
func TestLinuxPortablePreviewAcceptsForeignDestinationsButCommitRequiresLocal(t *testing.T) {
	document := portableFixture()
	document.Connections[0].Letter = "S"
	document.Connections[0].MountFolder = ""
	document.Connections[0].MountMode = "network"
	raw, _ := json.Marshal(document)
	if _, err := decodePortable(raw); err != nil {
		t.Fatal("foreign preview rejected", err)
	}
	c := document.Connections[0].connection()
	if err := validateConnection(&c); err == nil {
		t.Fatal("foreign destination allowed for local use")
	}
	c.Letter = ""
	c.MountMode = "local"
	c.MountFolder = filepath.Join(t.TempDir(), "mounted")
	if err := validateConnection(&c); err != nil {
		t.Fatal(err)
	}
}
