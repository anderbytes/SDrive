//go:build windows

package platform

import (
	"context"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

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

var restartManager = windows.NewLazySystemDLL("rstrtmgr.dll")
var rmStartSession = restartManager.NewProc("RmStartSession")
var rmRegisterResources = restartManager.NewProc("RmRegisterResources")
var rmGetList = restartManager.NewProc("RmGetList")
var rmEndSession = restartManager.NewProc("RmEndSession")
var rmCancelCurrentTask = restartManager.NewProc("RmCancelCurrentTask")

// FileProcesses reports what Windows knows about one file, never a drive-wide safety verdict.
func FileProcesses(ctx context.Context, path string) FileAccess {
	result := FileAccess{State: "unknown", Processes: []FileProcess{}, CheckedAt: time.Now().UTC()}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil || ctx.Err() != nil {
		return result
	}
	var session uint32
	var key [33]uint16
	status, _, _ := rmStartSession.Call(uintptr(unsafe.Pointer(&session)), 0, uintptr(unsafe.Pointer(&key[0])))
	if status != 0 {
		return result
	}
	defer rmEndSession.Call(uintptr(session))
	status, _, _ = rmRegisterResources.Call(uintptr(session), 1, uintptr(unsafe.Pointer(&name)), 0, 0, 0, 0)
	if status != 0 || ctx.Err() != nil {
		return result
	}
	done := make(chan struct{})
	cancelled := make(chan struct{})
	go func() {
		defer close(cancelled)
		select {
		case <-ctx.Done():
			rmCancelCurrentTask.Call(uintptr(session))
		case <-done:
		}
	}()
	defer func() { close(done); <-cancelled }()
	type processInfo struct {
		PID                   uint32
		Started               windows.Filetime
		Name                  [256]uint16
		Service               [64]uint16
		Type, Status, Session uint32
		Restartable           int32
	}
	var needed, count, reason uint32
	for attempts := 0; attempts < 3; attempts++ {
		if needed > 256 || ctx.Err() != nil {
			return result
		}
		buffer := make([]processInfo, max(needed, 1))
		count = uint32(len(buffer))
		status, _, _ = rmGetList.Call(uintptr(session), uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&reason)))
		if status == uintptr(windows.ERROR_MORE_DATA) {
			continue
		}
		if status != 0 || count > uint32(len(buffer)) || ctx.Err() != nil {
			return result
		}
		for _, p := range buffer[:count] {
			result.Processes = append(result.Processes, FileProcess{PID: p.PID, Name: windows.UTF16ToString(p.Name[:]), StartedAt: time.Unix(0, p.Started.Nanoseconds()).UTC()})
		}
		result.State = "reported"
		return result
	}
	return result
}
