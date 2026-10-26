package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"sdrive/internal/core"
	"sdrive/internal/diagnostics"
	"sdrive/internal/engine"
	"sdrive/internal/i18n"
	"sdrive/internal/platform"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed installer/prerequisites.json
var prerequisiteManifest []byte

//go:embed frontend/package.json
var packageMetadata []byte

// Set only in the separate, locally built UI smoke-test binary.
var uiSmoke string

// nsExec expects UTF-16 output for Unicode download details.
type installerLog struct{ io.Writer }

func (w installerLog) Write(data []byte) (int, error) {
	units := utf16.Encode([]rune(string(data)))
	encoded := make([]byte, len(units)*2)
	for i, unit := range units {
		binary.LittleEndian.PutUint16(encoded[i*2:], unit)
	}
	n, err := w.Writer.Write(encoded)
	if err != nil {
		return 0, err
	}
	if n != len(encoded) {
		return 0, io.ErrShortWrite
	}
	return len(data), nil
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--restore-startup" {
		if err := restoreStartup(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--cleanup-drive-icons" {
		if err := errors.Join(platform.CleanupDriveIcons(), platform.CleanupDriveLabels()); err != nil {
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 4 && (os.Args[1] == "--download-prerequisite" || os.Args[1] == "--install-prerequisite") {
		component, err := platform.PrerequisiteFromManifest(prerequisiteManifest, os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(13)
		}
		if os.Args[1] == "--install-prerequisite" {
			os.Exit(platform.InstallPrerequisite(component, filepath.Join(os.Args[3], component.File)))
		}
		log := installerLog{os.Stdout}
		fmt.Fprint(log, "\ufeff")
		if err = platform.DownloadPrerequisite(context.Background(), component, os.Args[3], log); err != nil {
			fmt.Fprintln(log, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--check-prerequisites" {
		os.Exit(platform.PrerequisiteStatus())
	}
	if len(os.Args) == 3 && os.Args[1] == "--config-key" {
		key, err := platform.Key(os.Args[2])
		if err != nil {
			os.Exit(1)
		}
		fmt.Print(key)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--self-check" {
		if err := selfCheck(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	dir, err := platform.DataDir()
	if err != nil {
		platform.Alert(err.Error())
		return
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		platform.Alert(err.Error())
		return
	}
	if err = platform.ProtectDir(dir); err != nil {
		platform.Alert("Could not restrict the data directory to the Windows user.")
		return
	}
	instanceID, err := platform.InstanceID(dir)
	if err != nil {
		platform.Alert(err.Error())
		return
	}
	unlock, err := platform.SingleInstance(dir)
	if err != nil {
		if errors.Is(err, platform.ErrInstanceRunning) {
			if err = platform.ActivateInstance(instanceID); err == nil {
				return
			}
		}
		platform.Alert(err.Error())
		return
	}
	defer unlock()
	diagnosticLog := diagnostics.New(dir)
	diagnosticLog.Record("application", "started", "none", nil)
	defer func() { diagnosticLog.Operation("application", nil); diagnosticLog.Close() }()
	var rpc *engine.Engine
	var engineMu sync.Mutex
	defer func() {
		engineMu.Lock()
		defer engineMu.Unlock()
		if rpc != nil {
			rpc.Close()
		}
	}()
	content, _ := fs.Sub(assets, "frontend/dist")
	var activationMu sync.Mutex
	var activate func()
	activationPending := false
	options := application.Options{Name: "SDrive", Description: "Open source remote storage", LogLevel: slog.LevelError, Assets: application.AssetOptions{Handler: application.BundledAssetFileServer(content)}, Windows: application.WindowsOptions{WebviewUserDataPath: filepath.Join(dir, "webview")}, SingleInstance: &application.SingleInstanceOptions{UniqueID: instanceID, OnSecondInstanceLaunch: func(application.SecondInstanceData) {
		activationMu.Lock()
		show := activate
		if show == nil {
			activationPending = true
		}
		activationMu.Unlock()
		if show != nil {
			application.InvokeAsync(show)
		}
	}}}
	options.Windows.AdditionalBrowserArgs = smokeBrowserArgs()
	app := application.New(options)
	service, err := core.New(dir, nil, "")
	if err != nil {
		platform.Alert(err.Error())
		return
	}
	if uiSmoke == "true" {
		// Disposable preferences must never change the user's Windows startup entry.
		core.SetAutoStart(service, func(bool) error { return nil })
	} else {
		diagnosticLog.Operation("startup_restore", core.ReconcileStartup(service))
	}
	core.SetDiagnostics(service, diagnosticLog)
	core.SetDriveLabels(service, func(connections []core.Connection) error {
		drives := []platform.DriveLabel{}
		for _, c := range connections {
			if c.MountMode != "local" {
				drives = append(drives, platform.DriveLabel{Letter: c.Letter, Name: c.Name})
			}
		}
		return platform.SyncDriveLabels(dir, drives)
	})
	core.SetDriveIcons(service, func(connections []core.Connection) error {
		drives := []platform.DriveIcon{}
		for _, c := range connections {
			drives = append(drives, platform.DriveIcon{Letter: c.Letter, Provider: c.Provider})
		}
		return platform.SyncDriveIcons(dir, drives)
	})
	app.RegisterService(application.NewService(service))
	core.SetDesktop(service, func(target string) error {
		if filepath.IsAbs(target) {
			return app.Browser.OpenFile(target)
		}
		return app.Browser.OpenURL(target)
	}, app.Quit, func() (core.RPC, error) {
		engineMu.Lock()
		defer engineMu.Unlock()
		if rpc != nil {
			rpc.Close()
		}
		binary, err := engine.FindBinary()
		diagnosticLog.Operation("engine_find", err)
		if err != nil {
			return nil, err
		}
		if _, err = platform.Key(dir); err != nil {
			diagnosticLog.Operation("key", err)
			return nil, err
		}
		diagnosticLog.Operation("key", nil)
		if err = engine.Init(binary, dir); err != nil {
			diagnosticLog.Operation("engine_init", err)
			return nil, err
		}
		diagnosticLog.Operation("engine_init", nil)
		rpc, err = engine.Start(binary, dir, diagnosticLog)
		if err != nil {
			return nil, err
		}
		return rpc, nil
	})
	core.SetConnectionEngine(service, func(c core.Connection, proxy string) (core.RPC, error) {
		return startConnectionEngine(dir, core.CachePath(service), c, proxy, diagnosticLog)
	})
	defer core.CloseConnectionEngines(service)
	core.StartEngine(service)
	hidden := len(os.Args) > 1 && os.Args[1] == "--background" && service.Snapshot().SettingsError == ""
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "SDrive", Width: 1180, Height: 800, MinWidth: 850, MinHeight: 620, Hidden: hidden})
	core.SetFilePicker(service, func(kind string) (string, error) {
		dialog := app.Dialog.OpenFile().AttachToWindow(window).SetTitle(i18n.Text(core.Language(service), "Select credential file"))
		if kind == "service_account_file" {
			dialog.AddFilter(i18n.Text(core.Language(service), "JSON credentials"), "*.json")
		}
		return dialog.PromptForSingleSelection()
	})
	core.SetFileAccessPicker(service, func(root string) (string, error) {
		return app.Dialog.OpenFile().AttachToWindow(window).SetTitle(i18n.Text(core.Language(service), "Select mounted file")).SetDirectory(root).CanChooseDirectories(false).CanChooseFiles(true).PromptForSingleSelection()
	})
	core.SetDirectoryPicker(service, func() (string, error) {
		return app.Dialog.OpenFile().AttachToWindow(window).SetTitle(i18n.Text(core.Language(service), "Select local directory")).CanChooseDirectories(true).CanChooseFiles(false).PromptForSingleSelection()
	})
	core.SetPortableFilePicker(service, func(save bool) (string, error) {
		if uiSmoke == "true" {
			return filepath.Join(filepath.Dir(dir), "portable.json"), nil
		}
		if save {
			return app.Dialog.SaveFile().AttachToWindow(window).SetMessage(i18n.Text(core.Language(service), "Export portable configuration")).SetFilename("SDrive-configuration.json").AddFilter("JSON", "*.json").PromptForSingleSelection()
		}
		return app.Dialog.OpenFile().AttachToWindow(window).SetTitle(i18n.Text(core.Language(service), "Import portable configuration")).AddFilter("JSON", "*.json").PromptForSingleSelection()
	})
	var metadata struct {
		Version string `json:"version"`
	}
	_ = json.Unmarshal(packageMetadata, &metadata)
	desktop := setupDesktop(app, window, service, dir, metadata.Version, hidden)
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		<-desktop.ready
		activationMu.Lock()
		activate = desktop.show
		pending := activationPending
		activationMu.Unlock()
		if pending {
			application.InvokeAsync(desktop.show)
		}
	})
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) { go core.AutoMount(service) })
	if err = app.Run(); err != nil {
		diagnosticLog.Operation("application", err)
		platform.Alert("Could not open SDrive. Run the SDrive installer again to repair the required components.")
	}
}

func smokeBrowserArgs() []string {
	if uiSmoke != "true" {
		return nil
	}
	return []string{"--remote-debugging-port=0", "--remote-debugging-address=127.0.0.1"}
}

func trayIcon() []byte {
	return trayIconColor(color.RGBA{33, 185, 147, 255})
}

func trayIconColor(fill color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 3; y < 29; y++ {
		for x := 3; x < 29; x++ {
			img.Set(x, y, fill)
		}
	}
	for y := 9; y < 23; y++ {
		for x := 8; x < 24; x++ {
			if y < 12 || y > 19 || x < 11 || x > 20 {
				img.Set(x, y, color.RGBA{11, 30, 37, 255})
			}
		}
	}
	var out bytes.Buffer
	_ = png.Encode(&out, img)
	return out.Bytes()
}

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

func startConnectionEngine(dir, cachePath string, c core.Connection, proxy string, diagnosticLog *diagnostics.Collector) (core.RPC, error) {
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
