package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"time"

	"sdrive/internal/platform"
)

const metadataLimit = 8 << 20

type UploadView struct {
	Active      int `json:"active"`
	Queued      int `json:"queued"`
	CacheErrors int `json:"cacheErrors"`
}

type CacheView struct {
	CheckedAt    *time.Time `json:"checkedAt"`
	PendingBytes *int64     `json:"pendingBytes"`
	UsedBytes    *uint64    `json:"usedBytes"`
	Error        string     `json:"error"`
}

type localCache struct {
	CacheView
	pending int
	err     error
}

type cacheInspection struct {
	connection Connection
	at         time.Time
	complete   bool
	result     localCache
}

// shortcut: these fields follow rclone v1.75.2; revalidate on engine upgrades.
func readCacheStats(ctx context.Context, rpc RPC, c Connection) (*UploadView, *uint64, error) {
	var response struct {
		DiskCache *struct {
			UploadsInProgress *int
			UploadsQueued     *int
			ErroredFiles      *int
			Files             *int
			OutOfSpace        *bool
			BytesUsed         *int64
		}
	}
	if rpc == nil {
		return nil, nil, fmt.Errorf("could not check pending uploads; engine unavailable")
	}
	if err := rpc.Call(ctx, "vfs/stats", map[string]string{"fs": remote(c)}, &response); err != nil {
		return nil, nil, fmt.Errorf("could not check pending uploads; preserve the cache")
	}
	d := response.DiskCache
	if d == nil || d.UploadsInProgress == nil || d.UploadsQueued == nil || d.ErroredFiles == nil || d.Files == nil || d.OutOfSpace == nil || d.BytesUsed == nil {
		return nil, nil, fmt.Errorf("rclone did not confirm cache state; preserve the connection")
	}
	if *d.UploadsInProgress < 0 || *d.UploadsQueued < 0 || *d.ErroredFiles < 0 || *d.Files < 0 || *d.BytesUsed < 0 || *d.UploadsInProgress > math.MaxInt-*d.UploadsQueued {
		return nil, nil, fmt.Errorf("invalid cache statistics; preserve the connection")
	}
	uploads := &UploadView{Active: *d.UploadsInProgress, Queued: *d.UploadsQueued, CacheErrors: *d.ErroredFiles}
	used := uint64(*d.BytesUsed)
	if *d.OutOfSpace {
		return uploads, &used, fmt.Errorf("cache disk is full; preserve the cache and free up space")
	}
	return uploads, &used, nil
}

// Each directory is read in batches so cancellation does not require loading its entire listing.
func inspectFiles(ctx context.Context, root string, visit func(string, fs.FileInfo) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unexpected cache directory")
	}
	dir, err := os.Open(root)
	if err != nil {
		return err
	}
	defer dir.Close()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, readErr := dir.ReadDir(128)
		if readErr != nil && readErr != io.EOF {
			return readErr
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			path := filepath.Join(root, entry.Name())
			if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
				return fmt.Errorf("unexpected cache entry")
			}
			if info.IsDir() {
				err = inspectFiles(ctx, path, visit)
			} else {
				err = visit(path, info)
			}
			if err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			return ctx.Err()
		}
	}
}

func readDirty(ctx context.Context, path string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, metadataLimit+1))
	if err != nil || len(b) > metadataLimit {
		return false, fmt.Errorf("could not read bounded cache metadata")
	}
	var info struct{ Dirty *bool }
	if err = json.Unmarshal(b, &info); err != nil || info.Dirty == nil {
		return false, fmt.Errorf("cache metadata must contain an explicit Dirty boolean")
	}
	return *info.Dirty, ctx.Err()
}

func (s *Service) inspectCache(ctx context.Context, c Connection) localCache {
	result := localCache{}
	if err := s.cacheReady(); err != nil {
		result.err = err
		result.Error = "Could not inspect local cache; preserve the files and try again."
		return result
	}
	var used uint64
	var pendingBytes int64
	usageKnown, sizeKnown := true, true
	dirtyFiles := map[string]bool{}
	seenDirty := 0
	meta := filepath.Join(s.dataPath(), "vfsMeta", "sd_"+c.Remote)
	data := filepath.Join(s.dataPath(), "vfs", "sd_"+c.Remote)
	// Check ancestors too: a linked cache root must never redirect inspection elsewhere.
	for _, path := range []string{s.dataPath(), filepath.Dir(meta), filepath.Dir(data)} {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			result.err = fmt.Errorf("unexpected cache directory")
			return result
		}
	}
	err := inspectFiles(ctx, meta, func(path string, _ fs.FileInfo) error {
		dirty, err := readDirty(ctx, path)
		if err != nil {
			return err
		}
		if dirty {
			result.pending++
			rel, _ := filepath.Rel(meta, path)
			dirtyFiles[rel] = true
		}
		return nil
	})
	if err == nil {
		err = inspectFiles(ctx, data, func(path string, info fs.FileInfo) error {
			rel, _ := filepath.Rel(data, path)
			// A backing file without metadata cannot establish a clean cache.
			dirty, err := readDirty(ctx, filepath.Join(meta, rel))
			if err != nil {
				return err
			}
			if dirty && !dirtyFiles[rel] {
				result.pending++
				dirtyFiles[rel] = true
			}
			if dirtyFiles[rel] {
				seenDirty++
				if info.Size() < 0 || pendingBytes > math.MaxInt64-info.Size() {
					sizeKnown = false
				} else {
					pendingBytes += info.Size()
				}
			}
			n, err := platform.AllocatedBytes(path)
			if err != nil || used > math.MaxUint64-n {
				usageKnown = false
			} else {
				used += n
			}
			return nil
		})
	}
	result.err = err
	if err != nil {
		result.Error = "Could not inspect local cache; preserve the files and try again."
		return result
	}
	if usageKnown {
		result.UsedBytes = &used
	}
	if sizeKnown && seenDirty == result.pending {
		result.PendingBytes = &pendingBytes
	}
	return result
}

func (s *Service) cachedPending(ctx context.Context, c Connection) (int, error) {
	result := s.inspectCache(ctx, c)
	return result.pending, result.err
}

func (s *Service) sampleCache(ctx context.Context, c Connection) localCache {
	s.mu.Lock()
	i := s.index(c.ID)
	if i < 0 || s.store.Connections[i] != c || s.busy[c.ID] {
		s.mu.Unlock()
		return localCache{err: fmt.Errorf("connection changed or busy")}
	}
	if s.inspections == nil {
		s.inspections = map[string]*cacheInspection{}
	}
	if previous := s.inspections[c.ID]; previous != nil && previous.connection == c && (!previous.complete || time.Since(previous.at) < 30*time.Second) {
		complete, result := previous.complete, previous.result
		s.mu.Unlock()
		if complete {
			return result
		}
		return localCache{err: fmt.Errorf("cache inspection in progress")}
	}
	if ctx.Err() != nil {
		s.mu.Unlock()
		return localCache{err: ctx.Err()}
	}
	inspection := &cacheInspection{connection: c, at: time.Now()}
	s.inspections[c.ID] = inspection
	s.mu.Unlock()
	result := s.inspectCache(ctx, c)
	result.CheckedAt = &inspection.at
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inspections[c.ID] != inspection {
		return localCache{err: fmt.Errorf("cache inspection invalidated")}
	}
	inspection.result, inspection.complete = result, true
	return result
}
