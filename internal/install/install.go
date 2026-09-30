// Package install manages the player's Diablo II base install and the
// per-server folders inside it. The base install is only ever read: the
// launcher creates server folders in it, and nothing else.
package install

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Archive is one of Blizzard's base archives. Servers never ship these; their
// presence in the base folder is how ownership is checked.
type Archive struct {
	Name string
	// Required archives stop the game starting when missing. The others make
	// it ask for a disc.
	Required bool
	// Expansion archives are only needed for Lord of Destruction.
	Expansion bool
}

// Archives are the base archives, required ones first.
var Archives = []Archive{
	{Name: "d2data.mpq", Required: true},
	{Name: "d2sfx.mpq", Required: true},
	{Name: "d2speech.mpq", Required: true},
	{Name: "d2exp.mpq", Required: true, Expansion: true},
	{Name: "d2char.mpq"},
	{Name: "d2music.mpq"},
	{Name: "d2video.mpq"},
	{Name: "d2xmusic.mpq", Expansion: true},
	{Name: "d2xtalk.mpq", Expansion: true},
	{Name: "d2xvideo.mpq", Expansion: true},
}

// Report is the result of checking a base install.
type Report struct {
	// Missing required archives: the game won't start.
	Missing []string `json:"missing"`
	// Missing optional archives: the game asks for a disc at some point.
	MissingOptional []string `json:"missingOptional"`
	// Present archives, by name, with their size.
	Present map[string]int64 `json:"present"`
}

// OK reports whether the game can start from this base.
func (r Report) OK() bool {
	return len(r.Missing) == 0
}

// Check looks for the base archives in dir.
func Check(dir string, expansion bool) (Report, error) {
	r := Report{Missing: []string{}, MissingOptional: []string{}, Present: map[string]int64{}}

	info, err := os.Stat(dir)
	if err != nil {
		return r, fmt.Errorf("Diablo II folder: %w", err)
	}
	if !info.IsDir() {
		return r, fmt.Errorf("%s is not a folder", dir)
	}

	for _, a := range Archives {
		if a.Expansion && !expansion {
			continue
		}

		fi, err := os.Stat(filepath.Join(dir, a.Name))
		switch {
		case err == nil && !fi.IsDir():
			r.Present[a.Name] = fi.Size()
		case a.Required:
			r.Missing = append(r.Missing, a.Name)
		default:
			r.MissingOptional = append(r.MissingOptional, a.Name)
		}
	}

	return r, nil
}

// ServerDir is where a server's files live under the base install.
func ServerDir(base, id string) string {
	return filepath.Join(base, id)
}

// ErrCrossVolume means the server folder is on a different drive from the
// base archives, so they can't be hard linked and would have to be copied.
var ErrCrossVolume = errors.New("hard links need the server folder on the same drive as Diablo II")

// LinkArchives makes the base archives visible inside a server folder as
// hard links: no extra disk space and no admin rights, but only on the same
// NTFS volume. Archives already linked are left alone.
func LinkArchives(base, serverDir string, expansion bool) error {
	if err := os.MkdirAll(serverDir, 0o755); err != nil {
		return err
	}

	for _, a := range Archives {
		if a.Expansion && !expansion {
			continue
		}

		src := filepath.Join(base, a.Name)
		srcInfo, err := os.Stat(src)
		if err != nil {
			// Missing optional archives are reported by Check.
			continue
		}

		dst := filepath.Join(serverDir, a.Name)
		if dstInfo, err := os.Stat(dst); err == nil {
			if os.SameFile(srcInfo, dstInfo) {
				continue
			}

			// A stale copy or link to an older file: replace it.
			if err := os.Remove(dst); err != nil {
				return err
			}
		}

		if err := os.Link(src, dst); err != nil {
			if isCrossVolume(err) {
				return ErrCrossVolume
			}
			return fmt.Errorf("link %s: %w", a.Name, err)
		}
	}

	return nil
}

// CopyArchives is the fallback when LinkArchives can't link. It uses real
// disk space, so the launcher asks the player first, showing CopySize.
func CopyArchives(base, serverDir string, expansion bool) error {
	if err := os.MkdirAll(serverDir, 0o755); err != nil {
		return err
	}

	for _, a := range Archives {
		if a.Expansion && !expansion {
			continue
		}

		src := filepath.Join(base, a.Name)
		srcInfo, err := os.Stat(src)
		if err != nil {
			continue
		}

		dst := filepath.Join(serverDir, a.Name)
		if dstInfo, err := os.Stat(dst); err == nil && dstInfo.Size() == srcInfo.Size() {
			continue
		}

		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("copy %s: %w", a.Name, err)
		}
	}

	return nil
}

// CopySize is how much disk space CopyArchives would use.
func CopySize(r Report) int64 {
	var total int64
	for _, size := range r.Present {
		total += size
	}

	return total
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".launcher-copy"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}

	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}

	return os.Rename(tmp, dst)
}

// Warning describes a problem with where Diablo II is installed that won't
// stop it working but is likely to cause trouble.
func Warning(base string) string {
	lower := strings.ToLower(filepath.Clean(base))

	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
		if pf := os.Getenv(env); pf != "" && strings.HasPrefix(lower, strings.ToLower(filepath.Clean(pf))+string(filepath.Separator)) {
			return "Diablo II is installed under Program Files. Windows redirects the game's writes " +
				"elsewhere there, which can make settings and updates appear to vanish. " +
				"Moving it to a folder such as C:\\Games\\Diablo II avoids this."
		}
	}

	return ""
}
