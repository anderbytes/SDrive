package desktop

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"
	"golang.org/x/sys/windows"
	"sdrive/internal/core"
	"sdrive/internal/platform"
)

func TestWindowGeometryRemainsOnAvailableMonitor(t *testing.T) {
	primary := &application.Screen{IsPrimary: true, WorkArea: application.Rect{X: 0, Y: 0, Width: 1920, Height: 1040}}
	left := &application.Screen{WorkArea: application.Rect{X: -1280, Y: 0, Width: 1280, Height: 960}}
	for _, test := range []struct {
		name    string
		bounds  application.Rect
		screens []*application.Screen
		want    application.Rect
	}{
		{"missing", application.Rect{}, []*application.Screen{primary}, application.Rect{X: 370, Y: 120, Width: 1180, Height: 800}},
		{"negative monitor", application.Rect{X: -1200, Y: 30, Width: 1000, Height: 700}, []*application.Screen{primary, left}, application.Rect{X: -1200, Y: 30, Width: 1000, Height: 700}},
		{"removed monitor", application.Rect{X: -1200, Y: 30, Width: 1000, Height: 700}, []*application.Screen{primary}, application.Rect{X: 370, Y: 120, Width: 1180, Height: 800}},
		{"partly outside", application.Rect{X: 1800, Y: 1000, Width: 1180, Height: 800}, []*application.Screen{primary}, application.Rect{X: 740, Y: 240, Width: 1180, Height: 800}},
		{"small work area", application.Rect{}, []*application.Screen{{IsPrimary: true, WorkArea: application.Rect{Width: 800, Height: 600}}}, application.Rect{Width: 800, Height: 600}},
		{"invalid width", application.Rect{Width: -1, Height: 800}, []*application.Screen{primary}, application.Rect{X: 370, Y: 120, Width: 1180, Height: 800}},
		{"overflow", application.Rect{X: int(^uint(0) >> 1), Width: int(^uint(0) >> 1), Height: 800}, []*application.Screen{primary}, application.Rect{X: 370, Y: 120, Width: 1180, Height: 800}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, _ := clampGeometry(test.bounds, test.screens)
			if got != test.want {
				t.Fatalf("got %+v; want %+v", got, test.want)
			}
		})
	}
}

func TestActivationReceiverChild(t *testing.T) {
	dir := os.Getenv("SDRIVE_ACTIVATION_TEST_DIR")
	if dir == "" {
		return
	}
	runtime.LockOSThread()
	id, err := platform.InstanceID(dir)
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := platform.SingleInstance(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	application.New(application.Options{Name: "SDrive activation test", LogLevel: slog.LevelError, SingleInstance: &application.SingleInstanceOptions{UniqueID: id, OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
		if len(data.Args) != 0 || data.WorkingDir != "" || len(data.AdditionalData) != 0 {
			fmt.Println("UNEXPECTED PAYLOAD")
			return
		}
		fmt.Println("ACTIVATED")
	}}})
	fmt.Println("READY")
	var message w32.MSG
	for w32.GetMessage(&message, 0, 0, 0) > 0 {
		w32.TranslateMessage(&message)
		w32.DispatchMessage(&message)
	}
}

func TestActivationHandoffBetweenProcessesRetainsOwnership(t *testing.T) {
	dir := t.TempDir()
	id, err := platform.InstanceID(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestActivationReceiverChild$")
	cmd.Env = append(os.Environ(), "SDRIVE_ACTIVATION_TEST_DIR="+dir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() || scanner.Text() != "READY" {
		t.Fatal("activation receiver did not start", scanner.Text(), scanner.Err())
	}
	if unlock, err := platform.SingleInstance(dir); !errors.Is(err, platform.ErrInstanceRunning) {
		if unlock != nil {
			unlock()
		}
		t.Fatal("second owner admitted", err)
	}
	if err := platform.ActivateInstance(id); err != nil {
		t.Fatal(err)
	}
	if !scanner.Scan() || scanner.Text() != "ACTIVATED" {
		t.Fatal("handoff not delivered", scanner.Text(), scanner.Err())
	}
	if unlock, err := platform.SingleInstance(dir); !errors.Is(err, platform.ErrInstanceRunning) {
		if unlock != nil {
			unlock()
		}
		t.Fatal("handoff released primary ownership", err)
	}
}

func TestTrayStatusPreservesRecoveryAndUnknownSemantics(t *testing.T) {
	zero, pending := 0, 2
	for _, test := range []struct {
		name                string
		view                core.View
		state               string
		problems, connected int
	}{
		{"empty", core.View{}, "Disconnected", 0, 0},
		{"normal mounted recovery marker", core.View{Connections: []core.ConnectionView{{Connection: core.Connection{ID: "one", Recovery: true}, Mounted: true, Pending: &zero}}}, "Connected", 0, 1},
		{"uploading", core.View{Connections: []core.ConnectionView{{Mounted: true, Pending: &pending}}}, "Uploading or working", 0, 1},
		{"unknown", core.View{Connections: []core.ConnectionView{{Connection: core.Connection{ID: "one"}, Mounted: true}}}, "Recovery or unknown status", 1, 1},
		{"disconnected recovery", core.View{Connections: []core.ConnectionView{{Connection: core.Connection{ID: "one", Recovery: true}, Pending: &zero}}}, "Recovery or unknown status", 1, 0},
		{"mount in progress", core.View{Connections: []core.ConnectionView{{Busy: true}}}, "Uploading or working", 0, 0},
		{"engine stopped", core.View{EngineError: "stopped"}, "Attention required", 1, 0},
		{"connection error", core.View{Connections: []core.ConnectionView{{Connection: core.Connection{ID: "one"}, Error: "failed", Pending: &zero}}}, "Attention required", 1, 0},
		{"cache errors", core.View{Connections: []core.ConnectionView{{Connection: core.Connection{ID: "one"}, Mounted: true, Pending: &zero, Uploads: &core.UploadView{CacheErrors: 1}}}}, "Attention required", 1, 1},
		{"settings recovery", core.View{RecoveryHold: true}, "Recovery or unknown status", 1, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := statusForTray(test.view)
			if got.State != test.state || len(got.Problems) != test.problems || got.Connected != test.connected {
				t.Fatalf("unexpected status: %+v", got)
			}
		})
	}
}

func TestTrayAlertsOnlyForNewProblems(t *testing.T) {
	problem := map[string]string{"one": "Recovery required"}
	if !newTrayProblem(nil, problem) || newTrayProblem(problem, problem) || newTrayProblem(problem, nil) {
		t.Fatal("repeated problem alerted or first problem suppressed")
	}
	if !newTrayProblem(problem, map[string]string{"one": "failed"}) || !newTrayProblem(nil, problem) {
		t.Fatal("changed or recurrent problem suppressed")
	}
}

func TestDesktopRejectsOlderSnapshotsAndCoalescesUpdates(t *testing.T) {
	d := &desktop{wake: make(chan struct{}, 1)}
	var wg sync.WaitGroup
	for i := uint64(1); i <= 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); d.accept(i, core.View{EngineError: string(rune(i))}) }()
	}
	wg.Wait()
	if d.sequence != 100 || d.latest().EngineError != string(rune(100)) || len(d.wake) != 1 {
		t.Fatal("obsolete desktop state accepted")
	}
}

