//go:build windows

package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadIntegrityFailureCleanupAndRetry(t *testing.T) {
	payload := []byte("signed component placeholder")
	hash := sha256.Sum256(payload)
	var broken atomic.Bool
	broken.Store(true)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if broken.Load() {
			_, _ = w.Write([]byte("corrupted"))
			return
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	client := server.Client()
	client.CheckRedirect = secureRedirect
	p := Prerequisite{File: "winfsp.msi", URL: server.URL, SHA256: hex.EncodeToString(hash[:])}
	dir := t.TempDir()
	if _, err := downloadFile(context.Background(), client, p, dir, io.Discard); err == nil {
		t.Fatal("corrupted download accepted")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatal("failed transfer left partial files")
	}
	broken.Store(false)
	var progress bytes.Buffer
	path, err := downloadFile(context.Background(), client, p, dir, &progress)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	data, _ := os.ReadFile(path)
	if !bytes.Equal(data, payload) || !strings.Contains(progress.String(), "Downloading") {
		t.Fatal("retry did not receive expected payload/progress")
	}
	if _, err = verifiedPrerequisite(p, path); err == nil {
		t.Fatal("unsigned file accepted despite matching checksum")
	}
}

func TestDownloadRejectsCancellationHTTPAndDowngrades(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			<-r.Context().Done()
			return
		}
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "http://127.0.0.1/no-tls", http.StatusFound)
			return
		}
		if r.URL.Path == "/large" {
			w.Header().Set("Content-Length", "536870913")
			return
		}
		if r.URL.Path == "/broken" {
			w.Header().Set("Content-Length", "100")
			_, _ = w.Write([]byte("truncated"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client := server.Client()
	client.CheckRedirect = secureRedirect
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, item := range []struct {
		ctx context.Context
		url string
	}{
		{ctx, server.URL}, {context.Background(), "http://127.0.0.1/no-tls"},
		{context.Background(), server.URL}, {context.Background(), server.URL + "/redirect"},
		{context.Background(), server.URL + "/large"}, {context.Background(), server.URL + "/broken"},
	} {
		dir := t.TempDir()
		if _, err := downloadFile(item.ctx, client, Prerequisite{File: "webview2.exe", URL: item.url}, dir, io.Discard); err == nil {
			t.Fatalf("invalid transfer accepted: %s", item.url)
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) != 0 {
			t.Fatal("partial download was retained")
		}
	}
	client.Timeout = 20 * time.Millisecond
	if _, err := downloadFile(context.Background(), client, Prerequisite{File: "webview2.exe", URL: server.URL + "/slow"}, t.TempDir(), io.Discard); err == nil {
		t.Fatal("timed-out transfer accepted")
	}
}

func TestOfficialPrerequisiteSignatures(t *testing.T) {
	data, err := os.ReadFile("../../installer/prerequisites.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"winfsp", "webview2"} {
		t.Run(id, func(t *testing.T) {
			p, err := PrerequisiteFromManifest(data, id)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("../../.tools/prerequisites", p.File)
			if _, err = os.Stat(path); os.IsNotExist(err) {
				t.Skip("payload official not downloaded; packaging runs this check after download")
			}
			file, err := verifiedPrerequisite(p, path)
			if err != nil {
				t.Fatal(err)
			}
			file.Close()
			if id == "winfsp" {
				// Downloaded partials have a temporary extension; MSI signature detection must still work.
				copyPath := filepath.Join(t.TempDir(), "winfsp.download")
				blob, _ := os.ReadFile(path)
				if err = os.WriteFile(copyPath, blob, 0600); err != nil {
					t.Fatal(err)
				}
				file, err = verifiedPrerequisite(p, copyPath)
				if err != nil {
					t.Fatal(err)
				}
				file.Close()
			}
			p.Publisher = "CN=Unexpected Publisher,"
			if file, err = verifiedPrerequisite(p, path); err == nil {
				file.Close()
				t.Fatal("wrong publisher accepted")
			}
		})
	}
}

func TestPrerequisiteManifestRestrictsComponentsAndOrigins(t *testing.T) {
	data, err := os.ReadFile("../../installer/prerequisites.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = PrerequisiteFromManifest(data, "command"); err == nil {
		t.Fatal("unknown component accepted")
	}
	if _, err = PrerequisiteFromManifest([]byte(strings.ReplaceAll(string(data), "https://", "http://")), "winfsp"); err == nil {
		t.Fatal("insecure origin accepted")
	}
	if _, err = PrerequisiteFromManifest([]byte("null"), "winfsp"); err == nil {
		t.Fatal("missing manifest entry accepted")
	}
}
