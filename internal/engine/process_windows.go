package engine

import (
	"golang.org/x/sys/windows"
	"os/exec"
	"syscall"
	"unsafe"
)

type processSupervisor struct{ job windows.Handle }

func binaryName() string             { return "rclone.exe" }
func configureCommand(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
func (p *processSupervisor) attach(cmd *exec.Cmd) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	p.job = job
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if err != nil {
		return err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	return windows.AssignProcessToJobObject(job, process)
}
func (p *processSupervisor) kill(cmd *exec.Cmd) error { return cmd.Process.Kill() }
func (p *processSupervisor) close() {
	if p.job != 0 {
		windows.CloseHandle(p.job)
		p.job = 0
	}
}
