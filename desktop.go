package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"sdrive/internal/core"
	"sdrive/internal/i18n"
	"sdrive/internal/platform"
)

type windowGeometry struct {
	Bounds    application.Rect `json:"bounds"`
	Maximised bool             `json:"maximised"`
}

func clampGeometry(bounds application.Rect, screens []*application.Screen) (application.Rect, application.Rect) {
	if bounds.Width <= 0 || bounds.Height <= 0 || bounds.Width > 100000 || bounds.Height > 100000 || bounds.X < -1000000 || bounds.X > 1000000 || bounds.Y < -1000000 || bounds.Y > 1000000 {
		bounds = application.Rect{}
	}
	if len(screens) == 0 {
		return application.Rect{Width: 1180, Height: 800}, application.Rect{Width: 1180, Height: 800}
	}
	area := screens[0].WorkArea
	for _, screen := range screens {
		if screen.IsPrimary {
			area = screen.WorkArea
		}
	}
	best := 0
	for _, screen := range screens {
		a := screen.WorkArea
		intersection := max(0, min(bounds.X+bounds.Width, a.X+a.Width)-max(bounds.X, a.X)) * max(0, min(bounds.Y+bounds.Height, a.Y+a.Height)-max(bounds.Y, a.Y))
		if intersection > best {
			best, area = intersection, a
		}
	}
	if best == 0 {
		bounds = application.Rect{X: area.X + (area.Width-1180)/2, Y: area.Y + (area.Height-800)/2, Width: 1180, Height: 800}
	}
	bounds.Width = min(max(bounds.Width, 850), area.Width)
	bounds.Height = min(max(bounds.Height, 620), area.Height)
	bounds.X = max(area.X, min(bounds.X, area.X+area.Width-bounds.Width))
	bounds.Y = max(area.Y, min(bounds.Y, area.Y+area.Height-bounds.Height))
	return bounds, area
}

type trayStatus struct {
	Connected int
	State     string
	Problems  map[string]string
}

func statusForTray(view core.View) trayStatus {
	status := trayStatus{State: "Disconnected", Problems: map[string]string{}}
	for key, message := range map[string]string{"engine": view.EngineError, "settings": view.SettingsError, "maintenance": view.MaintenanceError, "startup": view.StartupWarning} {
		if message != "" {
			status.Problems[key] = message
			status.State = "Attention required"
		}
	}
	if view.RecoveryHold {
		status.Problems["recovery"] = "Settings recovery"
	}
	for _, c := range view.Connections {
		if c.Mounted {
			status.Connected++
		}
		if c.Busy {
			continue
		}
		problem := c.Error
		if problem == "" {
			problem = c.Cache.Error
		}
		if problem == "" && c.Uploads != nil && c.Uploads.CacheErrors > 0 {
			problem = "Cache errors"
		}
		if problem != "" {
			status.State = "Attention required"
		}
		if problem == "" && c.Recovery && !c.Mounted {
			problem = "Recovery required"
		}
		if problem == "" && c.Pending == nil {
			problem = "Pending status unknown"
		}
		if problem != "" {
			status.Problems[c.ID] = problem
		}
	}
	if status.Connected > 0 && status.State != "Attention required" {
		status.State = "Connected"
	}
	if status.State != "Attention required" {
		for _, c := range view.Connections {
			if c.Busy || (c.Pending != nil && *c.Pending > 0) {
				status.State = "Uploading or working"
			}
		}
	}
	if len(status.Problems) > 0 && status.State != "Attention required" {
		status.State = "Recovery or unknown status"
	}
	return status
}

func newTrayProblem(previous, current map[string]string) bool {
	for id, message := range current {
		if previous[id] != message {
			return true
		}
	}
	return false
}

type desktop struct {
	app             *application.App
	window          *application.WebviewWindow
	service         *core.Service
	dir             string
	visible         atomic.Bool
	stopped         atomic.Bool
	mu              sync.Mutex
	sequence        uint64
	view            core.View
	wake            chan struct{}
	done            chan struct{}
	ready           chan struct{}
	tray            *application.SystemTray
	menu            *application.Menu
	menuOpen        bool
	problems        map[string]string
	geometry        windowGeometry
	geometryTimer   *time.Timer
	geometryWarning bool
	geometryReady   bool
	iconState       string
	actions         sync.Map
}

