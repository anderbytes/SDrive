package desktop

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"sdrive/internal/core"
	"sdrive/internal/diagnostics"
	"sdrive/internal/engine"
	"sdrive/internal/i18n"
	"sdrive/internal/platform"
)

// Run dispatches auxiliary commands or starts the desktop application.
func Run(assets fs.FS, prerequisiteManifest, packageMetadata []byte, smoke string) {
	if handleCommand(prerequisiteManifest) {
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
	options.Windows.AdditionalBrowserArgs = smokeBrowserArgs(smoke)
	app := application.New(options)
	service, err := core.New(dir, nil, "")
	if err != nil {
		platform.Alert(err.Error())
		return
	}
	if smoke == "true" {
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
		return StartConnectionEngine(dir, core.CachePath(service), c, proxy, diagnosticLog)
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
		if smoke == "true" {
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

func smokeBrowserArgs(smoke string) []string {
	if smoke != "true" {
		return nil
	}
	return []string{"--remote-debugging-port=0", "--remote-debugging-address=127.0.0.1"}
}
