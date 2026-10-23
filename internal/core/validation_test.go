package core

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParameterFieldErrors(t *testing.T) {
	for _, tt := range []struct {
		provider, field, value string
	}{
		{"sftp", "host", "https://secret.test"},
		{"sftp", "host", "user@secret.test"},
		{"sftp", "host", "host:22"},
		{"sftp", "port", "0"},
		{"sftp", "port", "65536"},
		{"sftp", "port", "not-a-port"},
		{"webdav", "url", "http://secret.test"},
		{"webdav", "url", "https://user:secret@host.test"},
		{"s3", "endpoint", "relative-secret"},
		{"azureblob", "sas_url", "http://host.test?secret=value"},
		{"sftp", "pass", "secret\nvalue"},
		{"sftp", "user", ""},
	} {
		t.Run(tt.provider+"/"+tt.field+"/"+tt.value, func(t *testing.T) {
			p, _ := provider(tt.provider)
			values := fixture(p)
			values[tt.field] = tt.value
			_, err := parameters(p, values, false, "")
			var invalid *FieldError
			if !errors.As(err, &invalid) || invalid.Field != tt.field {
				t.Fatalf("expected field %s, got %v", tt.field, err)
			}
			// Wails marshals the error interface into RuntimeError.cause.
			data, marshalErr := json.Marshal(&err)
			var cause FieldError
			if marshalErr != nil || json.Unmarshal(data, &cause) != nil || !reflect.DeepEqual(cause, *invalid) || len(data) == 0 || strings.Contains(string(data), "secret") {
				t.Fatalf("invalid or sensitive error cause: %s (%v)", data, marshalErr)
			}
		})
	}
}

func TestServerValidationSharedWithSSHProbe(t *testing.T) {
	p, _ := provider("sftp")
	for _, host := range []string{"files.example.test", "127.0.0.1", "::1", "[2001:db8::1]"} {
		values := fixture(p)
		values["host"], values["port"] = host, "65535"
		if _, err := parameters(p, values, false, ""); err != nil {
			t.Fatalf("valid server %q rejected: %v", host, err)
		}
	}
	for _, tt := range []struct{ host, port, field string }{
		{"", "", "host"}, {"bad host", "22", "host"}, {"host:22", "", "host"},
		{"host", "65536", "port"}, {"host", "invalid", "port"},
	} {
		_, err := (&Service{}).ProbeSSH(tt.host, tt.port)
		var invalid *FieldError
		if !errors.As(err, &invalid) || invalid.Field != tt.field {
			t.Fatalf("probe failed to validate %s before networking: %v", tt.field, err)
		}
	}
}

func TestConnectionValidationIdentifiesNameAndPath(t *testing.T) {
	for _, path := range []string{"/absolute", "C:/absolute", "a\\b", "a/../b", "../outside", "a\x00b", "a\nb"} {
		c := testConnection()
		c.Path = path
		var invalid *FieldError
		if err := validateConnection(&c); !errors.As(err, &invalid) || invalid.Field != "path" {
			t.Fatalf("path %q: %v", path, err)
		}
	}
	for _, name := range []string{"", "name\ninvalid", strings.Repeat("a", 81)} {
		c := testConnection()
		c.Name = name
		var invalid *FieldError
		if err := validateConnection(&c); !errors.As(err, &invalid) || invalid.Field != "name" {
			t.Fatalf("name %q: %v", name, err)
		}
	}
	for _, path := range []string{"", "bucket/folder", "Documents/", "a/..hidden"} {
		c := testConnection()
		c.Path = path
		if err := validateConnection(&c); err != nil {
			t.Fatalf("valid path %q rejected: %v", path, err)
		}
	}
}

func TestInvalidPickerPathsNeverReachRPC(t *testing.T) {
	rpc := &fakeRPC{}
	s := testService(t, rpc)
	c := testConnection()
	s.session = &ConfigSession{ID: "draft", Authenticated: true, connection: c, rpc: rpc, ctx: context.Background()}
	for _, path := range []string{"../outside", "/absolute", "a\\b"} {
		if _, err := s.ListConfigFolders("draft", path); err == nil {
			t.Fatalf("unsafe path %q listed", path)
		}
		if _, err := s.ConfirmConfig("draft", path, "S"); err == nil {
			t.Fatalf("unsafe path %q confirmed", path)
		}
	}
	if len(rpc.calls) != 0 {
		t.Fatalf("invalid paths reached RPC: %v", rpc.calls)
	}
	s.session.inPlace = true
	s.session.connection.Path = "Original"
	if _, err := s.ListConfigFolders("draft", ""); err == nil {
		t.Fatal("recovery destination changed through root navigation")
	}
	if _, err := s.ConfirmConfig("draft", "Another", "S"); err == nil || len(rpc.calls) != 0 {
		t.Fatal("recovery destination changed through confirmation")
	}
}
