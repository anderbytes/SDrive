package core

import (
	"net"
	"strconv"
	"strings"
)

// FieldError is serialized by Wails as the runtime error's cause.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *FieldError) Error() string { return e.Message }

func validateServer(host, port string) error {
	if host == "" || strings.ContainsAny(host, "/\\@ \t\r\n\x00") || (strings.ContainsAny(host, ":[]") && net.ParseIP(strings.Trim(host, "[]")) == nil) {
		return &FieldError{"host", "enter only the server hostname or IP"}
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return &FieldError{"port", "invalid port"}
		}
	}
	return nil
}
