package engine

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func processProbeCommand() bool {
	if len(os.Args) != 2 || os.Args[1] != "--worker-parent-probe" {
		return false
	}
	cmd := exec.Command("sleep", "60")
	configureCommand(cmd)
	if err := cmd.Start(); err != nil {
		os.Exit(1)
	}
	fmt.Println(cmd.Process.Pid)
	_ = cmd.Wait()
	return true
}
func TestLinuxWorkerStopsWhenParentDies(t *testing.T) {
	parent := exec.Command(os.Args[0], "--worker-parent-probe")
	output, err := parent.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = parent.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { parent.Process.Kill(); parent.Wait() }()
	line := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(output)
		if scanner.Scan() {
			line <- scanner.Text()
		}
	}()
	var pid int
	select {
	case value := <-line:
		pid, err = strconv.Atoi(value)
		if err != nil || pid <= 0 {
			t.Fatal("invalid child process")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not start")
	}
	defer syscall.Kill(pid, syscall.SIGKILL)
	if err = parent.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if os.IsNotExist(err) {
			return
		}
		if err == nil {
			end := strings.LastIndex(string(data), ") ")
			if end >= 0 && strings.HasPrefix(string(data)[end+2:], "Z ") {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("worker survived the parent")
}
