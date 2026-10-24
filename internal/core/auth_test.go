package core

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestFTPRenewalRestoresTransportAndPreservesRecovery(t *testing.T) {
	for _, mode := range []string{"explicit", "implicit", "plain"} {
		t.Run(mode, func(t *testing.T) {
			config := map[string]string{"type": "ftp", "host": "example.test", "user": "original", "pass": "synthetic-secret"}
			if mode == "explicit" {
				config["explicit_tls"] = "true"
			}
			if mode == "implicit" {
				config["tls"], config["port"] = "true", "990"
			}
			var updated map[string]string
			rpc := &fakeRPC{handle: func(_ context.Context, method string, input any) (any, error) {
				switch method {
				case "config/get":
					return config, nil
				case "config/update":
					data, _ := json.Marshal(input)
					var request struct{ Parameters map[string]string }
					_ = json.Unmarshal(data, &request)
					updated = request.Parameters
					for k, v := range updated {
						config[k] = v
					}
				}
				return ConfigResult{}, nil
			}}
			s := testService(t, rpc)
			c := testConnection()
			c.Recovery = false
			c.TargetHash, _ = checkDestination(c, config)
			c.Recovery = true
			s.store.Connections = []Connection{c}
			fields, err := s.ConnectionFields(c.ID)
			if err != nil || fields.Transport != mode || fields.Values["pass"] != "" {
				t.Fatalf("unsafe fields: %v", err)
			}
			fields.Values["pass"] = "renewed-synthetic-secret"
			input := ConfigInput{Connection: c, Values: fields.Values, Transport: fields.Transport}
			if mode == "plain" {
				if _, err := s.BeginConfig(input); err == nil {
					t.Fatal("plain FTP renewed without consent")
				}
				input.AllowInsecure = true
			}
			changed := input
			changed.Transport = "explicit"
			if mode == "explicit" {
				changed.Transport = "implicit"
			}
			if _, err := s.BeginConfig(changed); err == nil {
				t.Fatal("recovery transport changed")
			}
			id, err := s.BeginConfig(input)
			if err != nil {
				t.Fatal(err)
			}
			ss := waitSession(t, s, id)
			if !ss.Complete || !ss.Authenticated || ss.Error != "" {
				t.Fatalf("renewal failed: %s", ss.Error)
			}
			if !reflect.DeepEqual(s.store.Connections, []Connection{c}) {
				t.Fatal("recovery destination changed")
			}
			wantTLS, wantExplicit := "false", "false"
			if mode == "implicit" {
				wantTLS = "true"
			}
			if mode == "explicit" {
				wantExplicit = "true"
			}
			if updated["tls"] != wantTLS || updated["explicit_tls"] != wantExplicit {
				t.Fatal("stale TLS flags retained")
			}
		})
	}
}

func TestConnectionFieldsRejectsInvalidSavedConfiguration(t *testing.T) {
	for _, config := range []map[string]string{nil, {}, {"type": "webdav"}, {"type": "ftp", "tls": "unknown"}, {"type": "ftp", "tls": "true", "explicit_tls": "true"}} {
		s := testService(t, &fakeRPC{handle: func(context.Context, string, any) (any, error) { return config, nil }})
		c := testConnection()
		s.store.Connections = []Connection{c}
		if _, err := s.ConnectionFields(c.ID); err == nil {
			t.Fatal("invalid access details accepted")
		}
	}
}

