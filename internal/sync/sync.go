// Package sync brings a server folder in line with a file manifest. It plans
// first, so the launcher can show what will change and how much it will
// download, then applies the plan.
//
// Files are never written in place. Each download goes to a temporary file
// that is checked and then renamed over the old one, so an interrupted update
// never leaves a half-written file, and a hard link in the folder (see
// install) is replaced rather than written through. A temporary file left by
// an interrupted download is carried on from next time, rather than
// downloaded again.
package sync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/diablo2org/launcher/internal/fetch"
	"github.com/diablo2org/launcher/internal/paths"
	"github.com/diablo2org/launcher/internal/spec"
)

// Kind is what an action does.
type Kind string

const (
	Download Kind = "download"
	Delete   Kind = "delete"
)

// Action is one change to the server folder.
type Action struct {
	Kind   Kind      `json:"kind"`
	File   spec.File `json:"file"`
	Reason string    `json:"reason"`
}

// Plan is everything needed to bring a folder up to date.
type Plan struct {
	Actions []Action `json:"actions"`
	// Bytes is the total to download.
	Bytes int64 `json:"bytes"`
}

// Cache remembers file hashes between runs, so an unchanged file isn't
// rehashed on every start. Entries are trusted only while size and
// modification time still match.
type Cache map[string]CacheEntry

// CacheEntry is what the cache knows about one file.
type CacheEntry struct {
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
	SHA256  string    `json:"sha256"`
}

func target(dir, rel string) (string, error) {
	if err := paths.Check(rel); err != nil {
		return "", err
	}

	return filepath.Join(dir, filepath.FromSlash(rel)), nil
}

// hash returns the file's SHA-256, using the cache when it's still valid.
// A missing file returns ok false.
func hash(path, rel string, cache Cache) (size int64, sha string, ok bool, err error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, err
	}
	if info.IsDir() {
		return 0, "", false, fmt.Errorf("%s is a folder, expected a file", rel)
	}

	key := paths.Key(rel)
	if e, hit := cache[key]; hit && e.Size == info.Size() && e.ModTime.Equal(info.ModTime()) {
		return e.Size, e.SHA256, true, nil
	}

	size, sha, err = fetch.HashFile(path)
	if err != nil {
		return 0, "", false, err
	}

	if cache != nil && settled(info.ModTime()) {
		cache[key] = CacheEntry{Size: size, ModTime: info.ModTime(), SHA256: sha}
	}

	return size, sha, true, nil
}

// racyWindow is how recently modified a file can be and still have its hash
// cached. File times are only as fine as the system clock (about 15ms on
// Windows), so a same-size edit moments after hashing could keep the same
// modification time; such files are hashed again next time instead.
const racyWindow = 2 * time.Second

func settled(modTime time.Time) bool {
	return time.Since(modTime) > racyWindow
}

// PlanManifest compares the folder with the manifest.
func PlanManifest(dir string, m *spec.Manifest, cache Cache) (*Plan, error) {
	plan := &Plan{Actions: []Action{}}

	for _, f := range m.Files {
		path, err := target(dir, f.Path)
		if err != nil {
			return nil, err
		}

		size, sha, exists, err := hash(path, f.Path, cache)
		if err != nil {
			return nil, err
		}

		switch f.FileMode() {
		case spec.ModeDelete:
			if exists {
				plan.Actions = append(plan.Actions, Action{Kind: Delete, File: f, Reason: "no longer used"})
			}

		case spec.ModeOnce:
			// Once installed, the player owns this file.
			if !exists {
				plan.add(f, "missing")
			}

		default:
			switch {
			case !exists:
				plan.add(f, "missing")
			case size != f.Size || sha != f.SHA256:
				plan.add(f, "changed")
			}
		}
	}

	return plan, nil
}

func (p *Plan) add(f spec.File, reason string) {
	p.Actions = append(p.Actions, Action{Kind: Download, File: f, Reason: reason})
	p.Bytes += f.Size
}

// PlanRemoval lists the files a component version installed, for turning it
// off or switching version. Files the player owns ("once") are kept.
func PlanRemoval(dir string, m *spec.Manifest) (*Plan, error) {
	plan := &Plan{Actions: []Action{}}

	for _, f := range m.Files {
		if f.FileMode() != spec.ModeReplace {
			continue
		}

		path, err := target(dir, f.Path)
		if err != nil {
			return nil, err
		}

		if _, err := os.Stat(path); err == nil {
			plan.Actions = append(plan.Actions, Action{Kind: Delete, File: f, Reason: "component removed"})
		}
	}

	return plan, nil
}

