package core

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestGuidedDraftIsOnlySavedAfterDestinationValidation(t *testing.T) {
	rpc := &fakeRPC{handle: func(_ context.Context, method string, input any) (any, error) {
		if method == "operations/list" {
			data, _ := json.Marshal(input)
			if strings.Contains(string(data), "denied") {
				return nil, fmt.Errorf("access denied with token do-not-leak")
			}
			return map[string]any{"list": []any{map[string]any{"Name": "Documents", "Path": "Documents", "IsDir": true}, map[string]any{"Name": "Bad", "Path": "../outside", "IsDir": true}, map[string]any{"Name": "Nested", "Path": "a/b", "IsDir": true}}}, nil
		}
		return ConfigResult{}, nil
	}}
	s := testService(t, rpc)
	id, err := s.BeginConfig(ConfigInput{Connection: Connection{Name: "Arquivos", Provider: "webdav", Letter: "S"}, Values: map[string]string{"url": "https://example.test/dav"}, Guided: true})
	if err != nil {
		t.Fatal(err)
	}
	ss := waitSession(t, s, id)
	if !ss.Authenticated || ss.Complete || len(s.store.Connections) != 0 {
		t.Fatal("draft persisted before choosing destination")
	}
	folders, err := s.ListConfigFolders(id, "")
	if err != nil || len(folders) != 1 || folders[0].Path != "Documents" {
		t.Fatalf("unsafe folder list: %v %v", folders, err)
	}
	if _, err = s.ListConfigFolders(id, "../outside"); err == nil {
		t.Fatal("path traversal accepted")
	}
	if _, err = s.ConfirmConfig(id, "denied"); err == nil || strings.Contains(err.Error(), "do-not-leak") {
		t.Fatal("validation/error redaction failed")
	}
	if len(s.store.Connections) != 0 {
		t.Fatal("invalid destination saved")
	}
	connectionID, err := s.ConfirmConfig(id, "Documents")
	if err != nil || connectionID == "" || len(s.store.Connections) != 1 || s.store.Connections[0].Path != "Documents" {
		t.Fatalf("confirm failed: %v", err)
	}
	if _, err = s.ConfirmConfig(id, "another"); err != nil || s.store.Connections[0].Path != "Documents" {
		t.Fatal("retry changed destination")
	}
	if err = s.EndConfig(id); err != nil {
		t.Fatal(err)
	}
	if len(s.store.Connections) != 1 {
		t.Fatal("completed connection removed")
	}
}

func TestCancelAuthenticatedDraftPreservesOldRemote(t *testing.T) {
	s := testService(t, &fakeRPC{})
	c := testConnection()
	c.Provider = "webdav"
	c.Recovery = false
	s.store.Connections = []Connection{c}
	id, err := s.BeginConfig(ConfigInput{Connection: c, Values: map[string]string{"url": "https://example.test/dav"}, Guided: true})
	if err != nil {
		t.Fatal(err)
	}
	if !waitSession(t, s, id).Authenticated {
		t.Fatal("not authenticated")
	}
	if err = s.EndConfig(id); err != nil {
		t.Fatal(err)
	}
	if len(s.store.Connections) != 1 || s.store.Connections[0].Remote != c.Remote {
		t.Fatal("cancel replaced original remote")
	}
}

func TestProjectOAuthAvailabilityAndInjection(t *testing.T) {
	p, _ := provider("drive")
	if p.Available || !p.OAuth {
		t.Fatal("unconfigured login advertised")
	}
	if oauthParameters(p, map[string]string{}) == nil {
		t.Fatal("shared OAuth fallback accepted")
	}
	previous := projectOAuth
	projectOAuth = map[string]oauthIdentity{"drive": {ClientID: "desktop-client", ClientSecret: "public-desktop-metadata"}}
	t.Cleanup(func() { projectOAuth = previous })
	p, _ = provider("drive")
	params := map[string]string{}
	if !p.Available || oauthParameters(p, params) != nil || params["client_id"] != "desktop-client" {
		t.Fatal("project OAuth not applied")
	}
	gcs, _ := provider("gcs")
	if oauthParameters(gcs, map[string]string{"service_account_file": "credentials.json"}) != nil {
		t.Fatal("service account incorrectly requires OAuth")
	}
}

