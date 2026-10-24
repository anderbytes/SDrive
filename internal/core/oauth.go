package core

import (
	"strings"
)

func oauthParameters(p Provider, params map[string]string) error {
	if !p.OAuth || (p.ID == "gcs" && params["service_account_file"] != "") {
		return nil
	}
	for _, key := range []string{"client_id", "client_secret"} {
		if strings.TrimSpace(params[key]) == "" {
			return fieldMessage(key, "validation.required", map[string]string{"label": map[string]string{"client_id": "Client ID", "client_secret": "Client Secret"}[key]})
		}
	}
	return nil
}

func oauthParametersForConnection(c Connection, config map[string]string) error {
	p, err := provider(c.Provider)
	if err != nil {
		return err
	}
	return oauthParameters(p, config)
}
