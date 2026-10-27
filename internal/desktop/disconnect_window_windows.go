package desktop

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"
)

func newDisconnectWindow(app *application.App) *application.WebviewWindow {
	var window *application.WebviewWindow
	foreground := w32.GetForegroundWindow()
	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: disconnectWindowName, Title: "SDrive", URL: "/?view=disconnect-progress",
		Width: 380, Height: 280, Hidden: true, Frameless: true, AlwaysOnTop: true,
		DisableResize: true, DefaultContextMenuDisabled: true,
		Windows: application.WindowsWindow{HiddenOnTaskbar: true,
			ExStyle: w32.WS_EX_CONTROLPARENT | w32.WS_EX_TOOLWINDOW | w32.WS_EX_TOPMOST | w32.WS_EX_NOACTIVATE},
	})
	// Wails applies the frameless frame with an activating SetWindowPos during creation.
	if foreground != 0 && w32.GetForegroundWindow() == w32.HWND(uintptr(window.NativeWindow())) {
		w32.SetForegroundWindow(foreground)
	}
	return window
}
func showDisconnectWindow(window *application.WebviewWindow) {
	// Suppress activation only while showing; clicks and keyboard focus remain available.
	foreground := w32.GetForegroundWindow()
	handle := w32.HWND(uintptr(window.NativeWindow()))
	style := w32.GetWindowLongPtr(handle, w32.GWL_EXSTYLE)
	w32.SetWindowLongPtr(handle, w32.GWL_EXSTYLE, style|w32.WS_EX_NOACTIVATE)
	window.Show()
	if !window.IsVisible() {
		window.Show()
	}
	w32.SetWindowLongPtr(handle, w32.GWL_EXSTYLE, style&^w32.WS_EX_NOACTIVATE)
	if foreground != 0 && w32.GetForegroundWindow() == handle {
		w32.SetForegroundWindow(foreground)
	}
}
