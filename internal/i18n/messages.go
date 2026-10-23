package i18n

import (
	_ "embed"
	"encoding/json"
	"errors"
	"regexp"
)

// Message keeps the English error usable by existing bridge and desktop callers.
type Message struct {
	Code    string            `json:"code"`
	Params  map[string]string `json:"params"`
	Message string            `json:"message"`
}

//go:embed messages.json
var messageJSON []byte

var messages = func() map[string]map[string]string {
	var catalog map[string]map[string]string
	if err := json.Unmarshal(messageJSON, &catalog); err != nil {
		panic(err)
	}
	return catalog
}()

var namedToken = regexp.MustCompile(`\{(\w+)\}`)

func (m *Message) Error() string              { return m.Message }
func (m *Message) LocalizedMessage() *Message { return m }

func NewMessage(code string, params map[string]string) *Message {
	m := &Message{Code: code, Params: params}
	m.Message = Format("en", m)
	return m
}

func MessageOf(err error) *Message {
	var localized interface{ LocalizedMessage() *Message }
	if errors.As(err, &localized) {
		return localized.LocalizedMessage()
	}
	return nil
}

func Error(language string, err error) string {
	if message := MessageOf(err); message != nil {
		return Format(language, message)
	}
	return Text(language, err.Error())
}

// Format substitutes once, so user values are never interpreted as templates.
func Format(language string, message *Message) string {
	entry, ok := messages[message.Code]
	if !ok {
		return message.Message
	}
	template := entry[language]
	if template == "" {
		template = entry["en"]
	}
	for _, token := range namedToken.FindAllStringSubmatch(template, -1) {
		if _, ok := message.Params[token[1]]; !ok {
			return message.Message
		}
	}
	return namedToken.ReplaceAllStringFunc(template, func(token string) string {
		return message.Params[token[1:len(token)-1]]
	})
}
