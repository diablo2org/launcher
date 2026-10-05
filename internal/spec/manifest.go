package spec

import (
	"github.com/diablo2org/launcher/internal/paths"
)

// File modes. See docs/SPEC.md section 4.
const (
	ModeReplace = "replace"
	ModeOnce    = "once"
	ModeDelete  = "delete"
)

// Manifest is the set of files for one channel or component version.
type Manifest struct {
	Schema  int    `json:"schema"`
	Server  string `json:"server"`
	Version string `json:"version"`
	Files   []File `json:"files"`
}

// File is one entry in a manifest.
type File struct {
	Path   string   `json:"path"`
	Size   int64    `json:"size,omitempty"`
	SHA256 string   `json:"sha256,omitempty"`
	URLs   []string `json:"urls,omitempty"`
	Mode   string   `json:"mode,omitempty"`
}

// FileMode returns the file's mode, defaulting to replace.
func (f File) FileMode() string {
	if f.Mode == "" {
		return ModeReplace
	}

	return f.Mode
}

// ParseManifest checks a manifest for the given server, whose profile
// declares the hosts files may be downloaded from.
func ParseManifest(data []byte, profile *Profile) (*Manifest, error) {
	var m Manifest
	if err := decode(manifestSchemaID, data, &m); err != nil {
		return nil, err
	}

	var problems Problems

	if m.Server != profile.ID {
		problems.add("manifest is for server %q, not %q", m.Server, profile.ID)
	}

	seen := make(map[string]string, len(m.Files))
	for _, f := range m.Files {
		if err := paths.Check(f.Path); err != nil {
			problems.add("%v", err)
			continue
		}

		key := paths.Key(f.Path)
		if first, ok := seen[key]; ok {
			problems.add("%q and %q are the same file on Windows", first, f.Path)
		}
		seen[key] = f.Path

		for _, u := range f.URLs {
			if !allowedHost(u, profile.Hosts) {
				problems.add("%s: %s is not on one of the profile's hosts", f.Path, u)
			}
		}
	}

	if err := problems.err(); err != nil {
		return nil, err
	}

	return &m, nil
}
