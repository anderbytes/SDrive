package integration

import (
	"fmt"
	"os"
	"testing"

	"sdrive/internal/platform"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 3 && os.Args[1] == "--config-key" {
		key, err := platform.Key(os.Args[2])
		if err != nil {
			os.Exit(1)
		}
		fmt.Print(key)
		os.Exit(0)
	}
	if _, err := os.Stat("go.mod"); os.IsNotExist(err) {
		if err := os.Chdir("../.."); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}
