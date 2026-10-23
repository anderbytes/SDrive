package core

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type Field struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Secret   bool     `json:"secret"`
	Required bool     `json:"required"`
	Default  string   `json:"default"`
	Choices  []string `json:"choices,omitempty"`
	Advanced bool     `json:"advanced"`
	Hint     string   `json:"hint,omitempty"`
	File     bool     `json:"file"`
}

type Provider struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Backend      string            `json:"backend"`
	Experimental bool              `json:"experimental"`
	Fields       []Field           `json:"fields"`
	Defaults     map[string]string `json:"-"`
	OAuth        bool              `json:"oauth"`
	Available    bool              `json:"available"`
	Notice       string            `json:"notice,omitempty"`
}

func field(key, label string, secret, required bool) Field {
	return Field{Key: key, Label: label, Secret: secret, Required: required}
}

func Catalog() []Provider {
	oauth := []Field{field("client_id", "Custom Client ID (optional)", false, false), field("client_secret", "Custom client secret (optional)", true, false)}
	webdav := []Field{field("url", "WebDAV HTTPS URL", false, true), field("user", "Username", false, false), field("pass", "Password / app password", true, false)}
	login := []Field{field("user", "Username", false, true), field("pass", "Password", true, true)}
	server := []Field{field("host", "Server", false, true), field("port", "Port (optional)", false, false), field("user", "Username", false, true), field("pass", "Password (optional with SSH key)", true, false)}
	providers := []Provider{
		{ID: "drive", Name: "Google Drive", Backend: "drive", Fields: oauth},
		{ID: "onedrive", Name: "OneDrive", Backend: "onedrive", Fields: oauth},
		{ID: "sharepoint", Name: "SharePoint Online", Backend: "onedrive", Fields: oauth},
		{ID: "dropbox", Name: "Dropbox", Backend: "dropbox", Fields: oauth},
		{ID: "box", Name: "Box", Backend: "box", Fields: oauth},
		{ID: "pcloud", Name: "pCloud", Backend: "pcloud", Fields: append(append([]Field{}, oauth...), Field{Key: "hostname", Label: "Region", Default: "api.pcloud.com", Choices: []string{"api.pcloud.com", "eapi.pcloud.com"}})},
		{ID: "mega", Name: "MEGA", Backend: "mega", Fields: append(append([]Field{}, login...), field("2fa", "Two-factor code (if enabled)", true, false))},
		{ID: "protondrive", Name: "Proton Drive", Backend: "protondrive", Experimental: true, Fields: []Field{field("username", "Username", false, true), field("password", "Password", true, true), field("2fa", "Two-factor code (if enabled)", true, false), field("mailbox_password", "Mailbox password (if separate)", true, false)}},
		{ID: "nextcloud", Name: "Nextcloud", Backend: "webdav", Fields: webdav, Defaults: map[string]string{"vendor": "nextcloud"}},
		{ID: "owncloud", Name: "ownCloud", Backend: "webdav", Fields: webdav, Defaults: map[string]string{"vendor": "owncloud"}},
		{ID: "webdav", Name: "WebDAV", Backend: "webdav", Fields: webdav, Defaults: map[string]string{"vendor": "other"}},
		{ID: "sftp", Name: "SFTP", Backend: "sftp", Fields: append(append([]Field{}, server...), field("key_file", "SSH private key path", false, false), field("key_file_pass", "Private key password", true, false))},
		{ID: "ftp", Name: "FTP / FTPS", Backend: "ftp", Fields: server},
		{ID: "smb", Name: "SMB / CIFS", Backend: "smb", Fields: append(append([]Field{}, server...), field("domain", "Domain (optional)", false, false))},
		{ID: "s3", Name: "S3 and compatible services", Backend: "s3", Fields: []Field{{Key: "provider", Label: "Provider", Default: "AWS", Choices: []string{"AWS", "Minio", "Cloudflare", "Wasabi", "Other"}}, field("access_key_id", "Access key ID", false, true), field("secret_access_key", "Secret access key", true, true), field("region", "Region", false, false), field("endpoint", "HTTPS endpoint (compatible services)", false, false)}},
		{ID: "b2", Name: "Backblaze B2", Backend: "b2", Fields: []Field{field("account", "Key ID", false, true), field("key", "Application key", true, true)}},
		{ID: "azureblob", Name: "Azure Blob Storage", Backend: "azureblob", Fields: []Field{field("account", "Account name", false, true), field("key", "Account key (or use SAS URL)", true, false), field("sas_url", "SAS URL (optional)", true, false)}},
		{ID: "gcs", Name: "Google Cloud Storage", Backend: "google cloud storage", Fields: append(append([]Field{}, oauth...), field("project_number", "Project number", false, false), field("service_account_file", "Service account JSON file (optional)", false, false))},
	}
	for i := range providers {
		p := &providers[i]
		p.Available = true
		for j := range p.Fields {
			f := &p.Fields[j]
			switch f.Key {
			case "client_id", "client_secret":
				p.OAuth = true
				f.Advanced = true
			case "port", "domain", "mailbox_password", "project_number":
				f.Advanced = true
			case "host":
				f.Hint = "Example: files.company.com"
			case "url":
				f.Hint = "Paste the WebDAV address provided by your service or administrator."
			case "endpoint":
				f.Hint = "Address provided by your service; not required for Amazon S3."
			case "key_file":
				f.File = true
				f.Hint = "Select your SSH private key, not the public .pub file. A key can replace the account password."
			case "key_file_pass":
				f.Advanced = true
				f.Hint = "Enter the passphrase protecting your SSH private key, if any."
			case "key", "sas_url":
				if p.ID == "azureblob" {
					f.Hint = "Use an account key or a SAS URL for the same storage account. A SAS URL can restrict access to a container."
				}
			case "service_account_file":
				f.File = true
				f.Hint = "Select the JSON credentials file for a service account authorized to access your bucket. This does not use browser sign-in."
			}
		}
		if p.OAuth && projectOAuth[p.ID].ClientID == "" {
			p.Available = false
			p.Notice = "Sign-in requires an OAuth application registration configured for this build."
		}
	}
	return providers
}

