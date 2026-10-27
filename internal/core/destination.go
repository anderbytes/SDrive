package core

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// This public upstream key decodes rclone's obfuscation, not its encrypted file.
func revealPassword(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(raw) < aes.BlockSize {
		return "", fmt.Errorf("Could not read saved WebDAV credentials.")
	}
	key := []byte{0x9c, 0x93, 0x5b, 0x48, 0x73, 0x0a, 0x55, 0x4d, 0x6b, 0xfd, 0x7c, 0x63, 0xc8, 0x86, 0xa9, 0x2b, 0xd3, 0x90, 0x19, 0x8e, 0xb8, 0x12, 0x8a, 0xfb, 0xf4, 0xde, 0x16, 0x2b, 0x8b, 0x95, 0xf6, 0x38}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	buf := raw[aes.BlockSize:]
	cipher.NewCTR(block, raw[:aes.BlockSize]).XORKeyStream(buf, buf)
	return string(buf), nil
}

func resourceDigest(id string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(id))) }

func probeResource(ctx context.Context, client *http.Client, address, path, user, password string) (string, bool, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", false, fmt.Errorf("Destination verification requires an HTTPS WebDAV address.")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + path
	u.RawPath = ""
	body := `<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:resource-id/></d:prop></d:propfind>`
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", u.String(), strings.NewReader(body))
	if err != nil {
		return "", false, fmt.Errorf("Could not verify the WebDAV destination.")
	}
	req.Header.Set("Depth", "0")
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	if user != "" || password != "" {
		req.SetBasicAuth(user, password)
	}
	response, err := client.Do(req)
	if err != nil {
		var operation *net.OpError
		var networkError net.Error
		unreachable := errors.As(err, &operation) && operation.Op == "dial"
		if errors.As(err, &networkError) && networkError.Timeout() {
			unreachable = true
		}
		return "", unreachable, fmt.Errorf("Could not reach or securely verify the WebDAV destination.")
	}
	defer response.Body.Close()
	if response.StatusCode != 207 {
		return "", false, fmt.Errorf("The WebDAV server did not provide a verifiable resource identity.")
	}
	var document struct {
		Responses []struct {
			Href  string `xml:"DAV: href"`
			Props []struct {
				Status string `xml:"DAV: status"`
				Prop   struct {
					ID struct {
						Href string `xml:"DAV: href"`
					} `xml:"DAV: resource-id"`
				} `xml:"DAV: prop"`
			} `xml:"DAV: propstat"`
		} `xml:"DAV: response"`
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
	if err != nil || len(data) > 64<<10 || xml.Unmarshal(data, &document) != nil || len(document.Responses) != 1 {
		return "", false, fmt.Errorf("The WebDAV server returned an invalid resource identity.")
	}
	href, parseErr := url.Parse(document.Responses[0].Href)
	if parseErr != nil || href == nil || strings.TrimRight(href.Path, "/") != strings.TrimRight(u.Path, "/") || (href.IsAbs() && (href.Host != u.Host || href.Scheme != u.Scheme)) {
		return "", false, fmt.Errorf("The WebDAV server returned an invalid resource identity.")
	}
	found := ""
	for _, props := range document.Responses[0].Props {
		if props.Status == "HTTP/1.1 200 OK" || props.Status == "HTTP/2 200 OK" {
			id := strings.TrimSpace(props.Prop.ID.Href)
			uri, e := url.Parse(id)
			if e == nil && uri.IsAbs() && len(id) <= 2048 {
				if found != "" && found != id {
					return "", false, fmt.Errorf("The WebDAV server returned an invalid resource identity.")
				}
				found = id
			}
		}
	}
	if found == "" {
		return "", false, fmt.Errorf("The WebDAV server did not provide a verifiable resource identity.")
	}
	return resourceDigest(found), false, nil
}

func (s *Service) destinationClient(c Connection) (*http.Client, error) {
	transport := &http.Transport{Proxy: nil, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second}
	s.mu.Lock()
	session := s.networkSessions[c.ID]
	s.mu.Unlock()
	if session != nil && session.remote == c.Remote {
		dial := http.DefaultTransport.(*http.Transport).DialContext
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			conn, err := dial(ctx, network, address)
			if err != nil {
				return nil, err
			}
			return &networkProbeConn{Conn: conn, s: s, id: c.ID, session: session}, nil
		}
	}
	if c.UseProxy {
		s.mu.Lock()
		p, err := s.readProxy()
		s.mu.Unlock()
		if err != nil {
			return nil, err
		}
		u, err := url.Parse(p.URL())
		if err != nil || u.Host == "" {
			return nil, fmt.Errorf("Configure a proxy before using it for this connection.")
		}
		transport.Proxy = http.ProxyURL(u)
	}
	return &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

// Verification never follows redirects or writes a marker into user storage.
func (s *Service) verifyDestination(ctx context.Context, c *Connection, config map[string]string, setup bool) (string, error) {
	if !c.SessionOnly && c.AlternateURL == "" {
		return config["url"], nil
	}
	if !webDAV(c.Provider) {
		if c.Provider == "drive" && (config["root_folder_id"] != "" && config["root_folder_id"] != "root" && config["root_folder_id"] != "appDataFolder" || config["team_drive"] != "") {
			return "", nil
		}
		if (c.Provider == "onedrive" || c.Provider == "sharepoint") && config["drive_id"] != "" {
			return "", nil
		}
		return "", &FieldError{Field: "sessionOnly", Message: "Session-only sign-in requires a verifiable destination for this service."}
	}
	client, err := s.destinationClient(*c)
	if err != nil {
		return "", err
	}
	defer client.CloseIdleConnections()
	return verifyWebDAV(ctx, client, c, config, setup)
}

func verifyWebDAV(ctx context.Context, client *http.Client, c *Connection, config map[string]string, setup bool) (string, error) {
	password, err := revealPassword(config["pass"])
	if err != nil {
		return "", err
	}
	primary := config["url"]
	if c.PrimaryURL != "" {
		primary = c.PrimaryURL
	}
	primaryID, unreachable, primaryErr := probeResource(ctx, client, primary, c.Path, config["user"], password)
	if primaryErr != nil && (!unreachable || setup || c.AlternateURL == "") {
		return "", primaryErr
	}
	expected := c.ResourceID
	if primaryErr == nil {
		if !setup && expected != "" && primaryID != expected {
			return "", fmt.Errorf("Destination identity changed. Preserve pending files and use the original storage.")
		}
		expected = primaryID
	}
	if c.AlternateURL != "" && (setup || primaryErr != nil) {
		alternateID, _, err := probeResource(ctx, client, c.AlternateURL, c.Path, config["user"], password)
		if err != nil {
			return "", err
		}
		if expected == "" || alternateID != expected {
			return "", &FieldError{Field: "alternateURL", Message: "Both addresses must identify the same WebDAV destination."}
		}
	}
	if c.Recovery && c.ResourceID != "" && expected != c.ResourceID {
		return "", fmt.Errorf("Destination identity changed. Preserve pending files and use the original storage.")
	}
	c.ResourceID = expected
	c.PrimaryURL = primary
	if primaryErr != nil {
		return c.AlternateURL, nil
	}
	return primary, nil
}
