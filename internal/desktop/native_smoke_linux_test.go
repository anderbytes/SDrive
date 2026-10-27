package desktop

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"sdrive/internal/core"
)

func TestLinuxNativeDesktop(t *testing.T) {
	if os.Getenv("SDRIVE_TEST_UI") != "1" {
		t.Skip("set SDRIVE_TEST_UI=1 for native WebKit desktop checks")
	}
	for _, tray := range []string{"0", "1"} {
		t.Run("tray="+tray, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "dbus-run-session", "--", os.Args[0], "--linux-native-smoke")
			cmd.Env = append(os.Environ(), "SDRIVE_NATIVE_PROFILE="+t.TempDir(), "SDRIVE_NATIVE_TRAY="+tray)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("native Linux desktop failed: %v\n%s", err, output)
			}
		})
	}
}

func nativeSmokeCommand() bool {
	if len(os.Args) != 2 || os.Args[1] != "--linux-native-smoke" {
		return false
	}
	if err := exerciseLinuxDesktop(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return true
}
func exerciseLinuxDesktop() error {
	profile := os.Getenv("SDRIVE_NATIVE_PROFILE")
	if profile == "" {
		return fmt.Errorf("isolated profile missing")
	}
	var tray *nativeTrayHost
	if os.Getenv("SDRIVE_NATIVE_TRAY") == "1" {
		var err error
		tray, err = newNativeTrayHost()
		if err != nil {
			return err
		}
		defer tray.conn.Close()
	}
	app := application.New(application.Options{Name: "SDrive native test", Linux: application.LinuxOptions{ApplicationID: "org.sdrive.SDrive"}, LogLevel: slog.LevelError, Assets: application.AssetOptions{Handler: application.BundledAssetFileServer(os.DirFS("frontend/dist"))}})
	service, err := core.New(profile, nil, "")
	if err != nil {
		return err
	}
	core.SetAutoStart(service, func(bool) error { return nil })
	app.RegisterService(application.NewService(service))
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{Name: "sdrive-native-test", Title: "SDrive native test", Width: 1100, Height: 800})
	setupDesktop(app, window, service, profile, "test", false)
	result := make(chan error, 1)
	app.Event.On("linux-smoke:result", func(event *application.CustomEvent) {
		value, ok := event.Data.(string)
		if !ok {
			return
		}
		var err error
		if value != "ok" {
			err = fmt.Errorf("native probe: %s", value)
		}
		if err == nil && tray != nil {
			err = tray.verifyMenu()
		}
		select {
		case result <- err:
		default:
		}
		app.Quit()
	})
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		window.ExecJS(`(async()=>{
   const request=async(object,args)=>{const response=await fetch('/wails/runtime',{method:'POST',headers:{'Content-Type':'application/json','x-wails-client-id':window._wails.clientId},body:JSON.stringify({object,method:0,args})});if(!response.ok)throw Error(await response.text());return object===0?response.json():null;};
   let result='ok';try{
    const view=await request(0,{methodName:'sdrive/internal/core.Service.Snapshot',args:[],'call-id':'linux-native-snapshot'});
    if(view.platform!=='linux'||view.letters.length||!view.mountAvailable)throw Error('Linux bridge capabilities unavailable');
    for(let attempt=0;attempt<100&&!document.querySelector('h1');attempt++)await new Promise(r=>setTimeout(r,50));
    if(!document.body.innerText.includes('My drives')||document.body.innerText.includes('No drive letters'))throw Error('Linux main view did not render');
    let button;for(let attempt=0;attempt<100;attempt++){button=[...document.querySelectorAll('button')].find(b=>b.textContent.includes('New connection'));if(button&&!button.disabled)break;await new Promise(r=>setTimeout(r,50));}
    if(!button||button.disabled)throw Error('New connection unavailable');button.click();
    for(let attempt=0;attempt<100&&!document.querySelector('dialog[open]');attempt++)await new Promise(r=>setTimeout(r,50));
    if(!document.querySelector('dialog[open]'))throw Error('Connection dialog did not open');
    if(document.body.innerText.includes('Windows drive letter'))throw Error('Windows destination leaked into Linux');
   }catch(error){result=String(error);}
   await request(3,{name:'linux-smoke:result',data:result});
  })()`)
	})
	timeout := time.AfterFunc(55*time.Second, func() {
		select {
		case result <- fmt.Errorf("native WebKit did not complete the probe"):
		default:
		}
		application.InvokeAsync(app.Quit)
	})
	defer timeout.Stop()
	if err := app.Run(); err != nil {
		return err
	}
	select {
	case err := <-result:
		return err
	default:
		return fmt.Errorf("native desktop exited without results: %s", strings.TrimSpace(os.Getenv("GDK_BACKEND")))
	}
}

type nativeTrayHost struct {
	conn  *dbus.Conn
	items chan string
}

func (h *nativeTrayHost) RegisterStatusNotifierItem(sender dbus.Sender, item string) *dbus.Error {
	select {
	case h.items <- string(sender):
	default:
	}
	return nil
}

func newNativeTrayHost() (*nativeTrayHost, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	host := &nativeTrayHost{conn: conn, items: make(chan string, 1)}
	if err = conn.Export(host, "/StatusNotifierWatcher", "org.kde.StatusNotifierWatcher"); err != nil {
		conn.Close()
		return nil, err
	}
	reply, err := conn.RequestName("org.kde.StatusNotifierWatcher", dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		conn.Close()
		return nil, fmt.Errorf("isolated tray host unavailable: %v", err)
	}
	return host, nil
}

func (h *nativeTrayHost) verifyMenu() error {
	select {
	case name := <-h.items:
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		call := h.conn.Object(name, "/StatusNotifierMenu").CallWithContext(ctx, "com.canonical.dbusmenu.GetLayout", 0, int32(0), int32(-1), []string{})
		if call.Err != nil {
			return call.Err
		}
		layout := fmt.Sprint(call.Body)
		if !strings.Contains(layout, "Open SDrive") || !strings.Contains(layout, "Exit safely") {
			return fmt.Errorf("native tray menu was not published")
		}
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("tray item did not register")
	}
}