func provider(id string) (Provider, error) {
	for _, p := range Catalog() {
		if p.ID == id {
			return p, nil
		}
	}
	return Provider{}, fmt.Errorf("unknown connection type")
}

func parameters(p Provider, values map[string]string, insecure bool, transport string) (map[string]string, error) {
	result := map[string]string{}
	for k, v := range p.Defaults {
		result[k] = v
	}
	allowed := map[string]Field{}
	for _, f := range p.Fields {
		allowed[f.Key] = f
		if f.Default != "" {
			result[f.Key] = f.Default
		}
	}
	for k, v := range values {
		f, ok := allowed[k]
		if !ok {
			return nil, fmt.Errorf("field not allowed: %s", k)
		}
		if strings.ContainsAny(v, "\x00\r\n") {
			return nil, fmt.Errorf("invalid field: %s", f.Label)
		}
		if len(v) > 32768 {
			return nil, fmt.Errorf("field too long: %s", f.Label)
		}
		if v != "" {
			result[k] = v
		}
	}
	for _, f := range p.Fields {
		if f.Required && result[f.Key] == "" {
			return nil, fmt.Errorf("fill in %s", f.Label)
		}
		if len(f.Choices) > 0 && result[f.Key] != "" {
			valid := false
			for _, v := range f.Choices {
				if v == result[f.Key] {
					valid = true
				}
			}
			if !valid {
				return nil, fmt.Errorf("invalid option: %s", f.Label)
			}
		}
	}
	for _, key := range []string{"url", "endpoint", "sas_url"} {
		if value := result[key]; value != "" {
			u, err := url.Parse(value)
			if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && !(insecure && u.Scheme == "http")) {
				return nil, fmt.Errorf("%s must be an HTTPS URL without embedded credentials", key)
			}
		}
	}
	if v := result["host"]; v != "" {
		if strings.ContainsAny(v, "/\\@ \t") || (strings.Contains(v, ":") && net.ParseIP(strings.Trim(v, "[]")) == nil) {
			return nil, fmt.Errorf("enter only the server hostname or IP")
		}
		result["host"] = strings.Trim(v, "[]")
	}
	if v := result["port"]; v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("invalid port")
		}
	}
	if p.ID == "azureblob" && result["key"] == "" && result["sas_url"] == "" {
		return nil, fmt.Errorf("enter the account key or SAS URL")
	}
	if p.ID == "ftp" {
		result["tls"], result["explicit_tls"] = "false", "false"
		switch transport {
		case "explicit":
			result["explicit_tls"] = "true"
		case "implicit":
			result["tls"] = "true"
			if result["port"] == "" {
				result["port"] = "990"
			}
		case "plain":
			if !insecure {
				return nil, fmt.Errorf("FTP without TLS requires authorization for this connection")
			}
		default:
			return nil, fmt.Errorf("select the FTP/FTPS mode")
		}
	}
	return result, nil
}

func ftpTransport(config map[string]string) (string, error) {
	flags := map[string]bool{}
	for _, key := range []string{"tls", "explicit_tls"} {
		if config[key] == "" {
			continue
		}
		value, err := strconv.ParseBool(config[key])
		if err != nil {
			return "", fmt.Errorf("invalid saved FTP transport; preserve the connection and try again")
		}
		flags[key] = value
	}
	if flags["tls"] && flags["explicit_tls"] {
		return "", fmt.Errorf("invalid saved FTP transport; preserve the connection and try again")
	}
	if flags["tls"] {
		return "implicit", nil
	}
	if flags["explicit_tls"] {
		return "explicit", nil
	}
	return "plain", nil
}
