package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"sdrive/internal/core"
)

func TestDisconnectOverlayExpiresSuccessButKeepsFailuresAndActiveDrives(t *testing.T) {
	now := time.Now()
	o := &disconnectOverlay{entries: map[string]disconnectEntry{
		"active": {DisconnectProgress: core.DisconnectProgress{ID: "active", Name: "Private drive", Stage: "waitingUploads"}},
		"failed": {DisconnectProgress: core.DisconnectProgress{ID: "failed", Stage: "failed"}},
		"done":   {DisconnectProgress: core.DisconnectProgress{ID: "done", Stage: "succeeded"}, expires: now.Add(3 * time.Second)},
	}, last: map[string]uint64{"active": 10}}
	if len(o.view(core.AccessStatus{}, now).Rows) != 3 {
		t.Fatal("simultaneous operations lost")
	}
	locked := o.view(core.AccessStatus{Locked: true}, now)
	data, err := json.Marshal(locked)
	if err != nil || strings.Contains(string(data), "Private") || strings.Contains(string(data), "active") {
		t.Fatal("locked overlay exposed connection data", string(data), err)
	}
	if len(o.view(core.AccessStatus{}, now.Add(3*time.Second)).Rows) != 2 {
		t.Fatal("success did not expire after three seconds")
	}
	o.update(core.DisconnectProgress{Sequence: 9, ID: "active", Stage: "succeeded"})
	if o.entries["active"].Stage != "waitingUploads" {
		t.Fatal("stale completion replaced active progress")
	}
	o.dismissCompleted()
	if len(o.entries) != 1 || o.entries["active"].Stage != "waitingUploads" {
		t.Fatal("dismissing results hid an active disconnect")
	}
	if len(o.view(core.AccessStatus{}, now.Add(time.Hour)).Rows) != 1 {
		t.Fatal("active progress timed out")
	}
}

func TestDisconnectOverlayBoundsRemainInsideMonitorWorkArea(t *testing.T) {
	for _, area := range []application.Rect{
		{Width: 1920, Height: 1040}, {X: -1280, Y: -200, Width: 1280, Height: 960}, {Width: 320, Height: 240},
	} {
		bounds := disconnectBounds(area)
		if bounds.X < area.X || bounds.Y < area.Y || bounds.X+bounds.Width > area.X+area.Width || bounds.Y+bounds.Height > area.Y+area.Height {
			t.Fatalf("overlay outside work area: %+v in %+v", bounds, area)
		}
	}
}
