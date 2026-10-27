package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sdrive/internal/i18n"
	"sdrive/internal/platform"
)

type MountTarget struct {
	Letter string `json:"letter"`
	Folder string `json:"folder"`
	Mode   string `json:"mode"`
}

func mountTarget(c Connection) MountTarget { return MountTarget{c.Letter, c.MountFolder, c.MountMode} }
func (t MountTarget) apply(c *Connection) {
	c.Letter, c.MountFolder, c.MountMode = t.Letter, t.Folder, t.Mode
}
func mountPoint(c Connection) string {
	if c.MountFolder != "" {
		return c.MountFolder
	}
	return c.Letter + ":"
}
func mountKey(point string) string {
	point = strings.TrimRight(strings.ReplaceAll(point, "/", `\`), `\`)
	if len(point) == 2 && point[1] == ':' {
		return strings.ToUpper(point)
	}
	return strings.ToUpper(filepath.Clean(point))
}
func sameMount(a, b Connection) bool { return mountKey(mountPoint(a)) == mountKey(mountPoint(b)) }
func mountTargetsOverlap(a, b Connection) bool {
	return sameMount(a, b) || (a.MountFolder != "" && pathWithin(a.MountFolder, mountPoint(b))) || (b.MountFolder != "" && pathWithin(b.MountFolder, mountPoint(a)))
}
func (s *Service) mountPresent(c Connection) bool {
	if c.MountFolder == "" {
		return s.driveUsed(c.Letter)
	}
	_, err := os.Lstat(c.MountFolder)
	return !os.IsNotExist(err)
}
func pathWithin(path, parent string) bool {
	path, parent = mountKey(path), mountKey(parent)
	return path == parent || strings.HasPrefix(path, parent+`\`)
}
func pathsOverlap(a, b string) bool { return pathWithin(a, b) || pathWithin(b, a) }

func validateMountTarget(c *Connection) error {
	if c.MountMode != "" && c.MountMode != "network" && c.MountMode != "local" {
		return &FieldError{Field: "mountMode", Message: "Choose network drive or local disk."}
	}
	if c.VirtualCapacityGiB < 0 || c.VirtualCapacityGiB > 8388607 {
		return &FieldError{Field: "virtualCapacityGiB", Message: "Virtual capacity must be between 0 and 8388607 GiB."}
	}
	if c.DirectoryCacheSeconds < 0 || c.DirectoryCacheSeconds > 86400 {
		return &FieldError{Field: "directoryCacheSeconds", Message: "Directory cache duration must be between 1 and 86400 seconds."}
	}
	if c.MountFolder != "" {
		path, err := platform.LocalPath(c.MountFolder)
		if err != nil || filepath.Dir(path) == path || c.MountMode != "local" || c.Letter != "" {
			return &FieldError{Field: "mountFolder", Message: "Choose a new local subfolder and local-disk mode, without a drive letter."}
		}
		c.MountFolder = path
		return nil
	}
	c.Letter = strings.ToUpper(strings.TrimSpace(c.Letter))
	if len(c.Letter) != 1 || c.Letter[0] < 'D' || c.Letter[0] > 'Z' {
		return &FieldError{Field: "letter", Message: "choose a letter between D and Z"}
	}
	return nil
}

// Caller holds s.mu. Folder mountpoints must not exist; no directory is removed.
func (s *Service) availableMount(c Connection) error {
	for _, other := range s.store.Connections {
		if other.ID != c.ID && mountTargetsOverlap(c, other) {
			field := "letter"
			if c.MountFolder != "" {
				field = "mountFolder"
			}
			return &FieldError{Field: field, Message: "The mount destination is reserved by another connection."}
		}
	}
	if c.MountFolder == "" {
		if s.driveUsed(c.Letter) {
			return i18n.NewMessage("drive.letter_in_use", map[string]string{"letter": c.Letter})
		}
		return nil
	}
	if pathsOverlap(c.MountFolder, s.dir) || pathsOverlap(c.MountFolder, s.dataPath()) {
		return &FieldError{Field: "mountFolder", Message: "The mount folder must not overlap the profile or cache."}
	}
	if _, err := platform.DirectoryIdentity(filepath.Dir(c.MountFolder)); err != nil {
		return &FieldError{Field: "mountFolder", Message: "Choose an available local parent directory without links."}
	}
	if _, err := os.Lstat(c.MountFolder); !os.IsNotExist(err) {
		return &FieldError{Field: "mountFolder", Message: "The mount folder must not exist. Choose a new subfolder; existing contents will not be removed."}
	}
	return nil
}

func showHiddenFiles(c Connection) bool { return c.ShowHiddenFiles == nil || *c.ShowHiddenFiles }

func mountOptions(c Connection, settings Settings) map[string]any {
	directorySeconds := c.DirectoryCacheSeconds
	if directorySeconds == 0 {
		directorySeconds = 300
	}
	poll := 60 * time.Second
	if directorySeconds <= 60 {
		poll = 0
	}
	maxAge, writeBack := cacheDurations(settings)
	vfs := map[string]any{"CacheMode": "full", "CacheMaxSize": int64(settings.CacheGiB) << 30, "CacheMinFreeSpace": int64(5) << 30, "CacheMaxAge": int64(maxAge), "WriteBack": int64(writeBack), "ReadOnly": c.ReadOnly, "DirCacheTime": int64(time.Duration(directorySeconds) * time.Second), "PollInterval": int64(poll)}
	if c.VirtualCapacityGiB > 0 {
		vfs["DiskSpaceTotalSize"] = c.VirtualCapacityGiB << 30
	}
	// Local disks and folder mounts must not expose Everyone's default rclone permissions.
	if c.MountMode == "local" {
		vfs["FilePerms"], vfs["DirPerms"] = uint32(0600), uint32(0700)
	}
	mount := map[string]any{"NetworkMode": c.MountMode != "local", "VolumeName": c.Name}
	if c.ReadOnly {
		// Deny delete access before WinFsp's cleanup phase can hide a VFS rejection.
		vfs["FilePerms"], vfs["DirPerms"] = uint32(0400), uint32(0500)
		mount["ExtraOptions"] = []string{"FileSecurity=D:P(A;;FRFX;;;OW)"}
	} else if c.MountMode == "local" {
		mount["ExtraOptions"] = []string{"FileSecurity=D:P(A;;FA;;;OW)"}
	}
	if !showHiddenFiles(c) {
		options, _ := mount["ExtraOptions"].([]string)
		mount["ExtraOptions"] = append(options, "dothidden")
	}
	return map[string]any{"fs": remote(c), "mountPoint": mountPoint(c), "mountOpt": mount, "vfsOpt": vfs}
}

func cacheDurations(settings Settings) (time.Duration, time.Duration) {
	age, delay := 24, 5
	if settings.CacheMaxAgeHours != nil {
		age = *settings.CacheMaxAgeHours
	}
	if settings.WriteBackSeconds != nil {
		delay = *settings.WriteBackSeconds
	}
	return time.Duration(age) * time.Hour, time.Duration(delay) * time.Second
}

func validateCacheDurations(settings Settings) error {
	if settings.CacheMaxAgeHours != nil && (*settings.CacheMaxAgeHours < 1 || *settings.CacheMaxAgeHours > 4320) {
		return fmt.Errorf("Cache retention must be between 1 and 4320 hours.")
	}
	if settings.WriteBackSeconds != nil && (*settings.WriteBackSeconds < 0 || *settings.WriteBackSeconds > 3600) {
		return fmt.Errorf("Upload delay must be between 0 and 3600 seconds.")
	}
	return nil
}
