package core

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func driveTestImage(t *testing.T, width int) string {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, image.NewNRGBA(image.Rect(0, 0, width, 4))); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(out.Bytes())
}

func TestDriveAppearanceValidatesImageContentAndStyle(t *testing.T) {
	valid := driveTestImage(t, 128)
	for _, entry := range drivePalette {
		style := entry[0]
		if err := validateDriveAppearance(DriveAppearance{style, valid}); err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range []DriveAppearance{{"#000000", ""}, {"#FFFFFF", ""}, {"#Ab23fF", ""}, {"unsupported", ""}, {"#123", ""}, {"#12345678", ""}, {"#gg0000", ""}, {"red; color: white", ""}, {"", "https://example.test/image.png"}, {"", "data:image/svg+xml;base64,PHN2Zy8+"}, {"", valid[:len(valid)-8]}, {"", driveTestImage(t, 129)}, {"", strings.Repeat("x", 128<<10+1)}} {
		if validateDriveAppearance(a) == nil {
			t.Fatal("invalid appearance accepted")
		}
	}
}

func TestDriveAppearancePreservesMountedRecoveryAndSurvivesReloadAndPortability(t *testing.T) {
	rpc := &fakeRPC{}
	s := testService(t, rpc)
	c := testConnection()
	s.store.Connections = []Connection{c}
	s.mounted[c.ID] = true
	appearance := DriveAppearance{"teal", driveTestImage(t, 32)}
	if err := s.UpdateDriveAppearance(c.ID, appearance); err != nil {
		t.Fatal(err)
	}
	got := s.store.Connections[0]
	got.CardStyle, got.IconImage = "", ""
	if got != c || !s.mounted[c.ID] || len(rpc.calls) != 0 {
		t.Fatal("appearance changed runtime or recovery")
	}
	reloaded, err := loadStore(s.dir)
	if err != nil || reloaded.Connections[0] != s.store.Connections[0] {
		t.Fatal("appearance did not survive reload", err)
	}
	d := portableDefinition(s.store.Connections[0])
	raw, _ := json.Marshal(d)
	var restored PortableConnection
	if json.Unmarshal(raw, &restored) != nil || validatePortableConnection(restored) != nil || restored.connection().IconImage != appearance.IconImage || restored.connection().CardStyle != appearance.CardStyle {
		t.Fatal("portable appearance lost")
	}
	if err := s.UpdateDriveAppearance(c.ID, DriveAppearance{}); err != nil || s.store.Connections[0] != c {
		t.Fatal("reset did not restore defaults", err)
	}
}

func TestDriveAppearanceRejectsActiveOperationsAndRollsBackFailedSave(t *testing.T) {
	for _, state := range []string{"busy", "starting", "setup", "exiting", "locked", "failed-save"} {
		t.Run(state, func(t *testing.T) {
			s := testService(t, &fakeRPC{})
			c := testConnection()
			s.store.Connections = []Connection{c}
			switch state {
			case "busy":
				s.busy[c.ID] = true
			case "starting":
				s.starting = map[string]bool{c.Remote: true}
			case "setup":
				s.session = &ConfigSession{connection: c}
			case "exiting":
				s.exiting = true
			case "locked":
				s.access.locked = true
			case "failed-save":
				if err := os.Mkdir(filepath.Join(s.dir, "settings.json"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if s.UpdateDriveAppearance(c.ID, DriveAppearance{CardStyle: "blue"}) == nil || !reflect.DeepEqual(s.store.Connections, []Connection{c}) {
				t.Fatal("blocked save changed the connection")
			}
		})
	}
}

func TestLegacyCardStylesNormalizeWithoutChangingFilesOrRuntime(t *testing.T) {
	for _, test := range []struct{ old, want string }{
		{"#FF0000", "red"}, {"#000000", "gray"}, {"#FFFFFF", "gray"}, {"#7a83bb", "indigo"}, {"teal", "teal"}, {"invalid", "invalid"}, {"#invalid", "#invalid"},
	} {
		if got := normalizeLegacyCardStyle(test.old); got != test.want {
			t.Fatalf("%s: got %s, want %s", test.old, got, test.want)
		}
	}
	store := defaultStore()
	c := testConnection()
	c.CardStyle = "#7A83BB"
	store.Connections = []Connection{c}
	store.ImportDrafts = []ImportDraft{{ID: strings.Repeat("b", 32), Definition: portableDefinition(c)}}
	raw, _ := json.Marshal(store)
	dir := t.TempDir()
	filename := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(filename, raw, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadStore(dir)
	if err != nil || loaded.Connections[0].CardStyle != "indigo" || loaded.ImportDrafts[0].Definition.CardStyle != "indigo" {
		t.Fatal("legacy appearance did not load", err)
	}
	original, _ := os.ReadFile(filename)
	if !bytes.Equal(original, raw) {
		t.Fatal("reading changed the original settings")
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.backup.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	backup, err := (&Service{dir: dir}).backupStore()
	if err != nil || backup.Connections[0].CardStyle != "indigo" {
		t.Fatal("legacy backup did not normalize", err)
	}
	originalBackup, _ := os.ReadFile(filepath.Join(dir, "settings.backup.json"))
	if !bytes.Equal(originalBackup, raw) {
		t.Fatal("reading changed the backup")
	}
	saved, err := encodeStore(loaded)
	if err != nil || bytes.Contains(saved, []byte("#7A83BB")) {
		t.Fatal("new settings retained RGB", err)
	}
	document := portableFixture()
	document.Connections[0].CardStyle = "#7A83BB"
	raw, _ = json.Marshal(document)
	portable, err := decodePortable(raw)
	if err != nil || portable.Connections[0].CardStyle != "indigo" {
		t.Fatal("legacy portable appearance did not normalize", err)
	}
	s := testService(t, &fakeRPC{})
	s.store.Connections = []Connection{c}
	if s.UpdateDriveAppearance(c.ID, DriveAppearance{CardStyle: "#7A83BB"}) == nil {
		t.Fatal("new RGB update accepted")
	}
}
