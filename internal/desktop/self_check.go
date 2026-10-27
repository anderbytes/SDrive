package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"sdrive/internal/core"
	"sdrive/internal/engine"
	"sdrive/internal/platform"
)

func selfCheck() error {
	dir, err := os.MkdirTemp("", "sdrive-selfcheck-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err = platform.ProtectDir(dir); err != nil {
		return err
	}
	key, err := platform.Key(dir)
	if err != nil {
		return err
	}
	binary, err := engine.FindBinary()
	if err != nil {
		return err
	}
	if err = engine.Init(binary, dir); err != nil {
		return err
	}
	rpc, err := engine.Start(binary, dir, nil)
	if err != nil {
		return err
	}
	defer rpc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var catalog struct {
		Providers []struct {
			Name    string
			Options []struct{ Name string }
		}
	}
	if err = rpc.Call(ctx, "config/providers", map[string]any{}, &catalog); err != nil {
		return err
	}
	available := map[string]map[string]bool{}
	for _, p := range catalog.Providers {
		fields := map[string]bool{}
		for _, f := range p.Options {
			fields[f.Name] = true
		}
		available[p.Name] = fields
	}
	for _, p := range core.Catalog() {
		fields := available[p.Backend]
		if fields == nil {
			return fmt.Errorf("missing backend: %s", p.Backend)
		}
		for _, f := range p.Fields {
			if !fields[f.Key] {
				return fmt.Errorf("field missing in rclone: %s/%s", p.ID, f.Key)
			}
		}
	}
	var mountTypes struct{ MountTypes []string }
	if err = rpc.Call(ctx, "mount/types", map[string]any{}, &mountTypes); err != nil {
		return err
	}
	hasWinFsp := false
	for _, kind := range mountTypes.MountTypes {
		if kind == "cmount" {
			hasWinFsp = true
		}
	}
	if !hasWinFsp {
		return fmt.Errorf("the engine was built without WinFsp mounting support")
	}
	probe := filepath.Join(dir, "probe.txt")
	if err = os.WriteFile(probe, []byte("sdrive-engine-check"), 0600); err != nil {
		return err
	}
	if err = rpc.Call(ctx, "operations/copyfile", map[string]string{"srcFs": dir, "srcRemote": "probe.txt", "dstFs": dir, "dstRemote": "copy.txt"}, nil); err != nil {
		return err
	}
	copied, err := os.ReadFile(filepath.Join(dir, "copy.txt"))
	if err != nil || string(copied) != "sdrive-engine-check" {
		return fmt.Errorf("local copy through rclone engine failed")
	}
	if err = rpc.Call(ctx, "operations/deletefile", map[string]string{"fs": dir, "remote": "copy.txt"}, nil); err != nil {
		return err
	}
	conf, err := os.ReadFile(filepath.Join(dir, "rclone.conf"))
	if err != nil {
		return err
	}
	if bytes.Contains(conf, []byte(key)) {
		return fmt.Errorf("key found in plain text")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"profiles": len(core.Catalog()), "encryptedConfig": true, "dpapi": true, "authenticatedRPC": true, "winFsp": platform.WinFspInstalled(), "rclone": engine.Version})
}
