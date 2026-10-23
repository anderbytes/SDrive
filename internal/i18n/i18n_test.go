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
		{"pt-BR", "Engine stopped (exit code 17; last signal: network). Restart SDrive to recover your drives.", "O mecanismo parou (código de saída 17; último sinal: network). Reinicie o SDrive para recuperar suas unidades."},
		{"pt-BR", "rclone exited during startup (exit code 17; last signal: credentials). Restart SDrive to recover your drives.", "O rclone encerrou durante a inicialização (código de saída 17; último sinal: credentials). Reinicie o SDrive para recuperar suas unidades."},
		{"pt-BR", "recover connection Root before exiting safely", "recupere a conexão Root antes de sair com segurança"},
	} {
		if got := Text(test.language, test.source); got != test.want {
			t.Fatalf("Text(%q, %q) = %q; want %q", test.language, test.source, got, test.want)
		}
	}
}
