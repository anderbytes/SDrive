//go:build windows

package desktop

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/w32"
	"golang.org/x/sys/windows"
	"sdrive/internal/core"
)

func TestDisconnectOverlayNative(t *testing.T) {
	if os.Getenv("SDRIVE_TEST_DESKTOP") != "1" {
		t.Skip("set SDRIVE_TEST_DESKTOP=1 to exercise the isolated native WebView2 overlay")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDisconnectOverlayNativeChild$", "-test.v")
	for _, value := range os.Environ() {
		key := strings.ToUpper(strings.SplitN(value, "=", 2)[0])
		if key != "LOCALAPPDATA" && !strings.HasPrefix(key, "WEBVIEW2_") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	profile := t.TempDir()
	cmd.Env = append(cmd.Env, "LOCALAPPDATA="+profile, "SDRIVE_DISCONNECT_NATIVE_CHILD=1")
	defer func() {
		// WebView2 children can release their profile briefly after the host exits.
		for attempts := 0; attempts < 40; attempts++ {
			if err := os.RemoveAll(profile); err == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("native overlay failed: %v\n%s", err, output)
	}
}

func TestDisconnectOverlayNativeChild(t *testing.T) {
	if os.Getenv("SDRIVE_DISCONNECT_NATIVE_CHILD") != "1" {
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil {
		t.Fatal(err)
	}
	defer windows.CoUninitialize()
	content := os.DirFS("frontend/dist")
	app := application.New(application.Options{Name: "SDrive overlay test", LogLevel: slog.LevelError,
		Windows: application.WindowsOptions{WebviewUserDataPath: filepath.Join(os.Getenv("LOCALAPPDATA"), "webview")},
		Assets:  application.AssetOptions{Handler: application.BundledAssetFileServer(content)}})
	service, err := core.New(t.TempDir(), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	core.SetAutoStart(service, func(bool) error { return nil })
	host := app.Window.NewWithOptions(application.WebviewWindowOptions{Name: "overlay-host", Title: "SDrive overlay test", Width: 600, Height: 400, HTML: "<html><body><input aria-label='Focus probe'></body></html>"})
	d := &desktop{app: app, window: host, service: service}
	o := setupDisconnectOverlay(d)
	core.SetAccessChanged(service, func(bool) { application.InvokeAsync(o.render) })
	core.SetLanguageChanged(service, func(string) { application.InvokeAsync(o.render) })
	text := make(chan string, 16)
	app.Event.On("overlay:probe", func(event *application.CustomEvent) {
		if value, ok := event.Data.(string); ok {
			text <- value
		}
	})
	result := make(chan error, 1)
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		go func() {
			result <- exerciseNativeDisconnectOverlay(o, text)
			application.InvokeAsync(func() {
				d.stopped.Store(true)
				if o.timer != nil {
					o.timer.Stop()
				}
				app.Quit()
			})
		}()
	})
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func exerciseNativeDisconnectOverlay(o *disconnectOverlay, text <-chan string) error {
	onDesktop := func(check func() error) error {
		done := make(chan error, 1)
		application.InvokeAsync(func() { done <- check() })
		select {
		case err := <-done:
			return err
		case <-time.After(5 * time.Second):
			return fmt.Errorf("desktop thread timed out")
		}
	}
	awaitText := func(want, absent string) error {
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			if err := onDesktop(func() error {
				o.window.ExecJS(`fetch("/wails/runtime", {method:"POST", headers:{"Content-Type":"application/json", "x-wails-client-id":window._wails.clientId}, body:JSON.stringify({object:3, method:0, args:{name:"overlay:probe", data:document.body.innerText}})})`)
				return nil
			}); err != nil {
				return err
			}
			select {
			case value := <-text:
				if strings.Contains(value, want) && (absent == "" || !strings.Contains(value, absent)) {
					return nil
				}
			case <-time.After(100 * time.Millisecond):
			}
		}
		return fmt.Errorf("native WebView did not render %q", want)
	}
	var foreground w32.HWND
	if err := onDesktop(func() error {
		o.desktop.window.Focus()
		foreground = w32.GetForegroundWindow()
		o.update(core.DisconnectProgress{Sequence: 1, ID: "one", Name: "Private fixture", Stage: "checkingUploads"})
		return nil
	}); err != nil {
		return err
	}
	if err := awaitText("Checking pending uploads…", ""); err != nil {
		return err
	}
	if err := onDesktop(func() error {
		if !o.window.IsVisible() || w32.GetForegroundWindow() != foreground {
			return fmt.Errorf("overlay visible=%v foreground=%v previous=%v host=%v overlay=%v", o.window.IsVisible(), w32.GetForegroundWindow(), foreground, w32.HWND(uintptr(o.desktop.window.NativeWindow())), w32.HWND(uintptr(o.window.NativeWindow())))
		}
		o.desktop.window.Minimise()
		o.update(core.DisconnectProgress{Sequence: 2, ID: "one", Name: "Private fixture", Stage: "waitingUploads"})
		o.update(core.DisconnectProgress{Sequence: 3, ID: "two", Name: "Other fixture", Stage: "unmounting"})
		return nil
	}); err != nil {
		return err
	}
	if err := awaitText("Waiting for uploads…", ""); err != nil {
		return err
	}
	if err := onDesktop(func() error {
		if !o.window.IsVisible() || !o.desktop.window.IsMinimised() {
			return fmt.Errorf("overlay did not remain visible while the host was minimised")
		}
		return nil
	}); err != nil {
		return err
	}
	settings, err := o.desktop.service.PreferenceDefaults()
	if err != nil {
		return err
	}
	settings.Language = "pt-BR"
	if err := o.desktop.service.SaveSettings(settings); err != nil {
		return err
	}
	if err := awaitText("Desconectando unidades…", ""); err != nil {
		return err
	}
	if _, err := o.desktop.service.ConfigureAccess("", "123456", core.AccessPolicy{IdleMinutes: 15}); err != nil {
		return err
	}
	if err := o.desktop.service.Lock(); err != nil {
		return err
	}
	if err := awaitText("Unidade", "Private fixture"); err != nil {
		return err
	}
	if err := onDesktop(func() error {
		o.update(core.DisconnectProgress{Sequence: 4, ID: "one", Name: "Private fixture", Stage: "succeeded"})
		o.update(core.DisconnectProgress{Sequence: 5, ID: "two", Name: "Other fixture", Stage: "failed"})
		return nil
	}); err != nil {
		return err
	}
	if err := awaitText("Unidade desconectada.", "Private fixture"); err != nil {
		return err
	}
	if err := awaitText("Abra o SDrive", "Unidade desconectada."); err != nil {
		return err
	}
	if err := onDesktop(func() error {
		if !o.window.IsVisible() || len(o.entries) != 1 || o.entries["two"].Stage != "failed" {
			return fmt.Errorf("failure disappeared or success did not expire")
		}
		handle := w32.HWND(uintptr(o.window.NativeWindow()))
		if w32.GetWindowLongPtr(handle, w32.GWL_EXSTYLE)&w32.WS_EX_NOACTIVATE != 0 {
			return fmt.Errorf("overlay controls cannot receive keyboard focus")
		}
		o.window.Focus()
		if w32.GetForegroundWindow() != handle {
			return fmt.Errorf("overlay could not receive focus after user interaction")
		}
		o.window.ExecJS(`document.querySelector("footer button:last-child").click()`)
		return nil
	}); err != nil {
		return err
	}
	for attempts := 0; attempts < 50; attempts++ {
		closed := false
		if err := onDesktop(func() error { closed = !o.window.IsVisible(); return nil }); err != nil {
			return err
		}
		if closed {
			return exerciseNativeDisconnectSuccessExpiry(o, onDesktop, awaitText)
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("close button did not dismiss the failure")
}

func exerciseNativeDisconnectSuccessExpiry(o *disconnectOverlay, onDesktop func(func() error) error, awaitText func(string, string) error) error {
	var completed time.Time
	if err := onDesktop(func() error {
		completed = time.Now()
		o.update(core.DisconnectProgress{Sequence: 6, ID: "one", Name: "Fixture", Stage: "succeeded"})
		return nil
	}); err != nil {
		return err
	}
	if err := awaitText("Unidade desconectada.", ""); err != nil {
		return err
	}
	for time.Since(completed) < 2*time.Second {
		visible := false
		if err := onDesktop(func() error { visible = o.window.IsVisible(); return nil }); err != nil {
			return err
		}
		if !visible {
			if time.Since(completed) < time.Second {
				return fmt.Errorf("success overlay disappeared before one second")
			}
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("success overlay did not hide within two seconds")
}
