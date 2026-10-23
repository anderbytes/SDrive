package i18n

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
)

func TestStructuredMessages(t *testing.T) {
	message := NewMessage("recovery.required_before_exit", map[string]string{"name": "Café {letter} %s 😀"})
	if got := Error("pt-BR", fmt.Errorf("wrapped: %w", message)); got != "recupere a conexão Café {letter} %s 😀 antes de sair com segurança" {
		t.Fatal(got)
	}
	for _, test := range []struct {
		language string
		message  Message
		want     string
	}{
		{"en", *message, message.Message},
		{"unknown", *message, message.Message},
		{"pt-BR", Message{Code: "unknown", Message: "original"}, "original"},
		{"pt-BR", Message{Code: "drive.letter_in_use", Message: "original"}, "original"},
	} {
		if got := Format(test.language, &test.message); got != test.want {
			t.Fatalf("%+v: %q", test, got)
		}
	}
	for code, entry := range messages {
		left, right := namedToken.FindAllString(entry["en"], -1), namedToken.FindAllString(entry["pt-BR"], -1)
		sort.Strings(left)
		sort.Strings(right)
		if entry["en"] == "" || entry["pt-BR"] == "" || !reflect.DeepEqual(left, right) {
			t.Fatalf("invalid catalog entry %s", code)
		}
		params := map[string]string{}
		for _, token := range left {
			params[token[1:len(token)-1]] = "synthetic"
		}
		if Format("pt-BR", NewMessage(code, params)) == "" {
			t.Fatalf("empty message %s", code)
		}
	}
}
