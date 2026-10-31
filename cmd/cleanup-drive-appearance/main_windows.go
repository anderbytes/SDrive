package main

import (
	"os"

	"sdrive/internal/platform"
)

func main() {
	if len(os.Args) != 2 || platform.CleanupDriveAppearance(os.Args[1]) != nil {
		os.Exit(1)
	}
}
