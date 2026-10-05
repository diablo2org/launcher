package app

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"

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
	if s.host.Updates == nil || s.host.RunInstaller == nil || s.host.Quit == nil {
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

	dir := filepath.Join(os.TempDir(), "diablo2org-launcher-update")
	if err := os.MkdirAll(dir, 0o755); err != nil {
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
