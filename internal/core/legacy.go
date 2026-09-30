package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/d2org/launcher/internal/install"
)

// LegacyServer is the server the old SlashDiablo launcher's settings belong to.
const LegacyServer = "slashdiablo"

// LegacyConfigPath is where the old SlashDiablo launcher kept its settings.
func LegacyConfigPath() string {
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}

	return filepath.Join(base, "slashdiablo.net", "Slashdiablo launcher", "config.json")
}

// legacyConfig is the part of the old launcher's config.json worth keeping.
type legacyConfig struct {
	LaunchDelay int `json:"launch_delay"`
	Games       []struct {
		Location          string   `json:"location"`
		Instances         int      `json:"instances"`
		Flags             []string `json:"flags"`
		HDVersion         string   `json:"hd_version"`
		MaphackVersion    string   `json:"maphack_version"`
		D2GLVersion       string   `json:"d2gl_version"`
		D2GLSplitProfiles bool     `json:"d2gl_split_profiles"`
		D2GLMainRes       string   `json:"d2gl_main_resolution"`
		D2GLLoaderRes     string   `json:"d2gl_loader_resolution"`
	} `json:"games"`
}

// legacyPath turns the old launcher's "/C:/Games/Diablo II" into a Windows path.
func legacyPath(p string) string {
	p = strings.TrimPrefix(p, "/")
	return filepath.Clean(filepath.FromSlash(p))
}

// ImportLegacy brings a SlashDiablo player's old launcher settings across,
// once: their Diablo II folder, box count, flags, maphack, HD and D2GL
// versions, and launch delay. It only fills in what isn't set yet, and only
// choices the SlashDiablo profile actually offers. It reports whether
// anything was imported.
func (m *Manager) ImportLegacy(ctx context.Context, configPath string) (bool, error) {
	m.mu.Lock()
	done := m.state.LegacyImported
	m.mu.Unlock()

	if done || !m.listed(LegacyServer) || configPath == "" {
		return false, nil
	}

	data, err := os.ReadFile(configPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, m.markLegacyImported()
	}
	if err != nil {
		return false, err
	}

	var old legacyConfig
	if err := json.Unmarshal(data, &old); err != nil || len(old.Games) == 0 {
		return false, m.markLegacyImported()
	}

	p, err := m.Profile(ctx, LegacyServer)
	if err != nil {
		// Try again next start, once the profile loads.
		return false, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// The old launcher patched each install in place, so its folders still
	// hold the base archives and make a good base for the new layout.
	if m.state.BasePath == "" {
		for _, g := range old.Games {
			dir := legacyPath(g.Location)
			if r, err := install.Check(dir, true); err == nil && r.OK() {
				m.state.BasePath = dir
				break
			}
		}
	}

	srv := m.state.Server(LegacyServer)

	boxes := 0
	for _, g := range old.Games {
		boxes += g.Instances
	}
	max := p.Launch.MaxInstances
	if max < 1 {
		max = 1
	}
	if boxes > max {
		boxes = max
	}
	if boxes > 0 {
		srv.Instances = boxes
	}

	g := old.Games[0]

	allowed := map[string]bool{}
	for _, f := range p.Launch.AllowedFlags {
		allowed[f] = true
	}
	for _, f := range g.Flags {
		if allowed[f] && !contains(srv.Flags, f) {
			srv.Flags = append(srv.Flags, f)
		}
	}

	// Map the old per-mod versions onto components of the same kind or id.
	for _, c := range p.Components {
		var version string
		switch {
		case c.ID == "maphack" || c.Kind == "bh":
			version = g.MaphackVersion
		case c.ID == "hd":
			version = g.HDVersion
		case c.ID == "d2gl" || c.Kind == "d2gl":
			version = g.D2GLVersion
		default:
			continue
		}

		if version == "none" {
			srv.Components[c.ID] = ""
			continue
		}
		for _, v := range c.Versions {
			if v.ID == version {
				srv.Components[c.ID] = version
			}
		}
	}
	srv.SplitD2GL = g.D2GLSplitProfiles
	if validResolution(g.D2GLMainRes) {
		srv.D2GLMainResolution = g.D2GLMainRes
	}
	if validResolution(g.D2GLLoaderRes) {
		srv.D2GLLoaderResolution = g.D2GLLoaderRes
	}

	if old.LaunchDelay > 0 && old.LaunchDelay <= 30000 {
		m.state.LaunchDelayMs = old.LaunchDelay
	}

	if len(m.state.Favourites) == 0 {
		m.state.Favourites = []string{LegacyServer}
	}

	m.state.LegacyImported = true
	return true, m.save()
}

func (m *Manager) markLegacyImported() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.state.LegacyImported = true
	return m.save()
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}

	return false
}
