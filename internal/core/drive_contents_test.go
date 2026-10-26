package core

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestGoogleDriveContentsPreserveLegacyHashesAndBindSharedRecovery(t *testing.T) {
	c := testConnection()
	c.Provider = "drive"
	c.Recovery = false
	config := map[string]string{"type": "drive", "root_folder_id": "original-root"}
	original, err := checkDestination(c, config)
	if err != nil {
		t.Fatal(err)
	}
	config["shared_with_me"] = "false"
	unchanged, _ := checkDestination(c, config)
	if original != unchanged {
		t.Fatal("explicit default changed legacy identity")
	}
	config["shared_with_me"] = "true"
	shared, _ := checkDestination(c, config)
	if shared == original {
		t.Fatal("shared contents missing from identity")
	}
	c.TargetHash = shared
	c.Recovery = true
	config["shared_with_me"] = "false"
	if _, err := checkDestination(c, config); err == nil {
		t.Fatal("recovery changed contents")
	}
	for _, value := range []string{"invalid", "TRUE", "1"} {
		config["shared_with_me"] = value
		if validateDriveContents(config) == nil {
			t.Fatal("invalid contents accepted")
		}
	}
	config["shared_with_me"] = "true"
	config["team_drive"] = "library"
	if validateDriveContents(config) == nil {
		t.Fatal("incompatible library accepted")
	}
	p, _ := provider("drive")
	if _, err := parameters(p, map[string]string{"shared_with_me": "invalid"}, false, ""); err == nil {
		t.Fatal("invalid input accepted")
	}
}

func TestGoogleDriveContentsSetupFieldsAndPortableDefinitions(t *testing.T) {
	for _, shared := range []string{"false", "true"} {
		t.Run(shared, func(t *testing.T) {
			rpc := &fakeRPC{}
			s := testService(t, rpc)
			c := Connection{Name: "Synthetic", Provider: "drive", Letter: "S"}
			id, err := s.BeginConfig(ConfigInput{Connection: c, Guided: true, Values: map[string]string{"client_id": "own-app", "client_secret": "own-secret", "root_folder_id": "explicit-root", "shared_with_me": shared}})
			if err != nil {
				t.Fatal(err)
			}
			ss := waitSession(t, s, id)
			if !ss.Authenticated || ss.Error != "" {
				t.Fatalf("setup: %+v", ss)
			}
			s.mu.Lock()
			saved := s.session.connection
			s.mu.Unlock()
			s.store.Connections = []Connection{saved}
			fields, err := s.ConnectionFields(saved.ID)
			if err != nil || fields.Values["shared_with_me"] != shared || fields.Values["client_secret"] != "" {
				t.Fatalf("fields: %+v %v", fields, err)
			}
			d := portableDefinition(saved)
			for key, value := range fields.Values {
				if portableField("drive", key) {
					d.Values[key] = value
				}
			}
			if d.Values["shared_with_me"] != shared || validatePortableConnection(d) != nil {
				t.Fatal("portable view lost")
			}
			data, _ := json.Marshal(d)
			var restored PortableConnection
			if json.Unmarshal(data, &restored) != nil || !reflect.DeepEqual(restored, d) {
				t.Fatal("portable round trip lost view")
			}
			_ = s.EndConfig(id)
		})
	}
}

