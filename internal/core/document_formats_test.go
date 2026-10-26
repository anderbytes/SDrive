package core

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestDocumentFormatChangeRequiresCleanDisconnectedCache(t *testing.T) {
	for _, metadata := range []string{`{"Dirty":true}`, `broken`} {
		s := testService(t, &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
			if method == "config/get" {
				return map[string]string{"type": "drive", "client_id": "own-app", "client_secret": "own-secret", "token": "synthetic"}, nil
			}
			return ConfigResult{}, nil
		}})
		c := testConnection()
		c.Provider, c.Recovery = "drive", false
		s.store.Connections = []Connection{c}
		data := "preserve pending content"
		path := cacheFixture(t, s, c, metadata, &data)
		c.Remote = "untrusted-empty-cache"
		input := ConfigInput{Connection: c, Guided: true, Values: map[string]string{"export_formats": "pdf,docx,xlsx,pptx,svg"}}
		if _, err := s.BeginConfig(input); err == nil {
			t.Fatal("format change accepted pending or unknown cache")
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != metadata {
			t.Fatal("cache metadata changed")
		}
	}
}

func TestDocumentFormatsBindRecoveryAndPreserveDefaultIdentity(t *testing.T) {
	c := testConnection()
	c.Provider, c.Recovery = "drive", false
	config := map[string]string{"type": "drive"}
	legacy, _ := checkDestination(c, config)
	config["export_formats"] = "docx,xlsx,pptx,svg"
	if hash, _ := checkDestination(c, config); hash != legacy {
		t.Fatal("explicit default changed legacy recovery identity")
	}
	config["export_formats"] = "pdf,docx,xlsx,pptx,svg"
	pdf, _ := checkDestination(c, config)
	if pdf == legacy {
		t.Fatal("document representation missing from recovery identity")
	}
	c.Recovery, c.TargetHash = true, pdf
	config["export_formats"] = "docx,xlsx,pptx,svg"
	if _, err := checkDestination(c, config); err == nil {
		t.Fatal("recovery accepted incompatible document extensions")
	}
}

func TestDocumentFormatsSetupFieldsAndPortableDefinitions(t *testing.T) {
	for _, format := range []string{"docx,xlsx,pptx,svg", "pdf,docx,xlsx,pptx,svg", "odt,ods,odp,svg"} {
		t.Run(format, func(t *testing.T) {
			s := testService(t, &fakeRPC{})
			c := Connection{Name: "Synthetic", Provider: "drive", Letter: "S"}
			id, err := s.BeginConfig(ConfigInput{Connection: c, Guided: true, Values: map[string]string{"client_id": "own-app", "client_secret": "own-secret", "export_formats": format}})
			if err != nil {
				t.Fatal(err)
			}
			ss := waitSession(t, s, id)
			if !ss.Authenticated || ss.Error != "" {
				t.Fatalf("setup: %+v", ss)
			}
			s.store.Connections = []Connection{ss.connection}
			fields, err := s.ConnectionFields(ss.connection.ID)
			if err != nil || fields.Values["export_formats"] != format {
				t.Fatalf("fields: %+v %v", fields, err)
			}
			d := portableDefinition(ss.connection)
			d.Values["export_formats"] = fields.Values["export_formats"]
			raw, _ := json.Marshal(d)
			var restored PortableConnection
			if json.Unmarshal(raw, &restored) != nil || validatePortableConnection(restored) != nil || restored.Values["export_formats"] != format {
				t.Fatal("portable definition lost document format")
			}
			_ = s.EndConfig(id)
		})
	}
	p, _ := provider("drive")
	if _, err := parameters(p, map[string]string{"export_formats": "invalid"}, false, ""); err == nil {
		t.Fatal("invalid document format accepted")
	}
}

func TestDocumentFormatRecoveryAllowsOnlyOriginalFormat(t *testing.T) {
	for _, original := range []string{"", "pdf,docx,xlsx,pptx,svg"} {
		config := map[string]string{"type": "drive", "client_id": "own-app", "client_secret": "own-secret", "token": "synthetic", "export_formats": original}
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
		c.Provider, c.Recovery = "drive", false
		c.TargetHash, _ = checkDestination(c, config)
		c.Recovery = true
		s.store.Connections = []Connection{c}
		same, changed := "docx,xlsx,pptx,svg", "pdf,docx,xlsx,pptx,svg"
		if original != "" {
			same, changed = changed, same
		}
		input := ConfigInput{Connection: c, Guided: true, Values: map[string]string{"export_formats": changed}}
		if _, err := s.BeginConfig(input); err == nil {
			t.Fatal("recovery allowed changed export format")
		}
		input.Values["export_formats"] = same
		id, err := s.BeginConfig(input)
		if err != nil {
			t.Fatal(err)
		}
		ss := waitSession(t, s, id)
		if !ss.Authenticated || ss.Error != "" {
			t.Fatalf("renewal: %+v", ss)
		}
		_ = s.EndConfig(id)
	}
}
