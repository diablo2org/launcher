package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/diablo2org/launcher/internal/updates"
)

// LauncherUpdateProgress is sent as the "launcher:update" event while the
// new launcher downloads.
type LauncherUpdateProgress struct {
	Done  int64 `json:"done"`
	Total int64 `json:"total"`
}

// InstallUpdate downloads the newest launcher's installer, checks it against
// the release's SHA-256, starts it, and closes the launcher so its files can
// be replaced. The player sees the installer and finishes it themselves.
func (s *ServerService) InstallUpdate(ctx context.Context) error {
	if s.host.Updates == nil || s.host.RunInstaller == nil || s.host.Quit == nil || s.host.UpdatesDir == "" {
		return errors.New("updating isn't available")
	}

	r, err := updates.Check(ctx, s.host.Updates, updates.LatestURL, Version)
	if err != nil {
		slog.Error("check for update", "err", err)
		return err
	}
	if r == nil || r.Installer == nil {
		return errors.New("there's no update to install; download it from the release page")
	}

	// Each attempt gets a folder of its own inside the launcher's, so two
	// launchers updating at once can't remove each other's installer, and
	// clearing out old attempts can't touch anything the launcher didn't
	// make.
	root, err := openUpdatesDir(s.host.UpdatesDir)
	if err != nil {
		return err
	}
	defer root.Close()
	removeOldUpdates(root)
	dir, err := newAttempt(root, s.host.UpdatesDir)
	if err != nil {
		return err
	}

	slog.Info("downloading launcher update", "version", r.Version, "installer", r.Installer.Name, "bytes", r.Installer.Size)
	path, err := updates.Download(ctx, s.host.Updates, r.Installer, dir, func(done int64) {
		s.host.Emit("launcher:update", LauncherUpdateProgress{Done: done, Total: r.Installer.Size})
	})
	if err != nil {
		slog.Error("download launcher update", "err", err)
		return err
	}

	if err := s.host.RunInstaller(path); err != nil {
		slog.Error("start launcher installer", "err", err)
		return err
	}
	slog.Info("launcher installer started, closing", "version", r.Version)
	s.host.Quit()

	return nil
}

// openUpdatesDir opens the launcher's updates folder as an os.Root, after
// checking that what it opened is that folder itself, not a link or junction
// to another: it compares the opened folder with an Lstat of the path, so a
// swap between the two is caught too. Everything after goes through the
// handle, so it stays in the folder that was checked.
func openUpdatesDir(dir string) (*os.Root, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}

	opened, err := root.Stat(".")
	if err == nil {
		var info os.FileInfo
		info, err = os.Lstat(dir)
		if err == nil && (!info.IsDir() || !os.SameFile(opened, info)) {
			err = fmt.Errorf("%s is a link, not a folder; remove it and try again", dir)
		}
	}
	if err != nil {
		root.Close()
		return nil, err
	}

	return root, nil
}

// newAttempt makes a new folder for one update attempt inside root, and
// returns its path under dir, the path root was opened at.
func newAttempt(root *os.Root, dir string) (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	name := "attempt-" + hex.EncodeToString(b)
	if err := root.Mkdir(name, 0o755); err != nil {
		return "", err
	}

	return filepath.Join(dir, name), nil
}

// removeOldUpdates removes earlier attempts' folders from the updates folder.
// A started installer runs from its folder, so only folders a day old go.
// Links, junctions included, aren't IsDir, so they're left alone.
func removeOldUpdates(root *os.Root) {
	f, err := root.Open(".")
	if err != nil {
		return
	}
	entries, err := f.ReadDir(-1)
	f.Close()
	if err != nil {
		return
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil || time.Since(info.ModTime()) < 24*time.Hour {
			continue
		}
		root.RemoveAll(e.Name())
	}
}
