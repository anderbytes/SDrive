//go:build linux

package platform

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"
)

const OS = "linux"
const AccessFile = "application-lock.enc"
const GlobalIdleSupported = false
const DefaultIdleMinutes = 0
const MountType = "mount2"
const CredentialStore = "Secret Service"

var ErrInstanceRunning = errors.New("SDrive already owns this profile. Reopen it in the same desktop session.")

func DataDir() (string, error) {
	root := os.Getenv("XDG_DATA_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".local", "share")
	}
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("XDG_DATA_HOME must be absolute")
	}
	return filepath.Join(root, "SDrive"), nil
}
func ProtectDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || stat.Uid != uint32(os.Getuid()) {
		return fmt.Errorf("The profile must be a directory owned by your user, without links.")
	}
	return os.Chmod(path, 0700)
}
func Replace(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(dst))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
func createFile(src, dst string) error {
	if err := os.Link(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}
func FreeBytes(path string) (uint64, error) {
	var s unix.Statfs_t
	err := unix.Statfs(path, &s)
	return s.Bavail * uint64(s.Bsize), err
}
func AllocatedBytes(path string) (uint64, error) {
	var s unix.Stat_t
	if err := unix.Lstat(path, &s); err != nil {
		return 0, err
	}
	if s.Mode&unix.S_IFMT != unix.S_IFREG || s.Blocks < 0 {
		return 0, fmt.Errorf("invalid cache file allocation")
	}
	return uint64(s.Blocks) * 512, nil
}
func LocalPath(path string) (string, error) {
	if !filepath.IsAbs(path) || strings.ContainsAny(path, "\x00\r\n") {
		return "", fmt.Errorf("Choose an absolute path on a local disk.")
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".." || part == "." {
			return "", fmt.Errorf("Choose an unambiguous local directory name.")
		}
	}
	return filepath.Clean(path), nil
}
func DirectoryIdentity(path string) (string, error) {
	path, err := LocalPath(path)
	if err != nil {
		return "", err
	}
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("The local directory is unavailable or contains a link. Preserve its contents.")
		}
		if current == "/" {
			break
		}
	}
	var stat unix.Stat_t
	if err := unix.Stat(path, &stat); err != nil {
		return "", err
	}
	var disk unix.Statfs_t
	if err := unix.Statfs(path, &disk); err != nil {
		return "", err
	}
	// Recovery and cache ownership require a local filesystem, never a remote/FUSE parent.
	switch disk.Type {
	case unix.NFS_SUPER_MAGIC, unix.CIFS_SUPER_MAGIC, unix.FUSE_SUPER_MAGIC, unix.SMB_SUPER_MAGIC:
		return "", fmt.Errorf("Choose a directory on an available local disk.")
	}
	return fmt.Sprintf("%x:%x", stat.Dev, stat.Ino), nil
}
func SingleInstance(dir string) (func(), error) {
	fd, err := unix.Open(filepath.Join(dir, "instance.lock"), unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		unix.Close(fd)
		return nil, ErrInstanceRunning
	}
	return func() { unix.Flock(fd, unix.LOCK_UN); unix.Close(fd) }, nil
}
func InstanceID(dir string) (string, error) {
	path, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("org.sdrive.profile.p%x", sha256.Sum256([]byte(strconv.Itoa(os.Getuid())+"|"+path))), nil
}
func ActivateInstance(id string) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return conn.Object(id+".SingleInstance", dbus.ObjectPath("/"+strings.ReplaceAll(id, ".", "/")+"/SingleInstance")).CallWithContext(ctx, id+".SingleInstance.SendMessage", 0, `{"args":[],"workingDir":""}`).Err
}
func MountAvailable() bool {
	_, err := exec.LookPath("fusermount3")
	if err != nil {
		return false
	}
	file, err := os.OpenFile("/dev/fuse", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	file.Close()
	return true
}
func DriveUsed(string) bool  { return true }
func WinFspInstalled() bool  { return false }
func WebViewInstalled() bool { return true }
func PrerequisiteStatus() int {
	if MountAvailable() {
		return 0
	}
	return 1
}
func InstallPrerequisite(Prerequisite, string) int { return 1 }
func verifiedPrerequisite(Prerequisite, string) (*os.File, error) {
	return nil, fmt.Errorf("Windows component installation is unavailable on Linux.")
}
func ComponentVersions() (string, string, string) {
	version := "Linux"
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				version = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
			}
		}
	}
	return version, "", ""
}
func Alert(message string) {
	fmt.Fprintln(os.Stderr, message)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, "zenity", "--error", "--title=SDrive", "--text="+message).Run()
}
func InputIdle() (time.Duration, error) {
	return 0, fmt.Errorf("Global inactivity automation is unavailable on Linux.")
}
func PortableValidationTarget() (string, string, string) { return "", "/sdrive-import-target", "local" }
func TrayAvailable() bool {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var owned bool
	return conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.NameHasOwner", 0, "org.kde.StatusNotifierWatcher").Store(&owned) == nil && owned
}

