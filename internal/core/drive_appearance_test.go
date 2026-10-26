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
	for _, style := range []string{"", "blue", "violet", "teal", "amber"} {
		if err := validateDriveAppearance(DriveAppearance{style, valid}); err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range []DriveAppearance{{"red", ""}, {"", "https://example.test/image.png"}, {"", "data:image/svg+xml;base64,PHN2Zy8+"}, {"", valid[:len(valid)-8]}, {"", driveTestImage(t, 129)}, {"", strings.Repeat("x", 128<<10+1)}} {
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