func TestWindowGeometryPersistenceIsIndependentOfSettings(t *testing.T) {
	dir := t.TempDir()
	settings := []byte("settings must remain unchanged")
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), settings, 0600); err != nil {
		t.Fatal(err)
	}
	d := &desktop{dir: dir, geometry: windowGeometry{Bounds: application.Rect{X: 12, Y: 34, Width: 1000, Height: 700}, Maximised: true}}
	d.saveGeometry()
	var got windowGeometry
	data, err := os.ReadFile(filepath.Join(dir, "window.json"))
	if err != nil || json.Unmarshal(data, &got) != nil || got != d.geometry {
		t.Fatal("geometry not preserved", err)
	}
	data, err = os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil || string(data) != string(settings) {
		t.Fatal("connection settings changed")
	}
}

func TestFailedGeometrySavePreservesPreviousFile(t *testing.T) {
	dir := t.TempDir()
	d := &desktop{dir: dir, geometry: windowGeometry{Bounds: application.Rect{Width: 1000, Height: 700}}}
	d.saveGeometry()
	path := filepath.Join(dir, "window.json")
	previous, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	name, _ := windows.UTF16PtrFromString(path)
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	d.geometry.Bounds.Width = 1200
	d.geometryWarning = true // Suppress the native warning dialog in this failure-path test.
	d.saveGeometry()
	current, err := os.ReadFile(path)
	if err != nil || string(previous) != string(current) {
		t.Fatal("failed replacement damaged previous geometry", err)
	}
}

func TestTrayActionRejectsDuplicateOperation(t *testing.T) {
	d := &desktop{}
	d.stopped.Store(true)
	started, finish, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	d.action("connection", func() error { calls.Add(1); close(started); <-finish; close(done); return nil })
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("action did not start")
	}
	d.action("connection", func() error { calls.Add(1); return nil })
	close(finish)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("action did not finish")
	}
	if calls.Load() != 1 {
		t.Fatal("duplicate action started")
	}
}

func TestTrayRepeatedOpenPreservesActiveMenu(t *testing.T) {
	menu := application.NewMenu()
	defer menu.Destroy()
	exit := menu.Add("Exit safely").OnClick(func(*application.Context) {})
	explorer := menu.AddSubmenu("Drive (S:)").Add("Open in Explorer").OnClick(func(*application.Context) {})
	d := &desktop{menu: menu, menuOpen: true}

	// A second Windows notification arrives inside the first popup's message loop.
	d.openMenu()
	if d.menu != menu || menu.FindByLabel("Exit safely") != exit || menu.FindByLabel("Open in Explorer") != explorer {
		t.Fatal("repeated notification replaced or destroyed the active menu")
	}
}

func TestLockedTrayHidesDriveActionsEvenWithAnOlderSnapshot(t *testing.T) {
	d := &desktop{service: &core.Service{}}
	view := core.View{Settings: core.Settings{Language: "en"}, Connections: []core.ConnectionView{{Connection: core.Connection{ID: "synthetic", Name: "Private drive", Letter: "S"}, Mounted: true}}}
	unlocked := d.trayMenu(view, core.AccessStatus{Enabled: true})
	defer unlocked.Destroy()
	if unlocked.FindByLabel("Private drive (S:)") == nil || unlocked.FindByLabel("Disconnect") == nil || unlocked.FindByLabel("Lock SDrive") == nil {
		t.Fatal("unlocked tray lost drive actions")
	}
	locked := d.trayMenu(view, core.AccessStatus{Enabled: true, Locked: true})
	defer locked.Destroy()
	for _, name := range []string{"Private drive (S:)", "Disconnect", "Open in Explorer", "Lock SDrive"} {
		if locked.FindByLabel(name) != nil {
			t.Fatal("locked tray exposed drive management")
		}
	}
	if locked.FindByLabel("Open SDrive") == nil || locked.FindByLabel("Exit safely") == nil {
		t.Fatal("locked tray lost unlock entry or safe exit")
	}
}
