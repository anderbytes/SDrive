package core

import (
	"net"
	"strconv"
	"strings"

	"sdrive/internal/i18n"
)

// FieldError is serialized by Wails as the runtime error's cause.
type FieldError struct {
	Field   string            `json:"field"`
	Message string            `json:"message"`
	Code    string            `json:"code,omitempty"`
	Params  map[string]string `json:"params,omitempty"`
}

func (e *FieldError) Error() string { return e.Message }

func (e *FieldError) LocalizedMessage() *i18n.Message {
	if e.Code == "" {
		return nil
	}
	return &i18n.Message{Code: e.Code, Params: e.Params, Message: e.Message}
}

func fieldMessage(field, code string, params map[string]string) *FieldError {
	message := i18n.NewMessage(code, params)
	return &FieldError{Field: field, Message: message.Message, Code: code, Params: params}
}

func validateServer(host, port string) error {
	if host == "" || strings.ContainsAny(host, "/\\@ \t\r\n\x00") || (strings.ContainsAny(host, ":[]") && net.ParseIP(strings.Trim(host, "[]")) == nil) {
		return &FieldError{Field: "host", Message: "enter only the server hostname or IP"}
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return &FieldError{Field: "port", Message: "invalid port"}
		}
	}
	return nil
}