func (d *desktop) accept(sequence uint64, view core.View) {
	d.mu.Lock()
	if sequence > d.sequence {
		d.sequence, d.view = sequence, view
		select {
		case d.wake <- struct{}{}:
		default:
		}
	}
	d.mu.Unlock()
}

func (d *desktop) latest() core.View { d.mu.Lock(); defer d.mu.Unlock(); return d.view }

func (d *desktop) show() {
	if d.stopped.Load() {
		return
	}
	d.window.Show()
	// Windows can apply a launcher's SW_HIDE to the first ShowWindow call.
	if !d.window.IsVisible() {
		d.window.Show()
	}
	if d.window.IsMinimised() {
		d.window.UnMinimise()
	}
	if !d.window.IsMaximised() {
		bounds, area := clampGeometry(d.window.Bounds(), d.app.Screen.GetAll())
		d.window.SetMinSize(min(850, area.Width), min(620, area.Height))
		d.window.SetBounds(bounds)
	}
	d.window.Focus()
	d.visibility(true)
}

func (d *desktop) visibility(visible bool) {
	if d.visible.Swap(visible) != visible {
		d.window.EmitEvent("desktop:visibility", visible)
	}
}

func (d *desktop) saveGeometry() {
	if d.geometry.Bounds.Width == 0 {
		return
	}
	data, err := json.Marshal(d.geometry)
	if err == nil {
		err = platform.AtomicWrite(filepath.Join(d.dir, "window.json"), data)
	}
	if err != nil && !d.geometryWarning {
		d.geometryWarning = true
		go platform.Alert(i18n.Text(core.Language(d.service), "Could not save window position. Your drives are unaffected."))
	}
}

func (d *desktop) captureGeometry() {
	if !d.geometryReady || !d.window.IsVisible() || d.window.IsMinimised() {
		return
	}
	d.geometry.Maximised = d.window.IsMaximised()
	if !d.geometry.Maximised {
		d.geometry.Bounds = d.window.Bounds()
	}
	if d.geometryTimer != nil {
		d.geometryTimer.Stop()
	}
	d.geometryTimer = time.AfterFunc(500*time.Millisecond, func() {
		application.InvokeAsync(func() {
			if !d.stopped.Load() {
				d.saveGeometry()
			}
		})
	})
}

func (d *desktop) action(key string, action func() error) {
	if _, loaded := d.actions.LoadOrStore(key, true); loaded {
		return
	}
	go func() {
		defer d.actions.Delete(key)
		if err := action(); err != nil {
			application.InvokeAsync(d.show)
			platform.Alert(i18n.Error(core.Language(d.service), err))
		}
		if !d.stopped.Load() {
			d.service.Snapshot()
		}
	}()
}

func (d *desktop) openMenu() {
	// Windows can deliver another tray notification while the native popup is tracking.
	if d.menuOpen || d.stopped.Load() {
		return
	}
	d.menuOpen = true
	defer func() { d.menuOpen = false }()
	if d.menu != nil {
		d.menu.Destroy()
	}
	view := d.latest()
	language := view.Settings.Language
	t := func(s string) string { return i18n.Text(language, s) }
	menu := application.NewMenu()
	menu.Add(t("Open SDrive")).OnClick(func(*application.Context) { d.show() })
	status := statusForTray(view)
	menu.Add(fmt.Sprintf("%s: %d · %s", t("Connected drives"), status.Connected, t(status.State))).SetEnabled(false)
	menu.AddSeparator()
	_, exiting := d.actions.Load("exit")
	for _, c := range view.Connections {
		_, working := d.actions.Load(c.ID)
		busy := c.Busy || working || exiting
		target := c.Letter + ":"
		if c.MountFolder != "" {
			target = c.MountFolder
		}
		submenu := menu.AddSubmenu(strings.ReplaceAll(c.Name+" ("+target+")", "&", "&&"))
		submenu.Add(t("Open in Explorer")).SetEnabled(c.Mounted && !busy).OnClick(func(*application.Context) { d.action(c.ID, func() error { return d.service.OpenExplorer(c.ID) }) })
		label := "Connect"
		if c.Mounted {
			label = "Disconnect"
		}
		submenu.Add(t(label)).SetEnabled(!busy && view.SettingsError == "" && view.EngineError == "" && (c.Mounted || !view.RecoveryHold || c.Recovery)).OnClick(func(*application.Context) {
			d.action(c.ID, func() error {
				if c.Mounted {
					return d.service.Unmount(c.ID)
				}
				return d.service.Mount(c.ID)
			})
		})
	}
	menu.AddSeparator()
	menu.Add(t("Exit safely")).SetEnabled(!exiting).OnClick(func(*application.Context) { d.action("exit", d.service.Exit) })
	d.menu = menu
	d.tray.SetMenu(menu)
	d.tray.OpenMenu()
}