func TestSetupRejectsInvalidCompletionAndSavedRemote(t *testing.T) {
	valid := map[string]string{"type": "webdav", "url": "https://example.test/dav"}
	for _, tc := range []struct {
		name, response string
		config         map[string]string
		want           bool
	}{
		{"null", `null`, valid, false},
		{"empty", `{}`, valid, false},
		{"missing option", `{"State":"","Error":""}`, valid, false},
		{"null state", `{"State":null,"Option":null,"Error":""}`, valid, false},
		{"wrong type", `{"State":7,"Option":null,"Error":""}`, valid, false},
		{"question without state", `{"State":"","Option":{"Name":"config_mfa"},"Error":""}`, valid, false},
		{"state without question", `{"State":"next","Option":null,"Error":""}`, valid, false},
		{"missing remote", `{"State":"","Option":null,"Error":""}`, nil, false},
		{"empty remote", `{"State":"","Option":null,"Error":""}`, map[string]string{}, false},
		{"wrong backend", `{"State":"","Option":null,"Error":""}`, map[string]string{"type": "ftp", "url": "https://example.test/dav"}, false},
		{"missing URL", `{"State":"","Option":null,"Error":""}`, map[string]string{"type": "webdav"}, false},
		{"complete", `{"State":"","Option":null,"Error":""}`, valid, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
				if method == "config/create" {
					return json.RawMessage(tc.response), nil
				}
				if method == "config/get" {
					return tc.config, nil
				}
				return ConfigResult{}, nil
			}}
			s := testService(t, rpc)
			id, err := s.BeginConfig(ConfigInput{Connection: Connection{Name: "Test", Provider: "webdav", Letter: "S"}, Values: map[string]string{"url": valid["url"]}, Guided: true})
			if err != nil {
				t.Fatal(err)
			}
			ss := waitSession(t, s, id)
			if ss.Authenticated != tc.want || ss.Complete || len(s.store.Connections) != 0 {
				t.Fatal("unverified setup persisted or authenticated")
			}
			if !tc.want && ss.Error == "" {
				t.Fatal("invalid result has no feedback")
			}
		})
	}
}

func TestConfiguredRemoteRequiresOAuthOrAlternateCredentials(t *testing.T) {
	for _, id := range []string{"drive", "gcs", "azureblob"} {
		p, _ := provider(id)
		config := map[string]string{"type": p.Backend, "account": "test"}
		if validateConfiguredRemote(p, config) == nil {
			t.Fatal("missing credentials accepted")
		}
		switch id {
		case "drive":
			config["client_id"] = "app"
			config["client_secret"] = "secret"
			config["token"] = "synthetic-token"
		case "gcs":
			config["service_account_file"] = "synthetic.json"
		case "azureblob":
			config["sas_url"] = "https://example.test/container?sig=synthetic"
		}
		if err := validateConfiguredRemote(p, config); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCredentialModesHaveInstructions(t *testing.T) {
	for _, tc := range []struct {
		provider, key  string
		file, advanced bool
	}{
		{"sftp", "key_file", true, false}, {"sftp", "key_file_pass", false, true},
		{"azureblob", "key", false, false}, {"azureblob", "sas_url", false, false},
		{"gcs", "service_account_file", true, false},
	} {
		p, _ := provider(tc.provider)
		found := false
		for _, f := range p.Fields {
			if f.Key == tc.key {
				found = true
				if f.Hint == "" || f.File != tc.file || f.Advanced != tc.advanced {
					t.Fatalf("missing mode instructions or controls for %s.%s", tc.provider, tc.key)
				}
			}
		}
		if !found {
			t.Fatal("credential mode disappeared")
		}
	}
}

func TestRecoveryRenewalVerifiesLibraryBeforeSuccess(t *testing.T) {
	for _, changeLibrary := range []bool{false, true} {
		t.Run(fmt.Sprint(changeLibrary), func(t *testing.T) {
			config := map[string]string{"type": "onedrive", "drive_id": "original-library", "token": "synthetic-token", "client_id": "custom-public-id", "client_secret": "custom-secret"}
			rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
				if method == "config/get" {
					return config, nil
				}
				if method == "config/update" {
					config["token"] = "renewed-synthetic-token"
					if changeLibrary {
						config["drive_id"] = "another-library"
					}
				}
				return ConfigResult{}, nil
			}}
			s := testService(t, rpc)
			c := testConnection()
			c.Provider, c.Recovery = "onedrive", false
			c.TargetHash, _ = checkDestination(c, config)
			c.Recovery = true
			s.store.Connections = []Connection{c}
			id, err := s.BeginConfig(ConfigInput{Connection: c, Guided: true})
			if err != nil {
				t.Fatal(err)
			}
			ss := waitSession(t, s, id)
			if ss.Authenticated == changeLibrary || ss.Complete {
				t.Fatalf("unsafe authentication state: %s", ss.Error)
			}
			if !reflect.DeepEqual(s.store.Connections, []Connection{c}) {
				t.Fatal("original recovery metadata changed")
			}
			if err := s.EndConfig(id); err != nil {
				t.Fatal(err)
			}
			for _, call := range rpc.calls {
				if call == "config/delete" {
					t.Fatal("recovery credentials deleted")
				}
			}
		})
	}
}

