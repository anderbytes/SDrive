package i18n

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
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
		for _, language := range []string{"en", "pt-BR", "es", "fr", "de", "it", "zh-CN"} {
			left, right := namedToken.FindAllString(entry["en"], -1), namedToken.FindAllString(entry[language], -1)
			sort.Strings(left)
			sort.Strings(right)
			if entry["en"] == "" || entry[language] == "" || !reflect.DeepEqual(left, right) {
				t.Fatalf("invalid catalog entry %s", code)
			}
			params := map[string]string{}
			for _, token := range left {
				params[token[1:len(token)-1]] = "Café {letter} %s 😀"
			}
			if Format(language, NewMessage(code, params)) == "" {
				t.Fatalf("empty message %s", code)
			}
			if !strings.Contains(Format(language, NewMessage(code, params)), "Café {letter} %s 😀") {
				t.Fatal("parameter was interpreted as a template")
			}
		}
	}
}