func TestGoogleDriveRecoveryRetainsContentsDuringRenewal(t *testing.T) {
	for _, original := range []string{"", "true"} {
		t.Run(original, func(t *testing.T) {
			config := map[string]string{"type": "drive", "root_folder_id": "original-root", "client_id": "own-app", "client_secret": "own-secret", "token": "synthetic"}
			if original != "" {
				config["shared_with_me"] = original
			}
			rpc := &fakeRPC{handle: func(_ context.Context, method string, input any) (any, error) {
				if method == "config/get" {
					return config, nil
				}
				if method == "config/create" {
					raw, _ := json.Marshal(input)
					var request struct{ Parameters map[string]string }
					_ = json.Unmarshal(raw, &request)
					for key, value := range request.Parameters {
						config[key] = value
					}
				}
				return ConfigResult{}, nil
			}}
			s := testService(t, rpc)
			c := testConnection()
			c.Provider = "drive"
			c.Recovery = false
			c.TargetHash, _ = checkDestination(c, config)
			c.Recovery = true
			s.store.Connections = []Connection{c}
			changed := "true"
			same := "false"
			if original == "true" {
				changed = "false"
				same = "true"
			}
			input := ConfigInput{Connection: c, Guided: true, Values: map[string]string{"shared_with_me": changed}}
			if _, err := s.BeginConfig(input); err == nil {
				t.Fatal("recovery accepted changed view")
			}
			input.Values["shared_with_me"] = same
			id, err := s.BeginConfig(input)
			if err != nil {
				t.Fatal(err)
			}
			ss := waitSession(t, s, id)
			if !ss.Authenticated || ss.Error != "" {
				t.Fatalf("renewal failed: %+v", ss)
			}
			_ = s.EndConfig(id)
		})
	}
}

func TestGoogleDriveSetupRejectsChangedEngineViewAndFolderPermissionFailure(t *testing.T) {
	rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
		if method == "config/get" {
			return map[string]string{"type": "drive", "client_id": "own-app", "client_secret": "own-secret", "token": "synthetic", "shared_with_me": "false"}, nil
		}
		return ConfigResult{}, nil
	}}
	s := testService(t, rpc)
	id, err := s.BeginConfig(ConfigInput{Connection: Connection{Name: "Synthetic", Provider: "drive", Letter: "S"}, Guided: true, Values: map[string]string{"client_id": "own-app", "client_secret": "own-secret", "shared_with_me": "true"}})
	if err != nil {
		t.Fatal(err)
	}
	ss := waitSession(t, s, id)
	if ss.Authenticated || ss.Error == "" {
		t.Fatal("engine view mismatch accepted")
	}
	_ = s.EndConfig(id)
	rpc = &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
		if method == "operations/list" {
			return nil, fmt.Errorf("permission denied")
		}
		return ConfigResult{}, nil
	}}
	if _, err := listFolders(context.Background(), rpc, Connection{Name: "Synthetic", Provider: "drive", Letter: "S"}, ""); err == nil {
		t.Fatal("permission failure ignored")
	}
}

func TestYandexCatalogAndUnsupportedNetworkModes(t *testing.T) {
	p, err := provider("yandex")
	if err != nil || !p.OAuth || !p.Available || p.Backend != "yandex" {
		t.Fatalf("provider: %+v %v", p, err)
	}
	for _, patch := range []func(*Connection){func(c *Connection) { c.SessionOnly = true }, func(c *Connection) { c.UseProxy = true }, func(c *Connection) { c.UploadLimit = 1024 }, func(c *Connection) { c.DownloadLimit = 1024 }} {
		c := Connection{Name: "Synthetic", Provider: "yandex", Letter: "S"}
		patch(&c)
		if validateConnection(&c) == nil {
			t.Fatal("unverified capability accepted")
		}
	}
}

func TestGoogleDriveRenewalPreservesSelectedLibrary(t *testing.T) {
	config := map[string]string{"type": "drive", "client_id": "own-app", "client_secret": "own-secret", "token": "synthetic", "team_drive": "original-library"}
	var configured map[string]string
	rpc := &fakeRPC{handle: func(_ context.Context, method string, input any) (any, error) {
		if method == "config/get" {
			return config, nil
		}
		if method == "config/create" {
			raw, _ := json.Marshal(input)
			var request struct{ Parameters map[string]string }
			_ = json.Unmarshal(raw, &request)
			configured = request.Parameters
		}
		return ConfigResult{}, nil
	}}
	s := testService(t, rpc)
	c := testConnection()
	c.Provider = "drive"
	c.Recovery = false
	s.store.Connections = []Connection{c}
	id, err := s.BeginConfig(ConfigInput{Connection: c, Guided: true, Values: map[string]string{"shared_with_me": "false"}})
	if err != nil {
		t.Fatal(err)
	}
	ss := waitSession(t, s, id)
	if !ss.Authenticated || configured["team_drive"] != "original-library" {
		t.Fatalf("renewal changed library: %+v", ss)
	}
	_ = s.EndConfig(id)
}
