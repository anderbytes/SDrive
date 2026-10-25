package i18n

import (
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestLanguageAndFallback(t *testing.T) {
	if !Valid("en") || !Valid("pt-BR") || Valid("unknown") {
		t.Fatal("unexpected supported languages")
	}
	for _, test := range []struct{ language, source, want string }{
		{"en", "Open SDrive", "Open SDrive"},
		{"pt-BR", "Open SDrive", "Abrir SDrive"},
		{"pt-BR", "Select credential file", "Selecionar arquivo de acesso"},
		{"pt-BR", "Unknown message", "Unknown message"},
		{"pt-BR", "Engine stopped (exit code 17; last signal: network). Restart SDrive to recover your drives.", "O mecanismo parou (código de saída 17; último sinal: network). Reinicie o SDrive para recuperar suas unidades."},
		{"pt-BR", "rclone exited during startup (exit code 17; last signal: credentials). Restart SDrive to recover your drives.", "O rclone encerrou durante a inicialização (código de saída 17; último sinal: credentials). Reinicie o SDrive para recuperar suas unidades."},
		{"pt-BR", "recover connection Root before exiting safely", "recupere a conexão Root antes de sair com segurança"},
	} {
		if got := Text(test.language, test.source); got != test.want {
			t.Fatalf("Text(%q, %q) = %q; want %q", test.language, test.source, got, test.want)
		}
	}
}

func TestLocaleCatalogsAreCompleteAndPreserveParameters(t *testing.T) {
	named := regexp.MustCompile(`\{\w+\}`)
	for language, catalog := range translations {
		if !Valid(language) || len(catalog) != len(translations["pt-BR"]) {
			t.Fatalf("invalid catalog: %s", language)
		}
		for source := range translations["pt-BR"] {
			translated := catalog[source]
			if strings.TrimSpace(translated) == "" || Text(language, source) != translated {
				t.Fatalf("%s: missing or incorrect translation for %q", language, source)
			}
			if !reflect.DeepEqual(formatToken.FindAllString(source, -1), formatToken.FindAllString(translated, -1)) {
				t.Fatalf("%s: reordered format parameters for %q", language, source)
			}
			left, right := named.FindAllString(source, -1), named.FindAllString(translated, -1)
			sort.Strings(left)
			sort.Strings(right)
			if !reflect.DeepEqual(left, right) {
				t.Fatalf("%s: changed named parameters for %q", language, source)
			}
		}
		source := "Engine stopped (exit code %d; last signal: %s). Restart SDrive to recover your drives."
		fill := strings.NewReplacer("%d", "17", "%s", "network")
		if got := Text(language, fill.Replace(source)); got != fill.Replace(catalog[source]) {
			t.Fatalf("%s: dynamic translation mismatch: %s", language, got)
		}
		if Text(language, "Unknown message") != "Unknown message" {
			t.Fatal("missing English fallback")
		}
	}
}