type Browser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func browserPath(id string) string {
	names := map[string][]string{"firefox": {"firefox"}, "chrome": {"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"}, "edge": {"microsoft-edge", "microsoft-edge-stable"}}
	for _, name := range names[id] {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}
func InstalledBrowsers() []Browser {
	result := []Browser{{ID: "", Name: "Default browser"}}
	for _, b := range []Browser{{"firefox", "Mozilla Firefox"}, {"chrome", "Google Chrome / Chromium"}, {"edge", "Microsoft Edge"}} {
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
		args = append(args, map[string]string{"firefox": "-private-window", "chrome": "--incognito", "edge": "--inprivate"}[id])
	}
	args = append(args, target)
	cmd := exec.Command(path, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Could not launch the selected browser.")
	}
	go cmd.Wait()
	return nil
}
func startupPath() (string, error) {
	root, err := os.UserConfigDir()
	return filepath.Join(root, "autostart", "sdrive.desktop"), err
}
func startupContent() ([]byte, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	// Desktop-entry escaping is applied before the Exec argument parser's escaping.
	quoted := strings.NewReplacer(`\`, `\\\\`, `"`, `\\"`, "`", "\\\\`", "$", `\\$`, "%", "%%").Replace(exe)
	if strings.ContainsAny(exe, "\r\n") {
		return nil, fmt.Errorf("invalid executable path")
	}
	return []byte("[Desktop Entry]\nType=Application\nName=SDrive\nExec=\"" + quoted + "\" --background\nTerminal=false\nX-GNOME-Autostart-enabled=true\n"), nil
}
func AutoStart(enabled bool) error {
	path, err := startupPath()
	if err != nil {
		return err
	}
	if !enabled {
		err = os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := startupContent()
	if err != nil {
		return err
	}
	return AtomicWrite(path, data)
}
func AutoStartMatches(enabled bool) (bool, error) {
	path, err := startupPath()
	if err != nil {
		return false, err
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return !enabled, nil
	}
	if err != nil {
		return false, err
	}
	expected, err := startupContent()
	return enabled && string(b) == string(expected), err
}

type DriveIcon struct{ Letter, Provider string }
type DriveLabel struct{ Letter, Name string }

func SyncDriveIcons(string, []DriveIcon) error   { return nil }
func SyncDriveLabels(string, []DriveLabel) error { return nil }
func CleanupDriveIcons() error                   { return nil }
func CleanupDriveLabels() error                  { return nil }

type FileProcess struct {
	PID       uint32    `json:"pid"`
	Name      string    `json:"name"`
	StartedAt time.Time `json:"startedAt"`
}
type FileAccess struct {
	State     string        `json:"state"`
	Processes []FileProcess `json:"processes"`
	CheckedAt time.Time     `json:"checkedAt"`
}

func FileProcesses(context.Context, string) FileAccess {
	return FileAccess{State: "unknown", Processes: []FileProcess{}, CheckedAt: time.Now().UTC()}
}
