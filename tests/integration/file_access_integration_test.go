//go:build windows

package integration

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"sdrive/internal/core"
	"sdrive/internal/platform"
)

func TestMountedFileHolder(t *testing.T) {
	path := os.Getenv("SDRIVE_FILE_HOLDER_PATH")
	if path == "" {
		t.Skip("isolated file-holder child only")
	}
	flag := os.O_RDONLY
	if os.Getenv("SDRIVE_FILE_HOLDER_WRITE") == "1" {
		flag = os.O_RDWR
	}
	file, err := os.OpenFile(path, flag, 0600)
	if err != nil {
		t.Fatal("could not open disposable file")
	}
	defer file.Close()
	if flag == os.O_RDWR {
		if _, err = file.WriteAt([]byte("disposable"), 0); err != nil {
			t.Fatal("disposable write failed")
		}
	} else {
		buffer := make([]byte, 1)
		if _, err = file.Read(buffer); err != nil {
			t.Fatal("disposable read failed")
		}
	}
	fmt.Println("FILE_READY")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}

func startMountedFileHolder(t *testing.T, path string, write bool) (uint32, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMountedFileHolder$")
	mode := "0"
	if write {
		mode = "1"
	}
	cmd.Env = append(os.Environ(), "SDRIVE_FILE_HOLDER_PATH="+path, "SDRIVE_FILE_HOLDER_WRITE="+mode)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		_, _ = io.WriteString(stdin, "close\n")
		_ = stdin.Close()
		select {
		case err := <-finished:
			if err != nil {
				t.Error("disposable file holder failed")
			}
		case <-time.After(3 * time.Second):
			cancel()
			<-finished
			t.Error("disposable file holder did not exit")
		}
		cancel()
	}
	t.Cleanup(stop)
	ready := make(chan bool, 1)
	go func() { scanner := bufio.NewScanner(stdout); ready <- scanner.Scan() && scanner.Text() == "FILE_READY" }()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("disposable file holder did not become ready")
		}
	case <-ctx.Done():
		t.Fatal("disposable file holder startup timed out")
	}
	return uint32(cmd.Process.Pid), stop
}

func TestWindowsMountedFileProcessDiscovery(t *testing.T) {
	requireNativeMount(t)
	f := newIntegrationFixture(t, true)
	s, _ := openIntegrationService(t, f.profile, nil)
	c := configureIntegration(t, s, f.url, f.letter)
	isolatedConnectionWorkers(t, s, f.profile)
	if err := os.WriteFile(filepath.Join(f.destination, "library", "process.txt"), []byte("disposable process probe"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Mount(c.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Unmount(c.ID) })
	path := c.Letter + `:\process.txt`
	file, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := platform.FileProcesses(ctx, path)
	found := false
	for _, p := range result.Processes {
		if p.PID == uint32(os.Getpid()) {
			found = true
		}
	}
	if result.State != "reported" || !found {
		t.Fatalf("mounted file process discovery is not useful: state=%s processes=%d holderFound=%v", result.State, len(result.Processes), found)
	}
	t.Log("Restart Manager identified the process holding a disposable WinFsp file")
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	core.SetFileAccessPicker(s, func(string) (string, error) { return path, nil })
	for _, write := range []bool{false, true} {
		pid, stop := startMountedFileHolder(t, path, write)
		view, err := s.InspectFileAccess(c.ID, "")
		if err != nil || view.State != "reported" || view.Path != "process.txt" {
			t.Fatalf("service file access query failed: %v", err)
		}
		found := false
		for _, p := range view.Processes {
			if p.PID == pid {
				found = true
			}
		}
		if !found {
			t.Fatalf("read/write child not identified (write=%v)", write)
		}
		stop()
		view, err = s.InspectFileAccess(c.ID, "process.txt")
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range view.Processes {
			if p.PID == pid {
				t.Fatal("exited process remained in the next query")
			}
		}
	}
	t.Log("Service queries identified independent reading/writing processes and removed exited holders")
}
