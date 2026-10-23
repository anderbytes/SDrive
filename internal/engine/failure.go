package engine

import "strings"

// Only the category crosses the engine boundary; provider text can contain secrets.
type operationFailure string

func (e operationFailure) FailureKind() string { return string(e) }
func (e operationFailure) Error() string {
	switch e {
	case "network":
		return "could not reach the storage service"
	case "credentials":
		return "the storage service rejected the credentials"
	case "permission":
		return "the storage service denied the required permission"
	case "engine":
		return "could not communicate with the local engine"
	case "invalid":
		return "invalid configuration result"
	default:
		return "could not configure the account; check credentials and permissions"
	}
}

func providerFailure(raw string) error {
	text := strings.ToLower(raw)
	for _, group := range []struct {
		kind    string
		signals []string
	}{
		{"permission", []string{"insufficient_scope", "insufficientpermissions", "permission denied", "access denied", "accessdenied"}},
		{"credentials", []string{"invalid_grant", "invalid credentials", "authentication failed", "invalid password", "login incorrect"}},
		{"network", []string{"no such host", "connection refused", "connection reset", "network is unreachable", "i/o timeout", "context deadline exceeded", "tls handshake timeout"}},
	} {
		for _, signal := range group.signals {
			if strings.Contains(text, signal) {
				return operationFailure(group.kind)
			}
		}
	}
	return operationFailure("unknown")
}

func (e *rpcRejection) FailureKind() string {
	// These statuses belong to the local RPC endpoint, not the storage account.
	if e.status == 401 || e.status == 403 {
		return "engine"
	}
	return "unknown"
}
