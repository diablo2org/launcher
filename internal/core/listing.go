package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/d2org/launcher/internal/fetch"
	"github.com/d2org/launcher/internal/spec"
	"github.com/d2org/launcher/internal/store"
)

// Listing supplies the listed servers' profiles. The listing is the trust
// decision: a server gets in by a reviewed pull request adding its profile to
// servers/, so anything in a listed profile has been approved.
type Listing interface {
	Profiles(ctx context.Context) ([]json.RawMessage, error)
}

// Index is servers/index.json: every listed profile in one file, so the
// launcher fetches the whole listing in one request. It is generated from
// servers/*.json by cmd/listing, and a test fails when it is out of date.
type Index struct {
	Schema  int               `json:"schema"`
	Servers []json.RawMessage `json:"servers"`
}

// DirListing reads profiles from a folder, like servers/ in the repo. Used
// for development and to build the index.
type DirListing string

// Profiles returns every *.json file in the folder except the index.
func (d DirListing) Profiles(context.Context) ([]json.RawMessage, error) {
	if d == "" {
		return nil, nil
	}

	files, err := filepath.Glob(filepath.Join(string(d), "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	var out []json.RawMessage
	for _, f := range files {
		if filepath.Base(f) == "index.json" {
			continue
		}

		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		out = append(out, json.RawMessage(data))
	}

	return out, nil
}

// BuildIndex checks every profile in a folder, including that each file is
// named after its server's id, and renders the index.
func BuildIndex(dir string) ([]byte, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	idx := Index{Schema: 1, Servers: []json.RawMessage{}}
	for _, f := range files {
		if filepath.Base(f) == "index.json" {
			continue
		}

		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}

		p, err := spec.ParseProfile(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(f), err)
		}
		if want := strings.TrimSuffix(filepath.Base(f), ".json"); p.ID != want {
			return nil, fmt.Errorf("%s: id %q does not match the file name", filepath.Base(f), p.ID)
		}

		// Stored compact, so the index doesn't depend on each file's layout.
		compact, err := json.Marshal(json.RawMessage(data))
		if err != nil {
			return nil, err
		}
		idx.Servers = append(idx.Servers, compact)
	}

	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return nil, err
	}

	return append(data, '\n'), nil
}

// RemoteListing fetches the published index, falling back to the last copy
// when offline.
type RemoteListing struct {
	URL    string
	Client *fetch.Client
	Store  *store.Store
}

// Profiles fetches the index.
func (r RemoteListing) Profiles(ctx context.Context) ([]json.RawMessage, error) {
	const cache = "listing-index.json"

	data, err := r.Client.Document(ctx, r.URL)
	if err != nil {
		cached, cerr := os.ReadFile(filepath.Join(r.Store.Dir(), cache))
		if cerr != nil {
			return nil, err
		}
		data = cached
	}

	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("server listing: %w", err)
	}
	if idx.Schema != 1 {
		return nil, fmt.Errorf("server listing: unsupported schema %d", idx.Schema)
	}

	if err == nil {
		os.WriteFile(filepath.Join(r.Store.Dir(), cache), data, 0o644)
	}

	return idx.Servers, nil
}
