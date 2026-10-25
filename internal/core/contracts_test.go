package core

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"sdrive/internal/i18n"
)

// Samples are synthetic; no service operation, profile or credential is used.
func bridgeSamples() map[string]any {
	connection := Connection{ID: "connection", Remote: "remote", Name: "Café 😀", Provider: "sftp", Path: "folder", Letter: "S", AuthMethod: "oauth", SessionOnly: true, UploadLimit: 1024, DownloadLimit: 2048, UseProxy: true, Browser: "edge", PrivateBrowser: true, ConnectTimeout: 30, IdleTimeout: 60, AlternateURL: "https://alternate.example.test/dav", ResourceID: "synthetic-resource", PrimaryURL: "https://example.test/dav", Binding: "{}", MountMode: "local", MountFolder: `C:\Synthetic\Mount`, DirectoryCacheSeconds: 300}
	age, delay := 24, 5
	settings := Settings{Language: "en", CacheGiB: 2, CacheMaxAgeHours: &age, WriteBackSeconds: &delay}
	field := Field{Key: "host", Label: "Server", Required: true, Choices: []string{"example.test"}, Hint: "Server hostname"}
	provider := Provider{ID: "sftp", Name: "SFTP", Backend: "sftp", Fields: []Field{field}, Available: true, Notice: "Synthetic provider"}
	question := Question{Name: "Library", Help: "Choose a library", Examples: []Example{{Value: "library", Help: "User library"}}}
	input := ConfigInput{Connection: connection, Values: map[string]string{"host": "example.test"}, Guided: true, ForgetCredentials: true, ImportDraftID: "draft"}
	message := i18n.NewMessage("drive.letter_in_use", map[string]string{"letter": "S"})
	count, size, pending := 2, uint64(1024), int64(512)
	checked := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	empty := View{Providers: []Provider{}, Connections: []ConnectionView{}, ImportDrafts: []ImportDraft{}, Settings: settings, Letters: []string{}}
	filled := empty
	filled.Providers = []Provider{provider}
	filled.Letters = []string{"T"}
	filled.FreeBytes = &size
	filled.Connections = []ConnectionView{{Connection: connection, Pending: &count, Operation: "mounting", Error: message.Message, ErrorMessage: message, Uploads: &UploadView{Active: 1, Queued: 1}, Cache: CacheView{CheckedAt: &checked, PendingBytes: &pending, UsedBytes: &size}}}
	return map[string]any{
		"importPreview": ImportPreview{ID: "preview", Document: PortableFile{Format: "sdrive-portable", Version: 1, Connections: []PortableConnection{portableDefinition(connection)}, Preferences: portablePreferences(settings)}},
		"importDraft":   ImportDraft{ID: "draft", Definition: portableDefinition(connection)},
		"mountTarget":   mountTarget(connection),
		"snapshotEmpty": empty, "snapshotFilled": filled,
		"connectionUnknown": ConnectionView{Connection: Connection{ID: "connection", Remote: "remote", Name: "Café 😀", Provider: "sftp", Path: "folder", Letter: "S"}},
		"fieldEmpty":        Field{}, "providerEmpty": Provider{},
		"sessionEmpty":  ConfigSession{},
		"sessionFilled": ConfigSession{ID: "session", Authenticated: true, Question: &question},
		"questionEmpty": Question{}, "configInput": input,
		"about": AboutInfo{AppVersion: "test"}, "folders": []Folder{{Name: "Folder", Path: "folder"}},
		"foldersEmpty": []Folder{}, "fields": ConnectionFieldsView{Values: map[string]string{}, Transport: "explicit"},
		"identity":          HostIdentity{Key: "synthetic-key", Fingerprint: "synthetic-fingerprint"},
		"fieldError":        &FieldError{Field: "port", Message: "invalid port"},
		"dynamicFieldError": fieldMessage("host", "validation.required", map[string]string{"label": "Server"}),
		"message":           message,
	}
}

func bridgeMethods() map[string]any {
	names := map[string]string{
		"exportConfiguration": "ExportConfiguration", "previewImport": "PreviewImport", "commitImport": "CommitImport", "removeImportDraft": "RemoveImportDraft",
		"selectDirectory": "SelectDirectory", "changeCacheLocation": "ChangeCacheLocation", "refreshDirectoryCache": "RefreshDirectoryCache",
		"browsers": "Browsers", "proxy": "ProxySettings", "saveProxy": "SaveProxy",
		"about": "About", "diagnostics": "Diagnostics", "desktopVisible": "DesktopVisible", "snapshot": "Snapshot",
		"begin": "BeginConfig", "status": "ConfigStatus", "answer": "AnswerConfig", "end": "EndConfig",
		"folders": "ListConfigFolders", "confirm": "ConfirmConfig", "selectFile": "SelectCredentialFile",
		"mount": "Mount", "unmount": "Unmount", "test": "TestConnection", "remove": "Remove",
		"update": "UpdateConnection", "fields": "ConnectionFields", "settings": "SaveSettings", "open": "OpenExplorer",
		"prerequisite": "Prerequisite", "probe": "ProbeSSH", "exit": "Exit", "restart": "RestartEngine",
		"restoreSettings": "RestoreSettings", "verifyRecovery": "VerifyRecovery", "retryCleanup": "RetryCleanup",
	}
	result := map[string]any{}
	service := reflect.TypeOf((*Service)(nil))
	for key, name := range names {
		method, ok := service.MethodByName(name)
		if !ok {
			panic("missing bridge method: " + name)
		}
		args, returns := []string{}, []string{}
		for i := 1; i < method.Type.NumIn(); i++ {
			args = append(args, method.Type.In(i).String())
		}
		for i := 0; i < method.Type.NumOut(); i++ {
			returns = append(returns, method.Type.Out(i).String())
		}
		result[key] = map[string]any{"name": name, "args": args, "returns": returns}
	}
	return result
}

func TestBridgeContracts(t *testing.T) {
	want, err := os.ReadFile("../../frontend/src/bridge-fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	actual, err := json.Marshal(map[string]any{"samples": bridgeSamples(), "methods": bridgeMethods()})
	if err != nil {
		t.Fatal(err)
	}
	var left, right any
	if json.Unmarshal(want, &left) != nil || json.Unmarshal(actual, &right) != nil || !reflect.DeepEqual(left, right) {
		t.Fatalf("bridge contract changed; review Go serialization, TypeScript and shared fixtures together\nactual: %s", actual)
	}
}
