package main

import (
	"embed"

	"sdrive/internal/desktop"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed installer/prerequisites.json
var prerequisiteManifest []byte

//go:embed frontend/package.json
var packageMetadata []byte

// Set only in the separate, locally built UI smoke-test binary.
var uiSmoke string

func main() {
	desktop.Run(assets, prerequisiteManifest, packageMetadata, uiSmoke)
}