func (d *desktop) updateTray() {
	if d.stopped.Load() {
		return
	}
	view := d.latest()
	status := statusForTray(view)
	if d.iconState != status.State {
		colors := map[string]color.RGBA{"Disconnected": {128, 140, 145, 255}, "Connected": {33, 185, 147, 255}, "Uploading or working": {65, 150, 230, 255}, "Recovery or unknown status": {235, 175, 40, 255}, "Attention required": {225, 75, 75, 255}}
		d.tray.SetIcon(trayIconColor(colors[status.State]))
		d.iconState = status.State
	}
	d.tray.SetTooltip(fmt.Sprintf("SDrive · %s: %d · %s", i18n.Text(view.Settings.Language, "Connected drives"), status.Connected, i18n.Text(view.Settings.Language, status.State)))
	alert := newTrayProblem(d.problems, status.Problems)
	d.problems = status.Problems
	if alert && !d.visible.Load() {
		d.show()
	}
}

func setupDesktop(app *application.App, window *application.WebviewWindow, service *core.Service, dir, version string, hidden bool) *desktop {
	d := &desktop{app: app, window: window, service: service, dir: dir, wake: make(chan struct{}, 1), done: make(chan struct{}), ready: make(chan struct{})}
	d.visible.Store(!hidden)
	core.SetDesktopStatus(service, d.visible.Load, d.accept, version)
	core.SetLanguageChanged(service, func(string) { go service.Snapshot() })
	d.tray = app.SystemTray.New()
	d.tray.SetIcon(trayIcon())
	d.tray.SetTooltip("SDrive")
	d.tray.OnClick(func() { application.InvokeAsync(d.show) })
	d.tray.OnRightClick(func() { application.InvokeAsync(d.openMenu) })
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		application.InvokeAsync(func() {
			if d.stopped.Load() {
				return
			}
			d.captureGeometry()
			d.saveGeometry()
			window.Hide()
			d.visibility(false)
		})
	})
	for _, event := range []events.WindowEventType{events.Common.WindowDidMove, events.Common.WindowDidResize, events.Common.WindowMaximise, events.Common.WindowUnMaximise} {
		window.OnWindowEvent(event, func(*application.WindowEvent) {
			application.InvokeAsync(func() {
				if !d.stopped.Load() {
					d.captureGeometry()
				}
			})
		})
	}
	for _, event := range []events.WindowEventType{events.Common.WindowShow, events.Common.WindowHide, events.Common.WindowMinimise, events.Common.WindowUnMinimise} {
		window.OnWindowEvent(event, func(*application.WindowEvent) {
			application.InvokeAsync(func() {
				if !d.stopped.Load() {
					d.visibility(window.IsVisible() && !window.IsMinimised())
				}
			})
		})
	}
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		application.InvokeAsync(func() {
			data, _ := os.ReadFile(filepath.Join(dir, "window.json"))
			if json.Unmarshal(data, &d.geometry) != nil {
				d.geometry = windowGeometry{}
			}
			bounds, area := clampGeometry(d.geometry.Bounds, app.Screen.GetAll())
			d.geometry.Bounds = bounds
			window.SetMinSize(min(850, area.Width), min(620, area.Height))
			window.SetBounds(bounds)
			if d.geometry.Maximised {
				window.Maximise()
				if hidden {
					window.Hide()
				}
			}
			d.geometryReady = true
			close(d.ready)
		})
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			d.service.Snapshot()
			for {
				select {
				case <-d.done:
					return
				case <-d.wake:
					application.InvokeAsync(d.updateTray)
				case <-ticker.C:
					if !d.visible.Load() {
						d.service.Snapshot()
					}
				}
			}
		}()
	})
	app.OnShutdown(func() {
		d.stopped.Store(true)
		close(d.done)
		if d.geometryTimer != nil {
			d.geometryTimer.Stop()
		}
		d.saveGeometry()
	})
	return d
}
