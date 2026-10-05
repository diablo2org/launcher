package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
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
	if err := os.MkdirAll(s.host.UpdatesDir, 0o755); err != nil {
		return err
	}
	if err := plainDir(s.host.UpdatesDir); err != nil {
		return err
	}
	removeOldUpdates(s.host.UpdatesDir)
	dir, err := os.MkdirTemp(s.host.UpdatesDir, "attempt-")
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

// removeOldUpdates removes earlier attempts' folders from the launcher's
// updates folder. A started installer runs from its folder, so only folders
// a day old go.
//
// Removal goes through an os.Root on dir, so nothing outside it can be
// removed, even if an entry is replaced by a link meanwhile.
func removeOldUpdates(dir string) {
	if plainDir(dir) != nil {
		return
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return
	}
	defer root.Close()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, e := range entries {
		// Links, junctions included, aren't IsDir, so they're left alone.
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

// plainDir returns an error unless dir is a folder itself, not a link or
// junction to one, which could send downloads and cleanup somewhere else.
func plainDir(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is a link or file, not a folder; remove it and try again", dir)
	}

	return nil
}
