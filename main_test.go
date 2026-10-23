package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"reflect"
	"testing"
	"unicode/utf16"
)

func TestSmokeDebuggerIsBuildOnly(t *testing.T) {
	previous := uiSmoke
	t.Cleanup(func() { uiSmoke = previous })
	t.Setenv("SDRIVE_UI_SMOKE", "true")
	for _, value := range []string{"", "false", "TRUE"} {
		uiSmoke = value
		if args := smokeBrowserArgs(); len(args) != 0 {
			t.Fatalf("normal build enables debugger: %v", args)
		}
	}
	uiSmoke = "true"
	if !reflect.DeepEqual(smokeBrowserArgs(), []string{"--remote-debugging-port=0", "--remote-debugging-address=127.0.0.1"}) {
		t.Fatal("smoke debugger must use ephemeral loopback port")
	}
}

func TestMain(m *testing.M) {
	if len(os.Args) == 3 && os.Args[1] == "--config-key" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestInstallerLogPreservesPortugueseText(t *testing.T) {
	var output bytes.Buffer
	text := "\ufeffVerification completed — 100%"
	n, err := (installerLog{&output}).Write([]byte(text))
	if err != nil || n != len(text) {
		t.Fatal("installer output failed")
	}
	encoded := output.Bytes()
	units := make([]uint16, len(encoded)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(encoded[i*2:])
	}
	if string(utf16.Decode(units)) != text {
		t.Fatal("Portuguese text was corrupted")
	}
}
func TestRealEngineSelfCheck(t *testing.T) {
	if _, err := os.Stat(".tools/rclone/rclone.exe"); err != nil {
		t.Skip("execute scripts/bootstrap.ps1 para validar o rclone real")
	}
	if err := selfCheck(); err != nil {
		t.Fatal(err)
	}
}
