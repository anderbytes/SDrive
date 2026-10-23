package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Prerequisite struct {
	File      string `json:"file"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	Publisher string `json:"publisher"`
}

func PrerequisiteFromManifest(data []byte, id string) (Prerequisite, error) {
	file := map[string]string{"winfsp": "winfsp.msi", "webview2": "webview2.exe"}[id]
	if file == "" {
		return Prerequisite{}, fmt.Errorf("unknown component")
	}
	var items []Prerequisite
	if json.Unmarshal(data, &items) != nil {
		return Prerequisite{}, fmt.Errorf("invalid component manifest")
	}
	var found *Prerequisite
	for _, p := range items {
		if p.File != file {
			continue
		}
		u, err := url.Parse(p.URL)
		hash, hashErr := hex.DecodeString(p.SHA256)
		if found != nil || err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || hashErr != nil || len(hash) != 32 || !strings.HasPrefix(p.Publisher, "CN=") || !strings.HasSuffix(p.Publisher, ",") {
			return Prerequisite{}, fmt.Errorf("invalid component manifest")
		}
		found = &p
	}
	if found == nil {
		return Prerequisite{}, fmt.Errorf("component missing from manifest")
	}
	return *found, nil
}

func secureRedirect(req *http.Request, via []*http.Request) error {
	if req.URL.Scheme != "https" || req.URL.User != nil || len(via) >= 10 {
		return fmt.Errorf("unsafe or excessive redirects")
	}
	return nil
}

func DownloadPrerequisite(ctx context.Context, p Prerequisite, dir string, progress io.Writer) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("could not prepare the temporary folder")
	}
	target := filepath.Join(dir, p.File)
	if file, err := verifiedPrerequisite(p, target); err == nil {
		file.Close()
		fmt.Fprintln(progress, "Component already verified; no download needed.")
		return nil
	}
	client := &http.Client{Timeout: 30 * time.Minute, CheckRedirect: secureRedirect}
	part, err := downloadFile(ctx, client, p, dir, progress)
	if err != nil {
		return err
	}
	defer os.Remove(part)
	fmt.Fprintln(progress, "Checking signature and publisher…")
	file, err := verifiedPrerequisite(p, part)
	if err != nil {
		return err
	}
	file.Close()
	if err = os.Rename(part, target); err != nil {
		return fmt.Errorf("could not complete the download")
	}
	fmt.Fprintln(progress, "Download completed and verified.")
	return nil
}

func downloadFile(ctx context.Context, client *http.Client, p Prerequisite, dir string, progress io.Writer) (result string, err error) {
	const maxSize = 512 << 20
	u, parseErr := url.Parse(p.URL)
	if parseErr != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", fmt.Errorf("invalid download address")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Abort stalled transfers, while permitting slow connections that keep receiving data.
	idle := time.AfterFunc(60*time.Second, cancel)
	defer idle.Stop()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		return "", fmt.Errorf("could not prepare the download")
	}
	fmt.Fprintf(progress, "Downloading %s…\n", p.File)
	response, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not download the component; check your internet connection and try again")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength > maxSize {
		return "", fmt.Errorf("the server did not provide a valid component")
	}
	file, err := os.CreateTemp(dir, p.File+"-*.download")
	if err != nil {
		return "", fmt.Errorf("could not write the download")
	}
	defer func() {
		file.Close()
		if err != nil {
			os.Remove(file.Name())
		}
	}()
	hash := sha256.New()
	writer := io.MultiWriter(file, hash)
	buffer := make([]byte, 128<<10)
	var received int64
	last := time.Time{}
	for {
		n, readErr := response.Body.Read(buffer)
		if n > 0 {
			idle.Reset(60 * time.Second)
			received += int64(n)
			if received > maxSize {
				return "", fmt.Errorf("download exceeded the size limit")
			}
			if _, err = writer.Write(buffer[:n]); err != nil {
				return "", fmt.Errorf("could not write the download; check disk space")
			}
			if time.Since(last) >= time.Second {
				if response.ContentLength > 0 {
					fmt.Fprintf(progress, "%s: %d%% (%d of %d MiB)\n", p.File, received*100/response.ContentLength, received>>20, response.ContentLength>>20)
				} else {
					fmt.Fprintf(progress, "%s: %d MiB received\n", p.File, received>>20)
				}
				last = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", fmt.Errorf("download interrupted; check your internet connection and try again")
		}
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), p.SHA256) {
		return "", fmt.Errorf("downloaded file failed integrity verification; try again")
	}
	if err = file.Close(); err != nil {
		return "", fmt.Errorf("could not finalize the downloaded file")
	}
	return file.Name(), nil
}
