package i18n

import "testing"

func TestLanguageAndFallback(t *testing.T) {
	if !Valid("en") || !Valid("pt-BR") || Valid("unknown") {
		t.Fatal("unexpected supported languages")
	}
	for _, test := range []struct{ language, source, want string }{
		{"en", "Open SDrive", "Open SDrive"},
		{"pt-BR", "Open SDrive", "Abrir SDrive"},
		{"pt-BR", "Select credential file", "Selecionar arquivo de acesso"},
		{"pt-BR", "Unknown message", "Unknown message"},
		{"pt-BR", "recover connection Root before exiting safely", "recupere a conexão Root antes de sair com segurança"},
	} {
		if got := Text(test.language, test.source); got != test.want {
			t.Fatalf("Text(%q, %q) = %q; want %q", test.language, test.source, got, test.want)
		}
	}
}