// PlanMatchingRemoval is the cautious form of PlanRemoval, for when the
// launcher has no record of installing a component but its files may be
// there, for example from an older layout. Only files that match the
// manifest exactly, by size and SHA-256, are removed.
func PlanMatchingRemoval(dir string, m *spec.Manifest, cache Cache) (*Plan, error) {
	plan := &Plan{Actions: []Action{}}

	for _, f := range m.Files {
		if f.FileMode() != spec.ModeReplace {
			continue
		}

		path, err := target(dir, f.Path)
		if err != nil {
			return nil, err
		}

		size, sha, exists, err := hash(path, f.Path, cache)
		if err != nil {
			return nil, err
		}

		if exists && size == f.Size && sha == f.SHA256 {
			plan.Actions = append(plan.Actions, Action{Kind: Delete, File: f, Reason: "component turned off"})
		}
	}

	return plan, nil
}

// Progress reports how far an apply has got.
type Progress struct {
	File       string `json:"file"`
	Done       int64  `json:"done"`
	Total      int64  `json:"total"`
	FilesDone  int    `json:"filesDone"`
	FilesTotal int    `json:"filesTotal"`
}

// ErrInUse means a file couldn't be replaced because something, usually the
// game, has it open.
var ErrInUse = errors.New("file is in use; close the game and try again")

// Apply carries out a plan. It stops at the first failure; files already
// replaced stay replaced, and running it again picks up where it left off.
func Apply(ctx context.Context, c *fetch.Client, dir string, plan *Plan, cache Cache, progress func(Progress)) error {
	var done int64

	for i, a := range plan.Actions {
		if err := ctx.Err(); err != nil {
			return err
		}

		path, err := target(dir, a.File.Path)
		if err != nil {
			return err
		}

		report := func(n int64) {
			if progress != nil {
				progress(Progress{File: a.File.Path, Done: done + n, Total: plan.Bytes, FilesDone: i, FilesTotal: len(plan.Actions)})
			}
		}

		switch a.Kind {
		case Delete:
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return inUse(a.File.Path, err)
			}
			delete(cache, paths.Key(a.File.Path))

		case Download:
			if err := download(ctx, c, path, a.File, report); err != nil {
				return err
			}
			done += a.File.Size

			if cache != nil {
				if info, err := os.Stat(path); err == nil && settled(info.ModTime()) {
					cache[paths.Key(a.File.Path)] = CacheEntry{Size: a.File.Size, ModTime: info.ModTime(), SHA256: a.File.SHA256}
				}
			}
		}

		report(0)
	}

	if progress != nil {
		progress(Progress{Done: plan.Bytes, Total: plan.Bytes, FilesDone: len(plan.Actions), FilesTotal: len(plan.Actions)})
	}

	return nil
}

// partialSuffix names a download in progress, beside the file it will
// replace.
const partialSuffix = ".launcher-download"

// tries is how many times one URL is tried, when its failures are worth
// retrying, before moving on to the next mirror. The wait between tries
// starts at retryWait and doubles.
var (
	tries     = 3
	retryWait = time.Second
)

// download fetches a file to a temporary name beside its target, trying each
// mirror in turn, then renames it into place. What arrived of a download that
// stopped part way, on an earlier try or an earlier run, is kept in the
// temporary file and carried on from, from any mirror, since they all serve
// the same bytes.
func download(ctx context.Context, c *fetch.Client, path string, f spec.File, report func(int64)) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	tmp := path + partialSuffix

	var errs []error
	for _, u := range f.URLs {
		wait := retryWait

		for try := 1; ; try++ {
			err := c.Resume(ctx, u, tmp, f.Size, f.SHA256, report)
			if err == nil {
				// A file in use keeps the finished download, so trying
				// again once the game is closed needs no download.
				if err := os.Rename(tmp, path); err != nil {
					return inUse(f.Path, err)
				}
				return nil
			}

			if try == tries || !fetch.Retryable(err) || ctx.Err() != nil {
				errs = append(errs, err)
				break
			}

			if err := sleep(ctx, wait); err != nil {
				errs = append(errs, err)
				break
			}
			wait *= 2
		}

		if ctx.Err() != nil {
			break
		}
	}

	// Stopped by the player: say so, rather than only the last server error.
	joined := errors.Join(errs...)
	if err := ctx.Err(); err != nil && !errors.Is(joined, err) {
		joined = errors.Join(err, joined)
	}

	return fmt.Errorf("%s: %w", f.Path, joined)
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func inUse(rel string, err error) error {
	if errors.Is(err, os.ErrPermission) || isSharingViolation(err) {
		return fmt.Errorf("%s: %w", rel, ErrInUse)
	}

	return fmt.Errorf("%s: %w", rel, err)
}
