package desktop

import (
	"os"
	"path/filepath"

	"sdrive/internal/core"
	"sdrive/internal/diagnostics"
	"sdrive/internal/engine"
	"sdrive/internal/platform"
)

// StartConnectionEngine starts the isolated worker for a connection.
func StartConnectionEngine(dir, cachePath string, c core.Connection, proxy string, diagnosticLog *diagnostics.Collector) (core.RPC, error) {
	binary, err := engine.FindBinary()
	if err != nil {
		return nil, err
	}
	workerDir := filepath.Join(dir, "connections", "sd_"+c.Remote)
	if !c.SessionOnly {
		if err = os.MkdirAll(workerDir, 0700); err != nil {
			return nil, err
		}
		if err = platform.ProtectDir(workerDir); err != nil {
			return nil, err
		}
		if _, err = platform.Key(workerDir); err != nil {
			return nil, err
		}
		if err = engine.Init(binary, workerDir); err != nil {
			return nil, err
		}
	}
	return engine.StartWithOptions(binary, workerDir, diagnosticLog, engine.Options{CacheDir: cachePath, Memory: c.SessionOnly, UploadLimit: c.UploadLimit, DownloadLimit: c.DownloadLimit, ProxyURL: proxy, ConnectTimeout: c.ConnectTimeout, IdleTimeout: c.IdleTimeout})
}
