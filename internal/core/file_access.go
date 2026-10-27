package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sdrive/internal/platform"
)

type FileAccessView struct {
	platform.FileAccess
	Path string `json:"path"`
}

func SetFileAccessPicker(s *Service, picker func(string) (string, error)) { s.pickAccessFile = picker }

// InspectFileAccess accepts only a relative file in the currently mounted connection.
// An empty path asks the native picker; neither path is stored in diagnostics.
func (s *Service) InspectFileAccess(id, relative string) (FileAccessView, error) {
	if platform.OS == "linux" {
		return FileAccessView{}, fmt.Errorf("File process inspection is unavailable on Linux.")
	}
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return FileAccessView{}, err
	}
	i := s.index(id)
	if i < 0 || !s.mounted[id] || s.busy[id] || s.fileAccessBusy {
		s.mu.Unlock()
		return FileAccessView{}, fmt.Errorf("Connect the drive and wait for the current file access query.")
	}
	c, epoch := s.store.Connections[i], s.access.epoch
	rpc := s.connectionRPC(c)
	root := mountPoint(c)
	if c.MountFolder == "" {
		root += `\`
	}
	picker := s.pickAccessFile
	s.fileAccessBusy = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.fileAccessBusy = false; s.mu.Unlock() }()
	path := ""
	if relative == "" {
		if picker == nil {
			return FileAccessView{}, fmt.Errorf("Desktop interface unavailable.")
		}
		var err error
		path, err = picker(root)
		if err != nil {
			return FileAccessView{}, fmt.Errorf("Could not select a mounted file.")
		}
		if path == "" {
			return FileAccessView{FileAccess: platform.FileAccess{State: "cancelled", Processes: []platform.FileProcess{}}}, nil
		}
	} else {
		if filepath.IsAbs(relative) || filepath.VolumeName(relative) != "" || len(relative) > 32768 {
			return FileAccessView{}, fmt.Errorf("Select a regular file inside this mounted drive.")
		}
		path = filepath.Join(root, relative)
	}
	path, err := platform.LocalPath(path)
	if err != nil || !pathWithin(path, root) || strings.EqualFold(filepath.Clean(path), filepath.Clean(root)) {
		return FileAccessView{}, fmt.Errorf("Select a regular file inside this mounted drive.")
	}
	relative, err = filepath.Rel(root, path)
	if err != nil {
		return FileAccessView{}, fmt.Errorf("Select a regular file inside this mounted drive.")
	}
	// Check descendants, allowing the mount root itself to be a WinFsp reparse point.
	parent := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		parent = filepath.Join(parent, part)
		info, err := os.Lstat(parent)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 {
			return FileAccessView{}, fmt.Errorf("Select a regular file inside this mounted drive.")
		}
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return FileAccessView{}, fmt.Errorf("Select a regular file inside this mounted drive.")
	}
	if err := s.accessAllowed(); err != nil {
		return FileAccessView{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := platform.FileProcesses(ctx, path)
	s.mu.Lock()
	defer s.mu.Unlock()
	i = s.index(id)
	if s.access.locked || s.access.epoch != epoch || i < 0 || s.store.Connections[i] != c || s.connectionRPC(c) != rpc || !s.mounted[id] || s.busy[id] {
		return FileAccessView{}, fmt.Errorf("File access query changed. Unlock SDrive and select the file again.")
	}
	return FileAccessView{FileAccess: result, Path: relative}, nil
}
