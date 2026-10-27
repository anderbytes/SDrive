package engine

import (
	"os/exec"
	"syscall"
)

type processSupervisor struct{}

func binaryName() string { return "rclone" }
func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
}
func (p *processSupervisor) attach(cmd *exec.Cmd) error { return nil }
func (p *processSupervisor) kill(cmd *exec.Cmd) error {
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
func (p *processSupervisor) close() {}
