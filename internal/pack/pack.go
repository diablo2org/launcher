// Package pack is the server side of the spec: it builds a file manifest from
// a folder and writes a starter server profile. The d2pack command wraps it
// for server teams.
package pack

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diablo2org/launcher/internal/fetch"
	"github.com/diablo2org/launcher/internal/install"
	"github.com/diablo2org/launcher/internal/paths"
	"github.com/diablo2org/launcher/internal/spec"
)

// Options control how a folder becomes a manifest.
type Options struct {
	Server  string
	Version string
	// BaseURL is where the folder is published. Each file's URL is BaseURL
	// plus its path.
	BaseURL string
	// Once lists patterns for files the player owns after the first install,
	// such as maphack settings. Matched case-insensitively against the path.
	Once []string
	// Exclude lists patterns for files to leave out.
	Exclude []string
	// Only, when set, keeps just the files matching one of these patterns,
	// for building a component's manifest from a full install.
	Only []string
}

// alwaysExcluded are never put in a manifest: Blizzard's base archives, which
// servers must not host, and the documents this tool writes.
func alwaysExcluded(rel string) bool {
	lower := strings.ToLower(rel)

	for _, a := range install.Archives {
		if lower == a.Name {
			return true
		}
	}

	switch {
	case lower == "manifest.json", lower == "profile.json":
		return true
	case lower == "save" || strings.HasPrefix(lower, "save/"):
		return true
	}

	return false
}

func matches(patterns []string, rel string) (bool, error) {
	lower := strings.ToLower(rel)
	for _, p := range patterns {
		p = strings.ToLower(filepath.ToSlash(p))

		ok, err := path.Match(p, lower)
		if err != nil {
			return false, fmt.Errorf("bad pattern %q: %w", p, err)
		}
		// A pattern without a folder also matches by file name anywhere.
		if !ok && !strings.Contains(p, "/") {
			ok, _ = path.Match(p, path.Base(lower))
		}
		if ok {
			return true, nil
		}
	}

	return false, nil
}

// BuildManifest hashes every file under dir into a manifest.
func BuildManifest(dir string, opts Options) (*spec.Manifest, error) {
	base, err := url.Parse(strings.TrimSuffix(opts.BaseURL, "/") + "/")
	if err != nil || (base.Scheme != "https" && base.Scheme != "http") {
		return nil, fmt.Errorf("base URL %q must be an https URL", opts.BaseURL)
	}

	m := &spec.Manifest{Schema: 1, Server: opts.Server, Version: opts.Version, Files: []spec.File{}}
	seen := map[string]string{}

	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == dir {
			return nil
		}

		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		if alwaysExcluded(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		excluded, err := matches(opts.Exclude, rel)
		if err != nil {
			return err
		}
		if excluded {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s: not a regular file", rel)
		}

		if len(opts.Only) > 0 {
			keep, err := matches(opts.Only, rel)
			if err != nil {
				return err
			}
			if !keep {
				return nil
			}
		}

		if err := paths.Check(rel); err != nil {
			return err
		}
		if first, ok := seen[paths.Key(rel)]; ok {
			return fmt.Errorf("%q and %q are the same file on Windows", first, rel)
		}
		seen[paths.Key(rel)] = rel

		size, sha, err := fetch.HashFile(p)
		if err != nil {
			return err
		}

		f := spec.File{Path: rel, Size: size, SHA256: sha, URLs: []string{fileURL(base, rel)}}

		once, err := matches(opts.Once, rel)
		if err != nil {
			return err
		}
		if once {
			f.Mode = spec.ModeOnce
		}

		m.Files = append(m.Files, f)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(m.Files, func(i, j int) bool {
		return strings.ToLower(m.Files[i].Path) < strings.ToLower(m.Files[j].Path)
	})

	return m, nil
}

// fileURL escapes each path segment, so names with spaces work.
func fileURL(base *url.URL, rel string) string {
	parts := strings.Split(rel, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}

	return base.String() + strings.Join(parts, "/")
}

// WriteManifest writes a manifest as indented JSON.
func WriteManifest(m *spec.Manifest, file string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(file, append(data, '\n'), 0o644)
}

// InitOptions describe a new server, for InitProfile.
type InitOptions struct {
	ID          string
	Name        string
	Gateway     string
	GameVersion string
	// ManifestURL is where the live channel's manifest.json is published.
	ManifestURL string
}

// InitProfile writes a starter server profile: the minimum a server needs to
// be listed, ready to extend with branding, components and settings.
func InitProfile(opts InitOptions) ([]byte, error) {
	u, err := url.Parse(opts.ManifestURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return nil, fmt.Errorf("manifest URL %q must be an https URL", opts.ManifestURL)
	}

	version := opts.GameVersion
	if version == "" {
		version = "1.13c"
	}

	profile := map[string]any{
		"schema":  1,
		"id":      opts.ID,
		"name":    opts.Name,
		"summary": "",
		"version": 1,
		"hosts":   []string{strings.ToLower(u.Hostname())},
		"game": map[string]any{
			"version": version, "expansion": true, "baseArchives": "link", "saves": "shared",
		},
		"gateways": []any{
			map[string]any{"name": opts.Name, "host": opts.Gateway, "timezone": 0},
		},
		"channels": []any{
			map[string]any{"id": "live", "name": "Live", "manifest": opts.ManifestURL},
		},
		"launch": map[string]any{
			"defaultFlags": []string{},
			"allowedFlags": []string{"-w", "-3dfx", "-ns", "-skip", "-skiptobnet", "-nofixaspect"},
			"maxInstances": 1,
		},
	}

	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return nil, err
	}

	if _, err := spec.ParseProfile(data); err != nil {
		return nil, err
	}

	return append(data, '\n'), nil
}
