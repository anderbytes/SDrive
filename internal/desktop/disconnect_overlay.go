package desktop

import (
	"sort"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"sdrive/internal/core"
)

const disconnectWindowName = "disconnect-progress"

type disconnectEntry struct {
	core.DisconnectProgress
	expires time.Time
}

type disconnectRow struct {
	Name  string `json:"name"`
	Stage string `json:"stage"`
}

type disconnectView struct {
	Revision uint64          `json:"revision"`
	Language string          `json:"language"`
	Theme    string          `json:"theme"`
	Rows     []disconnectRow `json:"rows"`
}

// All overlay state is owned by the desktop thread.
type disconnectOverlay struct {
	desktop  *desktop
	window   *application.WebviewWindow
	ready    bool
	entries  map[string]disconnectEntry
	last     map[string]uint64
	revision uint64
	timer    *time.Timer
}

func (o *disconnectOverlay) update(progress core.DisconnectProgress) {
	if progress.Sequence <= o.last[progress.ID] {
		return
	}
	o.last[progress.ID] = progress.Sequence
	o.entries[progress.ID] = disconnectEntry{DisconnectProgress: progress}
	if o.window == nil {
		o.window = newDisconnectWindow(o.desktop.app)
		o.window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
			event.Cancel()
			application.InvokeAsync(func() { o.dismissCompleted(); o.render() })
		})
	}
	o.render()
}

func (o *disconnectOverlay) dismissCompleted() {
	for id, entry := range o.entries {
		if entry.Stage == "failed" || entry.Stage == "succeeded" {
			delete(o.entries, id)
		}
	}
}

func (o *disconnectOverlay) view(access core.AccessStatus, now time.Time) disconnectView {
	keys := make([]string, 0, len(o.entries))
	for id, entry := range o.entries {
		if !entry.expires.IsZero() && !now.Before(entry.expires) {
			delete(o.entries, id)
			continue
		}
		keys = append(keys, id)
	}
	sort.Strings(keys)
	o.revision++
	view := disconnectView{Revision: o.revision, Language: access.Language, Theme: access.Theme, Rows: []disconnectRow{}}
	for _, id := range keys {
		entry := o.entries[id]
		name := entry.Name
		if access.Locked {
			name = ""
		}
		view.Rows = append(view.Rows, disconnectRow{Name: name, Stage: entry.Stage})
	}
	return view
}

func disconnectBounds(area application.Rect) application.Rect {
	width, height := min(380, area.Width), min(280, area.Height)
	return application.Rect{X: max(area.X, area.X+area.Width-width-16), Y: max(area.Y, area.Y+area.Height-height-16), Width: width, Height: height}
}

func (o *disconnectOverlay) render() {
	if !o.ready || o.desktop.stopped.Load() {
		return
	}
	if o.timer != nil {
		o.timer.Stop()
		o.timer = nil
	}
	now := time.Now()
	for id, entry := range o.entries {
		if entry.Stage == "succeeded" && entry.expires.IsZero() {
			entry.expires = now.Add(time.Second)
			o.entries[id] = entry
		}
	}
	view := o.view(o.desktop.service.AccessStatus(), now)
	o.window.EmitEvent("disconnect:state", view)
	if len(view.Rows) == 0 {
		o.window.Hide()
		return
	}
	screen, err := o.desktop.window.GetScreen()
	if err != nil || screen == nil {
		for _, candidate := range o.desktop.app.Screen.GetAll() {
			if screen == nil || candidate.IsPrimary {
				screen = candidate
			}
		}
	}
	if screen != nil {
		o.window.SetBounds(disconnectBounds(screen.WorkArea))
	}
	if !o.window.IsVisible() {
		showDisconnectWindow(o.window)
	}
	var next time.Time
	for _, entry := range o.entries {
		if !entry.expires.IsZero() && (next.IsZero() || entry.expires.Before(next)) {
			next = entry.expires
		}
	}
	if !next.IsZero() {
		o.timer = time.AfterFunc(time.Until(next), func() { application.InvokeAsync(o.render) })
	}
}

func setupDisconnectOverlay(d *desktop) *disconnectOverlay {
	o := &disconnectOverlay{desktop: d, entries: map[string]disconnectEntry{}, last: map[string]uint64{}}
	core.SetDisconnectProgress(d.service, func(progress core.DisconnectProgress) {
		application.InvokeAsync(func() {
			if !d.stopped.Load() {
				o.update(progress)
			}
		})
	})
	for _, command := range []string{"ready", "dismiss", "open"} {
		d.app.Event.On("disconnect:"+command, func(event *application.CustomEvent) {
			if event.Sender != disconnectWindowName {
				return
			}
			application.InvokeAsync(func() {
				if d.stopped.Load() || o.window == nil {
					return
				}
				if command == "ready" {
					o.ready = true
				} else {
					o.dismissCompleted()
					if command == "open" {
						d.show()
					}
				}
				o.render()
			})
		})
	}
	return o
}