func TestSensitiveQuestionsWithoutProviderHintsAreMasked(t *testing.T) {
	for _, name := range []string{"config_mfa", "config_verification", "config_otp", "config_2fa", "config_mailbox_password", "config_token"} {
		for _, kind := range []string{"", "string", "int"} {
			rpc := &fakeRPC{handle: func(context.Context, string, any) (any, error) {
				return ConfigResult{State: "next", Option: &Question{Name: name, Type: kind, DefaultStr: "synthetic-secret", Examples: []Example{{Value: "synthetic-secret"}}}}, nil
			}}
			s := testService(t, rpc)
			id, err := s.BeginConfig(ConfigInput{Connection: Connection{Name: "Test", Provider: "webdav", Letter: "S"}, Values: map[string]string{"url": "https://example.test/dav"}, Guided: true})
			if err != nil {
				t.Fatal(err)
			}
			ss := waitSession(t, s, id)
			if ss.Question == nil || !ss.Question.Sensitive || ss.Question.DefaultStr != "" || len(ss.Question.Examples) != 0 {
				t.Fatalf("unmasked %s (%s)", name, kind)
			}
		}
	}
}

type testSetupFailure string

func (e testSetupFailure) FailureKind() string { return string(e) }
func (e testSetupFailure) Error() string       { return "synthetic-private-response" }

func TestSetupFailuresAreCategorizedWithoutExposingResponses(t *testing.T) {
	for _, kind := range []string{"network", "credentials", "permission", "engine", "invalid", "unknown"} {
		s := testService(t, &fakeRPC{handle: func(context.Context, string, any) (any, error) {
			return nil, testSetupFailure(kind)
		}})
		id, err := s.BeginConfig(ConfigInput{Connection: Connection{Name: "Test", Provider: "webdav", Letter: "S"}, Values: map[string]string{"url": "https://example.test/dav"}, Guided: true})
		if err != nil {
			t.Fatal(err)
		}
		ss := waitSession(t, s, id)
		if ss.Authenticated || ss.Complete || len(s.store.Connections) != 0 {
			t.Fatal("failed authentication persisted")
		}
		message := ss.Error
		if strings.Contains(message, "synthetic-private") || message == "" {
			t.Fatal("unsafe failure message")
		}
		if kind != "unknown" && message == setupFailure(fmt.Errorf("unknown")) {
			t.Fatalf("lost category %s", kind)
		}
	}
}

func TestCancelDuringRemoteVerificationDoesNotAuthenticateOrCommit(t *testing.T) {
	started, finished := make(chan struct{}), make(chan struct{})
	rpc := &fakeRPC{handle: func(ctx context.Context, method string, _ any) (any, error) {
		if method == "config/get" {
			close(started)
			<-ctx.Done()
			close(finished)
			return map[string]string{"type": "webdav", "url": "https://example.test/dav"}, nil
		}
		return ConfigResult{}, nil
	}}
	s := testService(t, rpc)
	id, err := s.BeginConfig(ConfigInput{Connection: Connection{Name: "Test", Provider: "webdav", Letter: "S"}, Values: map[string]string{"url": "https://example.test/dav"}})
	if err != nil {
		t.Fatal(err)
	}
	receive(t, started)
	if ss, err := s.ConfigStatus(id); err != nil || !ss.Busy {
		t.Fatal("verification blocked status or released reservation")
	}
	if err := s.EndConfig(id); err != nil {
		t.Fatal(err)
	}
	receive(t, finished)
	if len(s.store.Connections) != 0 {
		t.Fatal("cancelled verification committed")
	}
}
