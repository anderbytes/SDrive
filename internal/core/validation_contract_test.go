package core

import (
	"errors"
	"strings"
	"testing"

	"sdrive/internal/i18n"
)

func TestDriveNameCodePointsAndPersistence(t *testing.T) {
	for _, unit := range []string{"a", "é", "😀", "e\u0301", "👩‍💻"} {
		t.Run(unit, func(t *testing.T) {
			name := string([]rune(strings.Repeat(unit, 80))[:80])
			connection := testConnection()
			connection.Name = " \u0085" + name + "\u3000 "
			if err := validateConnection(&connection); err != nil || connection.Name != name {
				t.Fatalf("valid name rejected: %v", err)
			}
			dir := t.TempDir()
			store := defaultStore()
			store.Connections = []Connection{connection}
			if err := saveStore(dir, store); err != nil {
				t.Fatal(err)
			}
			loaded, err := loadStore(dir)
			if err != nil || loaded.Connections[0].Name != name {
				t.Fatalf("name did not survive persistence: %v", err)
			}
			connection.Name = name + "a"
			var invalid *FieldError
			if err := validateConnection(&connection); !errors.As(err, &invalid) || invalid.Field != "name" {
				t.Fatalf("81 code points accepted: %v", err)
			}
		})
	}
	connection := testConnection()
	connection.Name = "\uFEFFDrive\uFEFF"
	if err := validateConnection(&connection); err != nil || connection.Name != "\uFEFFDrive\uFEFF" {
		t.Fatalf("BOM trimming differs: %v", err)
	}
}

func TestStructuredMountErrorSnapshotAndRecoveryExit(t *testing.T) {
	s := testService(t, nil)
	connection := testConnection()
	s.store.Connections = []Connection{connection}
	s.winFsp = func() bool { return true }
	s.driveUsed = func(string) bool { return true }
	err := s.Mount(connection.ID)
	if message := i18n.MessageOf(err); message == nil || message.Code != "drive.letter_in_use" || err.Error() != "letter S: is already in use" {
		t.Fatalf("missing mount error metadata: %v", err)
	}
	view := s.Snapshot().Connections[0]
	if view.ErrorMessage == nil || i18n.Format("pt-BR", view.ErrorMessage) != "letra S: já está em uso" || view.Error != err.Error() {
		t.Fatalf("snapshot lost error metadata: %+v", view)
	}
	s.errors[connection.ID] = "newer inspection failure"
	if s.Snapshot().Connections[0].ErrorMessage != nil {
		t.Fatal("stale metadata overrode newer error")
	}
	err = s.Exit()
	if message := i18n.MessageOf(err); message == nil || message.Code != "recovery.required_before_exit" {
		t.Fatalf("missing recovery metadata: %v", err)
	}
	if s.exiting {
		t.Fatal("failed safe exit remained committed")
	}
}
