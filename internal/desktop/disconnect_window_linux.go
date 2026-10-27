package desktop

import "github.com/wailsapp/wails/v3/pkg/application"

func newDisconnectWindow(app *application.App) *application.WebviewWindow {
	return app.Window.NewWithOptions(application.WebviewWindowOptions{Name: disconnectWindowName, Title: "SDrive", URL: "/?view=disconnect-progress", Width: 380, Height: 280, Hidden: true, DisableResize: true, AlwaysOnTop: true, DefaultContextMenuDisabled: true})
}
func showDisconnectWindow(window *application.WebviewWindow) { window.Show() }
