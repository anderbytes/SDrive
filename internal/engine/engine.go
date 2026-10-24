package engine

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"sdrive/internal/diagnostics"
)

const Version = "v1.75.2"

type Engine struct {
	url, user, password string
	cmd                 *exec.Cmd
	job                 windows.Handle
	done                chan struct{}
	client              *http.Client
	mu                  sync.Mutex
	statusMu            sync.Mutex
	diagnostics         *diagnostics.Collector
	lastSignal          string
	lastRPCFailure      string
	exitCode            *int64
	requested, forced   bool
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
		key := strings.ToUpper(strings.SplitN(v, "=", 2)[0])
		if !strings.HasPrefix(key, "RCLONE_") && key != "HTTP_PROXY" && key != "HTTPS_PROXY" && key != "ALL_PROXY" && key != "NO_PROXY" {
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

func Start(binary, dir string, log *diagnostics.Collector) (*Engine, error) {
	return StartWithOptions(binary, dir, log, Options{CacheDir: filepath.Join(dir, "cache")})
}

type Options struct {
	CacheDir                    string
	Memory                      bool
	UploadLimit, DownloadLimit  int64
	ProxyURL                    string
	ConnectTimeout, IdleTimeout int
}

func StartWithOptions(binary, dir string, log *diagnostics.Collector, options Options) (*Engine, error) {
	e := &Engine{diagnostics: log, user: randomSecret(), password: randomSecret(), done: make(chan struct{}), client: &http.Client{Timeout: 6 * time.Minute, Transport: &http.Transport{Proxy: nil}}}
	log.Record("engine_start", "started", "none", nil)
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(dir, "rclone.conf")
	if options.Memory {
		configPath = os.DevNull
	}
	args := []string{"rcd", "--config", configPath, "--cache-dir", options.CacheDir, "--rc-addr", "127.0.0.1:0", "--use-json-log", "--log-level", "INFO", "--ask-password=false", "--password-command", quote(exe) + " --config-key " + quote(dir)}
	if options.UploadLimit != 0 || options.DownloadLimit != 0 {
		args = append(args, "--bwlimit", strconv.FormatInt(options.UploadLimit, 10)+"B:"+strconv.FormatInt(options.DownloadLimit, 10)+"B")
	}
	if options.ConnectTimeout > 0 {
		args = append(args, "--contimeout", strconv.Itoa(options.ConnectTimeout)+"s")
	}
	if options.IdleTimeout > 0 {
		args = append(args, "--timeout", strconv.Itoa(options.IdleTimeout)+"s")
	}
	cmd := exec.Command(binary, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Env = environment("RCLONE_RC_USER="+e.user, "RCLONE_RC_PASS="+e.password)
	if options.ProxyURL != "" {
		cmd.Env = append(cmd.Env, "RCLONE_HTTP_PROXY="+options.ProxyURL)
	}
	cmd.WaitDelay = time.Second
	ready := make(chan string, 1)
	logs := &logWriter{e: e, ready: ready}
	cmd.Stderr = logs
	cmd.Stdout = io.Discard
	if err = cmd.Start(); err != nil {
		log.Operation("engine_start", err)
		return nil, err
	}
	e.cmd = cmd
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, e.supervisionFailure(cmd, logs)
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
		failure := e.supervisionFailure(cmd, logs)
		windows.CloseHandle(job)
		e.job = 0
		return nil, failure
	}
	go e.wait(cmd, logs)
	select {
	case e.url = <-ready:
		if err = e.checkAuthentication(); err != nil {
			log.Operation("engine_auth", err)
			e.Close()
			return nil, err
		}
		log.Operation("engine_auth", nil)
		log.Operation("engine_start", nil)
		return e, nil
	case <-e.done:
		e.Close()
		return nil, fmt.Errorf("%s", e.exitSummary("rclone exited during startup"))
	case <-time.After(30 * time.Second):
		e.Close()
		log.Record("engine_start", "failed", "engine", nil)
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
func (e *Engine) Call(ctx context.Context, method string, input any, out any) (err error) {
	defer func() {
		if err != nil {
			kind := diagnostics.Kind(err)
			e.statusMu.Lock()
			changed := e.lastRPCFailure != kind
			e.lastRPCFailure = kind
			e.statusMu.Unlock()
			if changed {
				e.diagnostics.Operation("engine_rpc", err)
			}
		}
	}()
	if method == "config/create" || method == "config/update" {
		return e.configure(ctx, method, input, out)
	}
	return e.call(ctx, method, input, out)
}

// A finished HTTP request is not proof that a configuration job stopped.
func (e *Engine) configure(ctx context.Context, method string, input, out any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	var params map[string]any
	if err = json.Unmarshal(data, &params); err != nil || params == nil {
		return fmt.Errorf("invalid configuration request")
	}
	group := "sdrive-config-" + randomSecret()
	params["_async"], params["_group"] = true, group
	var job struct {
		ID int64 `json:"jobid"`
	}
	startCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = e.call(startCtx, method, params, &job)
	cancel()
	var rejection *rpcRejection
	if errors.As(err, &rejection) && rejection.status >= 400 && rejection.status < 500 {
		return err
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for e.Alive() {
		pollCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		// Recover the job after a lost start response, without starting another write.
		if job.ID <= 0 {
			var jobs struct {
				IDs []int64 `json:"jobids"`
			}
			if e.call(pollCtx, "job/list", map[string]any{}, &jobs) == nil {
				for _, id := range jobs.IDs {
					var status struct{ Group string }
					if e.call(pollCtx, "job/status", map[string]any{"jobid": id}, &status) == nil && status.Group == group {
						job.ID = id
						break
					}
				}
			}
		}
		if job.ID > 0 {
			args := map[string]any{"jobid": job.ID}
			if ctx.Err() != nil {
				_ = e.call(pollCtx, "job/stop", args, nil)
			}
			var status struct {
				Finished, Success *bool
				Output            json.RawMessage
				Error             string
			}
			if e.call(pollCtx, "job/status", args, &status) == nil && status.Finished != nil && *status.Finished && status.Success != nil {
				cancel()
				if err := ctx.Err(); err != nil {
					return err
				}
				if !*status.Success {
					return providerFailure(status.Error)
				}
				if out == nil {
					return nil
				}
				if len(status.Output) == 0 || status.Output[0] != '{' {
					return operationFailure("invalid")
				}
				return json.Unmarshal(status.Output, out)
			}
		}
		cancel()
		// Unknown completion preserves the setup until proof or engine termination.
		select {
		case <-e.done:
		case <-ticker.C:
		}
	}
	return fmt.Errorf("rclone engine stopped; restart SDrive to recover your drives")
}

type rpcRejection struct {
	method string
	status int
}

func (e *rpcRejection) Error() string {
	return fmt.Sprintf("rclone rejected %s (HTTP %d); check authentication, permissions and destination", e.method, e.status)
}

func (e *Engine) call(ctx context.Context, method string, input any, out any) error {
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
		return operationFailure("engine")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode >= 500 {
			var response struct{ Error string }
			if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&response) == nil {
				return providerFailure(response.Error)
			}
		}
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return &rpcRejection{method: method, status: resp.StatusCode}
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
			e.statusMu.Lock()
			e.requested = true
			e.statusMu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = e.Call(ctx, "core/quit", map[string]any{}, nil)
			cancel()
			select {
			case <-e.done:
			case <-time.After(3 * time.Second):
				e.statusMu.Lock()
				e.forced = true
				e.statusMu.Unlock()
				_ = e.cmd.Process.Kill()
				select {
				case <-e.done:
				case <-time.After(time.Second):
				}
			}
		}
		e.cmd = nil
	}
	if e.job != 0 {
		windows.CloseHandle(e.job)
		e.job = 0
	}
}
