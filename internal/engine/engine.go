package engine

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const Version = "v1.75.2"

type Engine struct {
	url, user, password string
	cmd                 *exec.Cmd
	job                 windows.Handle
	done                chan struct{}
	client              *http.Client
	mu                  sync.Mutex
}

func FindBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	for _, path := range []string{filepath.Join(filepath.Dir(exe), "rclone.exe"), filepath.Join(".tools", "rclone", "rclone.exe")} {
		if _, err = os.Stat(path); err == nil {
			return filepath.Abs(path)
		}
	}
	return "", fmt.Errorf("rclone not found; run scripts/bootstrap.ps1 or reinstall SDrive")
}
func randomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func environment(extra ...string) []string {
	result := []string{}
	for _, v := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(v), "RCLONE_") {
			result = append(result, v)
		}
	}
	return append(result, extra...)
}
func quote(value string) string { return `"` + strings.ReplaceAll(value, `"`, `""`) + `"` }

// Init encrypts an empty configuration before any credentials are written.
func Init(binary, dir string) error {
	path := filepath.Join(dir, "rclone.conf")
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		if !bytes.Contains(data, []byte("\nRCLONE_ENCRYPT_V0:")) {
			return fmt.Errorf("unencrypted rclone configuration; it will not be loaded")
		}
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "config", "encryption", "set", "--config", path, "--password-command", quote(exe)+" --config-key "+quote(dir), "--ask-password=false")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Env = environment()
	if err = cmd.Run(); err != nil {
		return fmt.Errorf("could not initialize encrypted configuration: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Contains(data, []byte("\nRCLONE_ENCRYPT_V0:")) {
		return fmt.Errorf("configuration encryption was not confirmed")
	}
	return nil
}

func Start(binary, dir string) (*Engine, error) {
	e := &Engine{user: randomSecret(), password: randomSecret(), done: make(chan struct{}), client: &http.Client{Timeout: 6 * time.Minute, Transport: &http.Transport{Proxy: nil}}}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(binary, "rcd", "--config", filepath.Join(dir, "rclone.conf"), "--cache-dir", filepath.Join(dir, "cache"), "--rc-addr", "127.0.0.1:0", "--use-json-log", "--log-level", "INFO", "--ask-password=false", "--password-command", quote(exe)+" --config-key "+quote(dir))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Env = environment("RCLONE_RC_USER="+e.user, "RCLONE_RC_PASS="+e.password)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdout = io.Discard
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	e.cmd = cmd
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return nil, err
	}
	e.job = job
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if err == nil {
		process, openErr := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
		if openErr != nil {
			err = openErr
		} else {
			err = windows.AssignProcessToJobObject(job, process)
			windows.CloseHandle(process)
		}
	}
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		windows.CloseHandle(job)
		return nil, fmt.Errorf("could not supervise the rclone process: %w", err)
	}
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		for scanner.Scan() {
			var record struct {
				Msg string `json:"msg"`
			}
			if json.Unmarshal(scanner.Bytes(), &record) == nil {
				const marker = "Serving remote control on "
				if strings.HasPrefix(record.Msg, marker) {
					address := strings.TrimSpace(strings.TrimPrefix(record.Msg, marker))
					if strings.HasPrefix(address, "http://127.0.0.1:") {
						select {
						case ready <- strings.TrimRight(address, "/"):
						default:
						}
					}
				}
			}
		}
	}()
	go func() { cmd.Wait(); close(e.done) }()
	select {
	case e.url = <-ready:
		if err = e.checkAuthentication(); err != nil {
			e.Close()
			return nil, err
		}
		return e, nil
	case <-e.done:
		e.Close()
		return nil, fmt.Errorf("rclone exited during startup")
	case <-time.After(30 * time.Second):
		e.Close()
		return nil, fmt.Errorf("rclone did not start the local API in time")
	}
}

func (e *Engine) checkAuthentication() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", e.url+"/config/listremotes", strings.NewReader("{}"))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("could not verify local API protection")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		return fmt.Errorf("local API accepted unauthenticated access; startup blocked")
	}
	return nil
}

func (e *Engine) Alive() bool {
	select {
	case <-e.done:
		return false
	default:
		return true
	}
}
func (e *Engine) Call(ctx context.Context, method string, input any, out any) error {
	if !e.Alive() {
		return fmt.Errorf("rclone engine stopped; restart SDrive to recover your drives")
	}
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", e.url+"/"+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(e.user, e.password)
	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("could not complete the operation in the local engine")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("rclone rejected %s (HTTP %d); check authentication, permissions and destination", method, resp.StatusCode)
	}
	if out == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<20))
		return err
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
}
func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cmd != nil {
		if e.Alive() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = e.Call(ctx, "core/quit", map[string]any{}, nil)
			cancel()
			select {
			case <-e.done:
			case <-time.After(3 * time.Second):
				_ = e.cmd.Process.Kill()
			}
		}
		e.cmd = nil
	}
	if e.job != 0 {
		windows.CloseHandle(e.job)
		e.job = 0
	}
}
