package platform

import (
	"fmt"
	"golang.org/x/sys/windows/registry"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

type Browser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func browserPath(id string) string {
	names := map[string]string{"edge": "msedge.exe", "chrome": "chrome.exe", "firefox": "firefox.exe"}
	exe := names[id]
	if exe == "" {
		return ""
	}
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		key, err := registry.OpenKey(root, `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\`+exe, registry.QUERY_VALUE)
		if err == nil {
			value, _, _ := key.GetStringValue("")
			key.Close()
			value = strings.Trim(value, `"`)
			if info, e := os.Stat(value); e == nil && !info.IsDir() {
				return value
			}
		}
	}
	relative := map[string]string{"edge": `Microsoft\Edge\Application\msedge.exe`, "chrome": `Google\Chrome\Application\chrome.exe`, "firefox": `Mozilla Firefox\firefox.exe`}[id]
	for _, root := range []string{os.Getenv("LOCALAPPDATA"), os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		if root != "" {
			path := filepath.Join(root, relative)
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path
			}
		}
	}
	return ""
}
func InstalledBrowsers() []Browser {
	result := []Browser{{ID: "", Name: "Default browser"}}
	for _, b := range []Browser{{ID: "edge", Name: "Microsoft Edge"}, {ID: "chrome", Name: "Google Chrome"}, {ID: "firefox", Name: "Mozilla Firefox"}} {
		if browserPath(b.ID) != "" {
			result = append(result, b)
		}
	}
	return result
}
func OpenSignIn(id string, private bool, target string) error {
	path := browserPath(id)
	if path == "" {
		return fmt.Errorf("Selected browser is unavailable. Choose another browser.")
	}
	args := []string{}
	if private {
		args = append(args, map[string]string{"edge": "--inprivate", "chrome": "--incognito", "firefox": "-private-window"}[id])
	}
	args = append(args, target)
	cmd := exec.Command(path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Could not launch the selected browser.")
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
