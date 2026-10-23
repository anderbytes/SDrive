package core

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// Desktop OAuth identities are public application metadata, never user credentials.
//
//go:embed oauth.json
var oauthJSON []byte

type oauthIdentity struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret,omitempty"`
}

var projectOAuth = func() map[string]oauthIdentity {
	var identities map[string]oauthIdentity
	if err := json.Unmarshal(oauthJSON, &identities); err != nil {
		panic("invalid OAuth registrations")
	}
	return identities
}()

func oauthParameters(p Provider, params map[string]string) error {
	if !p.OAuth || (p.ID == "gcs" && params["service_account_file"] != "") {
		return nil
	}
	identity := projectOAuth[p.ID]
	if identity.ClientID == "" {
		return fmt.Errorf("sign-in unavailable; configure an OAuth application registration for %s", p.Name)
	}
	params["client_id"] = identity.ClientID
	if identity.ClientSecret != "" {
		params["client_secret"] = identity.ClientSecret
	}
	return nil
}
