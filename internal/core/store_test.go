package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStoreRoundTripPreservesRecovery(t *testing.T) {
	dir := t.TempDir()
	want := Store{Settings: Settings{Theme: "system", CacheGiB: 8, Language: "pt-BR"}, Connections: []Connection{testConnection()}}
	want.Connections[0].TargetHash = strings.Repeat("a", 64)
	if err := saveStore(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadStore(dir)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip: got %+v, error %v; want %+v", got, err, want)
	}
}

func TestLoadStoreRejectsInvalidDataWithoutChangingFile(t *testing.T) {
	for _, name := range []string{"cache below minimum", "cache above maximum", "invalid ID", "invalid remote", "duplicate ID", "duplicate letter", "unknown provider", "unsafe path"} {
		t.Run(name, func(t *testing.T) {
			store := Store{Settings: Settings{CacheGiB: 2}, Connections: []Connection{testConnection()}}
			c := &store.Connections[0]
			switch name {
			case "cache below minimum":
				store.Settings.CacheGiB = 0
			case "cache above maximum":
				store.Settings.CacheGiB = 1025
			case "invalid ID":
				c.ID = "not-an-id"
			case "invalid remote":
				c.Remote = "../outside"
			case "duplicate ID", "duplicate letter":
				other := *c
				if name == "duplicate ID" {
					other.Letter = "T"
				} else {
					other.ID = strings.Repeat("3", 32)
					other.Letter = " s "
				}
				store.Connections = append(store.Connections, other)
			case "unknown provider":
				c.Provider = "unknown"
			case "unsafe path":
				c.Path = "../outside"
			}
			data, err := json.Marshal(store)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "settings.json")
			if err = os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = loadStore(dir); err == nil {
				t.Fatal("invalid settings accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != string(data) {
				t.Fatalf("rejected settings were changed: %v", err)
			}
		})
	}
}

func TestFailedConnectionWriteRollsBackMemoryAndPreservesCredentials(t *testing.T) {
	for _, operation := range []string{"update", "remove", "configure"} {
		t.Run(operation, func(t *testing.T) {
			rpc := &fakeRPC{}
			s := testService(t, rpc)
			c := testConnection()
			c.Recovery = false
			s.store.Connections = []Connection{c}
			// A directory at the file's destination fails replacement on Windows, even as administrator.
			if err := os.Mkdir(filepath.Join(s.dir, "settings.json"), 0700); err != nil {
				t.Fatal(err)
			}
			changed := c
			changed.Name = "Changed"
			switch operation {
			case "update":
				if s.UpdateConnection(changed) == nil {
					t.Fatal("write failure ignored")
				}
			case "remove":
				if s.Remove(c.ID) == nil {
					t.Fatal("write failure ignored")
				}
			case "configure":
				ss := &ConfigSession{connection: changed, oldRemote: c.Remote}
				if s.commitConfig(ss) == "" || ss.Complete {
					t.Fatal("failed setup marked complete")
				}
			}
			if !reflect.DeepEqual(s.store.Connections, []Connection{c}) {
				t.Fatalf("original connection lost: %+v", s.store.Connections)
			}
			for _, method := range rpc.calls {
				if method == "config/delete" {
					t.Fatal("credentials deleted before saving settings")
				}
			}
			partials, err := filepath.Glob(filepath.Join(s.dir, ".sdrive-*"))
			if err != nil || len(partials) != 0 {
				t.Fatalf("temporary settings left behind: %v %v", partials, err)
			}
		})
	}
}
