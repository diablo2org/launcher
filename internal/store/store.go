// Package store keeps the launcher's own state in its data folder: the
// player's choices, pinned signing keys, and caches. Nothing here is written
// into the game or server folders.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/d2org/launcher/internal/paths"
)

// State is everything the launcher remembers between runs.
type State struct {
	// BasePath is the player's Diablo II install.
	BasePath string `json:"basePath"`
	// LaunchDelayMs is the wait between starting boxes.
	LaunchDelayMs int `json:"launchDelayMs"`
	// Favourites are server ids in the order the player pinned them.
	Favourites []string `json:"favourites"`
	// Servers holds per-server choices, by id.
	Servers map[string]*Server `json:"servers"`
	// LegacyImported records that the old SlashDiablo launcher's settings
	// have been brought across, so it only happens once.
	LegacyImported bool `json:"legacyImported"`
}

// Server is what the launcher remembers about one server.
type Server struct {
	// ProfileURL is set for servers added by URL; listed servers' profiles
	// come from the listing.
	ProfileURL string `json:"profileUrl,omitempty"`

	// ProfileVersion is the highest profile version seen for a server added
	// by URL. An older one is refused, so a replayed old profile can't roll it
	// back.
	ProfileVersion int `json:"profileVersion"`

	Channel    string            `json:"channel,omitempty"`
	Components map[string]string `json:"components"`
	Flags      []string          `json:"flags"`
	Instances  int               `json:"instances"`
	SplitD2GL  bool              `json:"splitD2gl"`

	// D2GL window sizes for the main box and the loader boxes, used when
	// profiles are split. "" leaves the profile alone.
	D2GLMainResolution   string `json:"d2glMainResolution,omitempty"`
	D2GLLoaderResolution string `json:"d2glLoaderResolution,omitempty"`

	// Applied maps each installed component to the version installed, so a
	// switch knows what to remove.
	Applied map[string]string `json:"applied"`
}

// Store reads and writes State in a folder.
type Store struct {
	dir string
	mu  sync.Mutex
}

// Open uses dir as the launcher's data folder, creating it if needed.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	return &Store{dir: dir}, nil
}

// DefaultDir is %LOCALAPPDATA%\d2org\launcher, or the platform equivalent.
func DefaultDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(base, "d2org", "launcher"), nil
}

// Dir is the data folder.
func (s *Store) Dir() string {
	return s.dir
}

// Load reads the state, returning defaults if there is none yet.
func (s *Store) Load() (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := &State{LaunchDelayMs: 2000, Servers: map[string]*Server{}}

	data, err := os.ReadFile(filepath.Join(s.dir, "state.json"))
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, st); err != nil {
		return nil, err
	}
	if st.Servers == nil {
		st.Servers = map[string]*Server{}
	}

	return st, nil
}

// Save writes the state through a temporary file.
func (s *Store) Save(st *State) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}

	return writeAtomic(filepath.Join(s.dir, "state.json"), data)
}

// Server returns the state for id, creating it with defaults if needed.
func (st *State) Server(id string) *Server {
	srv, ok := st.Servers[id]
	if !ok {
		srv = &Server{}
		st.Servers[id] = srv
	}
	if srv.Components == nil {
		srv.Components = map[string]string{}
	}
	if srv.Applied == nil {
		srv.Applied = map[string]string{}
	}
	if srv.Flags == nil {
		srv.Flags = []string{}
	}
	if srv.Instances < 1 {
		srv.Instances = 1
	}

	return srv
}

// ServerDir is the data folder for one server's caches.
func (s *Store) ServerDir(id string) (string, error) {
	// Server ids are checked by the schema, but this is a path, so check again.
	if err := paths.Check(id); err != nil {
		return "", err
	}

	dir := filepath.Join(s.dir, "servers", id)
	return dir, os.MkdirAll(dir, 0o755)
}

// ReadCache reads a cached file for a server; a missing file is nil, nil.
func (s *Store) ReadCache(id, name string) ([]byte, error) {
	dir, err := s.ServerDir(id)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filepath.Join(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	return data, err
}

// WriteCache writes a cached file for a server.
func (s *Store) WriteCache(id, name string, data []byte) error {
	dir, err := s.ServerDir(id)
	if err != nil {
		return err
	}

	return writeAtomic(filepath.Join(dir, name), data)
}

func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}

	return nil
}
