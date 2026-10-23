package core

import "sdrive/internal/platform"

func replaceFile(src, dst string) error { return platform.Replace(src, dst) }