func TestPickerAndUnknownQuestionsStayRestricted(t *testing.T) {
	s := testService(t, &fakeRPC{})
	if _, err := s.SelectCredentialFile("command"); err == nil {
		t.Fatal("unsupported picker exposed")
	}
	q := Question{Name: "unknown", Help: "raw English prompt", DefaultStr: "secret"}
	localizeQuestion(&q)
	if !q.Unsupported || strings.Contains(q.Help, "raw English") {
		t.Fatal("unknown provider prompt exposed")
	}
	q = Question{Name: "config_type", Examples: []Example{{Value: "url", Help: "Sharepoint site name or URL"}}}
	localizeQuestion(&q)
	if q.Unsupported || q.Name != "Account type" || q.Examples[0].Help != "Enter SharePoint address" {
		t.Fatal("known question not localized")
	}
}

func TestMountAllowsConfirmedSessionAndBlocksUnfinishedAuthentication(t *testing.T) {
	s := testService(t, &fakeRPC{})
	c := testConnection()
	c.Recovery = false
	s.store.Connections = []Connection{c}
	s.session = &ConfigSession{connection: c}
	if err := s.Mount(c.ID); err == nil || !strings.Contains(err.Error(), "complete or cancel") {
		t.Fatal("unfinished authentication did not block mounting")
	}
	s.session.Complete = true
	if err := s.Mount(c.ID); err != nil && strings.Contains(err.Error(), "complete or cancel") {
		t.Fatal("confirmed wizard still blocks mounting")
	}
}

func TestFolderPickerAcceptsExistingPathWithTrailingSlash(t *testing.T) {
	rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
		if method == "operations/list" {
			return map[string]any{"list": []any{map[string]any{"Name": "Fotos", "Path": "Documents/Fotos", "IsDir": true}}}, nil
		}
		return map[string]any{}, nil
	}}
	s := testService(t, rpc)
	c := testConnection()
	folders, err := listFolders(context.Background(), s.rpc, c, "Documents/")
	if err != nil || len(folders) != 1 || folders[0].Path != "Documents/Fotos" {
		t.Fatalf("trailing slash hid folders: %v %v", folders, err)
	}
}

func TestReconfigurationRetriesOldCredentialCleanupWithoutDeletingNewRemote(t *testing.T) {
	var deleted []string
	fail := true
	rpc := &fakeRPC{handle: func(_ context.Context, method string, input any) (any, error) {
		if method == "config/delete" {
			deleted = append(deleted, input.(map[string]string)["name"])
			if fail {
				return nil, context.DeadlineExceeded
			}
		}
		return ConfigResult{}, nil
	}}
	s := testService(t, rpc)
	c := testConnection()
	c.Recovery = false
	s.store.Connections = []Connection{c}
	id, err := s.BeginConfig(ConfigInput{Connection: c, Values: map[string]string{"host": "example.test", "user": "user"}, Transport: "explicit"})
	if err != nil {
		t.Fatal(err)
	}
	ss := waitSession(t, s, id)
	if !ss.Complete || ss.Error == "" || s.store.Connections[0].Remote == c.Remote {
		t.Fatal("committed reconfiguration not preserved after cleanup failure")
	}
	want := s.store.Connections[0]
	persisted, err := loadStore(s.dir)
	if err != nil || !reflect.DeepEqual(persisted.Connections, []Connection{want}) {
		t.Fatalf("replacement not persisted: %v", err)
	}
	if s.EndConfig(id) == nil || s.session == nil {
		t.Fatal("failed cleanup cannot be retried")
	}
	fail = false
	if err := s.EndConfig(id); err != nil || s.session != nil {
		t.Fatalf("cleanup retry failed: %v", err)
	}
	if len(deleted) != 3 {
		t.Fatalf("expected three cleanup attempts, got %v", deleted)
	}
	for _, name := range deleted {
		if name != "sd_"+c.Remote {
			t.Fatalf("cleanup deleted the replacement: %s", name)
		}
	}
}
