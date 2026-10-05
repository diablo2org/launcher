// Package core is what the launcher does, independent of any UI: it loads
// servers, keeps their folders up to date, reads and writes their settings,
// and starts them.
package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	gosync "sync"
	"time"

	"github.com/diablo2org/launcher/internal/fetch"
	"github.com/diablo2org/launcher/internal/install"
	"github.com/diablo2org/launcher/internal/launch"
	"github.com/diablo2org/launcher/internal/paths"
	"github.com/diablo2org/launcher/internal/spec"
	"github.com/diablo2org/launcher/internal/store"
	"github.com/diablo2org/launcher/internal/sync"
)

// Manager runs the launcher.
type Manager struct {
	store      *store.Store
	listing    Listing
	launcher   *launch.Launcher
	clientOpts []fetch.Option

	mu      gosync.Mutex
	state   *store.State
	entries map[string]*entry
	// loaded is set once the listing has been read.
	loaded bool
	// busy stops two updates of the same server running at once.
	busy map[string]bool
}

// New returns a manager. clientOpts are passed to every fetch client, which
// is how development builds trust the local test server.
func New(st *store.Store, listing Listing, l *launch.Launcher, clientOpts ...fetch.Option) (*Manager, error) {
	state, err := st.Load()
	if err != nil {
		return nil, err
	}

	return &Manager{
		store:      st,
		listing:    listing,
		launcher:   l,
		clientOpts: clientOpts,
		state:      state,
		entries:    map[string]*entry{},
		busy:       map[string]bool{},
	}, nil
}

func (m *Manager) save() error {
	return m.store.Save(m.state)
}

// Base is the player's Diablo II folder.
func (m *Manager) Base() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.state.BasePath
}

// DetectBase fills in the Diablo II folder from where the game itself last
// recorded it, if the player hasn't chosen one.
func (m *Manager) DetectBase() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state.BasePath != "" {
		return nil
	}

	if dir := install.Detect(); dir != "" {
		m.state.BasePath = dir
		return m.save()
	}

	return nil
}

// SetBase changes the Diablo II folder after checking it.
func (m *Manager) SetBase(dir string) (install.Report, error) {
	r, err := install.Check(dir, true)
	if err != nil {
		return r, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.state.BasePath = dir
	return r, m.save()
}

// CheckBase reports on the current Diablo II folder.
func (m *Manager) CheckBase() (install.Report, string, error) {
	base := m.Base()
	if base == "" {
		return install.Report{}, "", errors.New("no Diablo II folder chosen")
	}

	r, err := install.Check(base, true)
	return r, install.Warning(base), err
}

// Favourites returns the pinned server ids in order.
func (m *Manager) Favourites() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return append([]string{}, m.state.Favourites...)
}

// MaxFavourites is how many servers can be pinned to the rail.
const MaxFavourites = 3

// ErrTooManyFavourites means the rail is full.
var ErrTooManyFavourites = fmt.Errorf("you can pin up to %d servers; unpin one first", MaxFavourites)

// SetFavourite pins or unpins a server.
func (m *Manager) SetFavourite(id string, on bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	favs := []string{}
	for _, f := range m.state.Favourites {
		if f != id {
			favs = append(favs, f)
		}
	}
	if on {
		if len(favs) >= MaxFavourites {
			return ErrTooManyFavourites
		}
		favs = append(favs, id)
	}
	m.state.Favourites = favs

	return m.save()
}

// SetFavouriteOrder reorders the pinned servers, as when the player drags them
// in the rail. ids must be exactly the pinned servers, in their new order.
func (m *Manager) SetFavouriteOrder(ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	current := map[string]bool{}
	for _, f := range m.state.Favourites {
		current[f] = true
	}

	seen := map[string]bool{}
	for _, id := range ids {
		if !current[id] || seen[id] {
			return fmt.Errorf("%q is not a pinned server", id)
		}
		seen[id] = true
	}
	if len(seen) != len(current) {
		return errors.New("the new order must include every pinned server")
	}

	m.state.Favourites = append([]string{}, ids...)
	return m.save()
}

// LaunchDelay is the wait between starting boxes, in milliseconds.
func (m *Manager) LaunchDelay() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.state.LaunchDelayMs
}

// SetLaunchDelay changes the wait between boxes.
func (m *Manager) SetLaunchDelay(ms int) error {
	if ms < 0 || ms > 30000 {
		return errors.New("launch delay must be between 0 and 30 seconds")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.state.LaunchDelayMs = ms
	return m.save()
}

// FirstRun reports whether to show the welcome screen: the player hasn't
// been through it, and has no servers pinned, as anyone who has used the
// launcher, or the old SlashDiablo one, will have.
func (m *Manager) FirstRun() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	return !m.state.Welcomed && len(m.state.Favourites) == 0
}

// FinishWelcome records that the player has been through the welcome screen,
// so it isn't shown again.
func (m *Manager) FinishWelcome() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.state.Welcomed = true
	return m.save()
}

// ServerInfo is a server as the catalog shows it.
type ServerInfo struct {
	ID        string        `json:"id"`
	Verified  bool          `json:"verified"`
	Favourite bool          `json:"favourite"`
	Profile   *spec.Profile `json:"profile"`
	Error     string        `json:"error"`
}

// entry is one known server: its profile, or why it couldn't be loaded.
type entry struct {
	profile *spec.Profile
	err     error
	// listed servers came in through the reviewed listing; the rest were
	// added by URL.
	listed bool
}

// rawID pulls the id out of a profile that failed to parse, so the catalog
// can still say which server is broken.
func rawID(raw json.RawMessage, fallback string) string {
	var v struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &v) == nil && v.ID != "" {
		return v.ID
	}

	return fallback
}

// Servers loads the listing and any servers the player added by URL. A server
// whose profile doesn't load is still returned, with the error, so the player
// sees why. If the listing itself can't be loaded, servers added by URL are
// still returned alongside the error.
func (m *Manager) Servers(ctx context.Context) ([]ServerInfo, error) {
	raws, listErr := m.listing.Profiles(ctx)
	if listErr != nil {
		listErr = fmt.Errorf("server listing: %w", listErr)
	}

	entries := map[string]*entry{}
	var order []string

	for i, raw := range raws {
		p, err := spec.ParseProfile(raw)
		if err != nil {
			id := rawID(raw, fmt.Sprintf("server-%d", i+1))
			if entries[id] == nil {
				entries[id] = &entry{err: err, listed: true}
				order = append(order, id)
			}
			continue
		}
		if entries[p.ID] != nil {
			continue
		}
		entries[p.ID] = &entry{profile: p, listed: true}
		order = append(order, p.ID)
	}

	// Servers added by URL. A listed server always wins over one added by URL
	// with the same id.
	m.mu.Lock()
	added := map[string]string{}
	for id, srv := range m.state.Servers {
		if srv.ProfileURL != "" && entries[id] == nil {
			added[id] = srv.ProfileURL
		}
	}
	m.mu.Unlock()

	addedIDs := make([]string, 0, len(added))
	for id := range added {
		addedIDs = append(addedIDs, id)
	}
	sort.Strings(addedIDs)

	for _, id := range addedIDs {
		p, err := m.fetchAdded(ctx, id, added[id])
		entries[id] = &entry{profile: p, err: err}
		order = append(order, id)
	}

	m.mu.Lock()
	m.entries = entries
	m.loaded = true
	favs := map[string]bool{}
	for _, f := range m.state.Favourites {
		favs[f] = true
	}
	m.mu.Unlock()

	out := make([]ServerInfo, 0, len(order))
	for _, id := range order {
		e := entries[id]
		info := ServerInfo{ID: id, Verified: e.listed, Favourite: favs[id], Profile: e.profile}
		if e.err != nil {
			info.Error = e.err.Error()
		}
		out = append(out, info)
	}

	return out, listErr
}

// ErrAlreadyListed means a server added by URL is already in the listing.
var ErrAlreadyListed = errors.New("this server is already listed; open it from All servers")

// AddServer adds a server that isn't in the listing, from the URL of its
// profile. Such servers haven't been reviewed and show as not verified.
func (m *Manager) AddServer(ctx context.Context, profileURL string) (*spec.Profile, error) {
	u, err := url.Parse(strings.TrimSpace(profileURL))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return nil, errors.New("the profile URL must start with https://")
	}

	data, err := m.client([]string{u.Hostname()}).Document(ctx, u.String())
	if err != nil {
		return nil, err
	}

	p, err := spec.ParseProfile(data)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if e := m.entries[p.ID]; e != nil && e.listed {
		return nil, ErrAlreadyListed
	}

	srv := m.state.Server(p.ID)
	srv.ProfileURL = u.String()
	if p.Version > srv.ProfileVersion {
		srv.ProfileVersion = p.Version
	}
	m.entries[p.ID] = &entry{profile: p}

	if err := m.store.WriteCache(p.ID, "profile.json", data); err != nil {
		return nil, err
	}

	return p, m.save()
}

// RemoveServer forgets a server added by URL. Its folder under the Diablo II
// install is left alone.
func (m *Manager) RemoveServer(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	srv := m.state.Servers[id]
	if srv == nil || srv.ProfileURL == "" {
		return errors.New("only servers added by URL can be removed")
	}

	delete(m.state.Servers, id)
	delete(m.entries, id)

	favs := []string{}
	for _, f := range m.state.Favourites {
		if f != id {
			favs = append(favs, f)
		}
	}
	m.state.Favourites = favs

	return m.save()
}

// client returns a fetch client limited to hosts.
func (m *Manager) client(hosts []string) *fetch.Client {
	return fetch.New(hosts, m.clientOpts...)
}

func cacheName(prefix, rawURL string) string {
	sum := sha256.Sum256([]byte(rawURL))
	return prefix + "-" + hex.EncodeToString(sum[:8]) + ".json"
}

// fetchAdded loads the profile of a server added by URL, falling back to the
// last copy when offline, and refusing an older version than one already
// seen.
func (m *Manager) fetchAdded(ctx context.Context, id, rawURL string) (*spec.Profile, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	data, fetchErr := m.client([]string{u.Hostname()}).Document(ctx, rawURL)
	if fetchErr != nil {
		cached, err := m.store.ReadCache(id, "profile.json")
		if err != nil || cached == nil {
			return nil, fetchErr
		}
		data = cached
	}

	p, err := spec.ParseProfile(data)
	if err != nil {
		return nil, err
	}
	if p.ID != id {
		return nil, fmt.Errorf("profile at %s is now for %q, not %q", rawURL, p.ID, id)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	srv := m.state.Server(id)
	if p.Version < srv.ProfileVersion {
		return nil, fmt.Errorf("%s: profile version %d is older than %d already seen; refusing a rollback", p.Name, p.Version, srv.ProfileVersion)
	}

	if fetchErr == nil {
		if err := m.store.WriteCache(id, "profile.json", data); err != nil {
			return nil, err
		}
		if p.Version > srv.ProfileVersion {
			srv.ProfileVersion = p.Version
			if err := m.save(); err != nil {
				return nil, err
			}
		}
	}

	return p, nil
}

// Profile returns a server's profile, loading the listing first if needed.
func (m *Manager) Profile(ctx context.Context, id string) (*spec.Profile, error) {
	m.mu.Lock()
	loaded := m.loaded
	m.mu.Unlock()

	if !loaded {
		m.Servers(ctx)
	}

	m.mu.Lock()
	e := m.entries[id]
	m.mu.Unlock()

	switch {
	case e == nil:
		return nil, fmt.Errorf("unknown server %q", id)
	case e.profile == nil:
		return nil, e.err
	}

	return e.profile, nil
}

// listed reports whether a server came from the listing.
func (m *Manager) listed(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	e := m.entries[id]
	return e != nil && e.listed && e.profile != nil
}

// layer is one manifest: the channel, or a component version.
type layer struct {
	component string
	version   string
	url       string
	manifest  *spec.Manifest
}

// enabled returns every component the player has on, with its version,
// falling back to each component's default. An empty stored version means
// turned off; CustomVersion means on, with the player's own files.
func enabled(p *spec.Profile, srv *store.Server) map[string]string {
	out := map[string]string{}
	for _, c := range p.Components {
		v, set := srv.Components[c.ID]
		if !set {
			v = c.Default
		}
		if v != "" {
			out[c.ID] = v
		}
	}

	return out
}

// wanted returns the components the launcher installs: those enabled at a
// real version. Custom components are the player's to manage.
func wanted(p *spec.Profile, srv *store.Server) map[string]string {
	out := enabled(p, srv)
	for id, v := range out {
		if v == CustomVersion {
			delete(out, id)
		}
	}

	return out
}

func componentManifestURL(p *spec.Profile, comp, version string) (string, error) {
	for _, c := range p.Components {
		if c.ID != comp {
			continue
		}
		for _, v := range c.Versions {
			if v.ID == version {
				return v.Manifest, nil
			}
		}
		return "", fmt.Errorf("%s has no version %q", c.Name, version)
	}

	return "", fmt.Errorf("unknown component %q", comp)
}

// manifest fetches a manifest from the profile's hosts, caching it so a later
// removal knows what a version installed even if it's gone from the server,
// and so an installed server still plays offline.
func (m *Manager) manifest(ctx context.Context, id string, p *spec.Profile, rawURL string) (*spec.Manifest, error) {
	name := cacheName("manifest", rawURL)

	data, fetchErr := m.client(p.Hosts).Document(ctx, rawURL)
	if fetchErr != nil {
		cached, err := m.store.ReadCache(id, name)
		if err != nil || cached == nil {
			return nil, fetchErr
		}
		data = cached
	} else if err := m.store.WriteCache(id, name, data); err != nil {
		return nil, err
	}

	return spec.ParseManifest(data, p)
}

// layers fetches the channel manifest and one per wanted component.
func (m *Manager) layers(ctx context.Context, id string, p *spec.Profile, srv *store.Server) ([]layer, error) {
	channel := p.Channels[0]
	for _, c := range p.Channels {
		if c.ID == srv.Channel {
			channel = c
		}
	}

	base, err := m.manifest(ctx, id, p, channel.Manifest)
	if err != nil {
		return nil, fmt.Errorf("%s files: %w", channel.Name, err)
	}
	out := []layer{{url: channel.Manifest, manifest: base}}

	want := wanted(p, srv)
	for _, c := range p.Components {
		v, on := want[c.ID]
		if !on {
			continue
		}

		u, err := componentManifestURL(p, c.ID, v)
		if err != nil {
			return nil, err
		}

		mf, err := m.manifest(ctx, id, p, u)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", c.Name, v, err)
		}
		out = append(out, layer{component: c.ID, version: v, url: u, manifest: mf})
	}

	return out, nil
}

// merge combines layers into one manifest. When layers list the same path,
// the later one wins: the channel first, then components in profile order,
// so a renderer's glide3x.dll replaces the base one rather than the two
// replacing each other on every update.
func merge(layers []layer) *spec.Manifest {
	index := map[string]int{}
	out := &spec.Manifest{}

	for _, l := range layers {
		for _, f := range l.manifest.Files {
			key := paths.Key(f.Path)
			if i, ok := index[key]; ok {
				out.Files[i] = f
				continue
			}
			index[key] = len(out.Files)
			out.Files = append(out.Files, f)
		}
	}

	return out
}

// Status is whether a server is ready to play.
type Status struct {
	Installed   bool   `json:"installed"`
	UpToDate    bool   `json:"upToDate"`
	UpdateFiles int    `json:"updateFiles"`
	UpdateBytes int64  `json:"updateBytes"`
	Error       string `json:"error"`
}

func (m *Manager) serverDir(id string) (string, error) {
	base := m.Base()
	if base == "" {
		return "", errors.New("choose your Diablo II folder first")
	}

	return install.ServerDir(base, id), nil
}

// ServerFolder returns a server's folder under the Diablo II install,
// creating it if needed, so the player can open it and add their own files.
func (m *Manager) ServerFolder(id string) (string, error) {
	if _, err := m.Profile(context.Background(), id); err != nil {
		return "", err
	}

	dir, err := m.serverDir(id)
	if err != nil {
		return "", err
	}

	return dir, os.MkdirAll(dir, 0o755)
}

// hashCache loads the server's file hash cache.
func (m *Manager) hashCache(id string) sync.Cache {
	cache := sync.Cache{}
	if data, _ := m.store.ReadCache(id, "hashes.json"); data != nil {
		json.Unmarshal(data, &cache)
	}

	return cache
}

func (m *Manager) saveHashCache(id string, cache sync.Cache) {
	if data, err := json.Marshal(cache); err == nil {
		m.store.WriteCache(id, "hashes.json", data)
	}
}

// plan works out everything an update would do: removals for components
// turned off or switched, then the merged manifest.
func (m *Manager) plan(ctx context.Context, id string) (*spec.Profile, []layer, *sync.Plan, *sync.Plan, sync.Cache, error) {
	p, err := m.Profile(ctx, id)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	dir, err := m.serverDir(id)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	m.mu.Lock()
	srv := m.state.Server(id)
	m.mu.Unlock()

	layers, err := m.layers(ctx, id, p, srv)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	on := enabled(p, srv)

	// Files a custom component could hold are the player's: never written,
	// repaired or removed, even when another layer lists the same path.
	custom := map[string]bool{}
	for _, c := range p.Components {
		if on[c.ID] != CustomVersion {
			continue
		}
		for _, v := range c.Versions {
			mf, err := m.manifest(ctx, id, p, v.Manifest)
			if err != nil {
				continue
			}
			for _, f := range mf.Files {
				custom[paths.Key(f.Path)] = true
			}
		}
	}

	merged := merge(layers)
	if len(custom) > 0 {
		files := merged.Files[:0]
		for _, f := range merged.Files {
			if !custom[paths.Key(f.Path)] {
				files = append(files, f)
			}
		}
		merged.Files = files
	}

	keep := map[string]bool{}
	for _, f := range merged.Files {
		keep[paths.Key(f.Path)] = true
	}
	for k := range custom {
		keep[k] = true
	}

	cache := m.hashCache(id)

	removal := &sync.Plan{}
	removing := map[string]bool{}
	addRemovals := func(r *sync.Plan) {
		for _, a := range r.Actions {
			key := paths.Key(a.File.Path)
			if !keep[key] && !removing[key] {
				removing[key] = true
				removal.Actions = append(removal.Actions, a)
			}
		}
	}

	want := wanted(p, srv)
	for _, c := range p.Components {
		applied, recorded := srv.Applied[c.ID]

		switch {
		case on[c.ID] == CustomVersion:
			// The player's own files: leave them exactly as they are.
			continue

		case recorded && applied != want[c.ID]:
			// Switched or turned off: remove what the recorded version
			// installed.
			u, err := componentManifestURL(p, c.ID, applied)
			if err != nil {
				// The version is gone from the profile; nothing to remove by.
				continue
			}
			old, err := m.manifest(ctx, id, p, u)
			if err != nil {
				continue
			}
			r, err := sync.PlanRemoval(dir, old)
			if err != nil {
				return nil, nil, nil, nil, nil, err
			}
			addRemovals(r)

		case !recorded && want[c.ID] == "":
			// Off, with no record of installing it: remove only files that
			// match one of its versions exactly.
			for _, v := range c.Versions {
				old, err := m.manifest(ctx, id, p, v.Manifest)
				if err != nil {
					continue
				}
				r, err := sync.PlanMatchingRemoval(dir, old, cache)
				if err != nil {
					return nil, nil, nil, nil, nil, err
				}
				addRemovals(r)
			}
		}
	}

	update, err := sync.PlanManifest(dir, merged, cache)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	return p, layers, removal, update, cache, nil
}

// Status reports whether the server is installed and up to date.
func (m *Manager) Status(ctx context.Context, id string) Status {
	p, _, removal, update, cache, err := m.plan(ctx, id)
	if err != nil {
		return Status{Error: err.Error()}
	}
	m.saveHashCache(id, cache)

	dir, _ := m.serverDir(id)
	_, exeErr := os.Stat(filepath.Join(dir, filepath.FromSlash(p.Launch.ExePath())))

	return Status{
		Installed:   exeErr == nil,
		UpToDate:    len(update.Actions) == 0 && len(removal.Actions) == 0,
		UpdateFiles: len(update.Actions) + len(removal.Actions),
		UpdateBytes: update.Bytes,
	}
}

// ErrNeedsCopy means the base archives can't be hard linked into the server
// folder and would need copying, which uses real disk space. The player is
// asked, and Update is called again with allowCopy.
type ErrNeedsCopy struct {
	Bytes int64
}

func (e ErrNeedsCopy) Error() string {
	return fmt.Sprintf("the game archives can't be linked here and need copying (%.1f GB)", float64(e.Bytes)/(1<<30))
}

// Update brings a server's folder up to date.
func (m *Manager) Update(ctx context.Context, id string, allowCopy bool, progress func(sync.Progress)) error {
	m.mu.Lock()
	if m.busy[id] {
		m.mu.Unlock()
		return errors.New("already updating")
	}
	m.busy[id] = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.busy, id)
		m.mu.Unlock()
	}()

	report, _, err := m.CheckBase()
	if err != nil {
		return err
	}
	if !report.OK() {
		return fmt.Errorf("your Diablo II folder is missing %s", strings.Join(report.Missing, ", "))
	}

	p, layers, removal, update, cache, err := m.plan(ctx, id)
	if err != nil {
		return err
	}

	base := m.Base()
	dir := install.ServerDir(base, id)

	if p.Game.BaseArchives == "link" {
		if err := install.LinkArchives(base, dir, p.Game.NeedsExpansion()); err != nil {
			if !allowCopy {
				return ErrNeedsCopy{Bytes: install.CopySize(report)}
			}
			if err := install.CopyArchives(base, dir, p.Game.NeedsExpansion()); err != nil {
				return err
			}
		}
	} else if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	client := m.client(p.Hosts)
	defer m.saveHashCache(id, cache)

	if err := sync.Apply(ctx, client, dir, removal, cache, nil); err != nil {
		return err
	}
	if err := sync.Apply(ctx, client, dir, update, cache, progress); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	srv := m.state.Server(id)
	srv.Applied = map[string]string{}
	for _, l := range layers {
		if l.component != "" {
			srv.Applied[l.component] = l.version
		}
	}

	return m.save()
}

// Play starts the server's boxes.
func (m *Manager) Play(ctx context.Context, id string) error {
	p, err := m.Profile(ctx, id)
	if err != nil {
		return err
	}

	base := m.Base()
	if base == "" {
		return errors.New("choose your Diablo II folder first")
	}

	m.mu.Lock()
	srv := m.state.Server(id)
	c := launch.Choices{
		// Custom components still count: their flags and d2gl profiles apply.
		Components: enabled(p, srv),
		Flags:      append([]string{}, srv.Flags...),
		Instances:  srv.Instances,
		SplitD2GL:  srv.SplitD2GL,
	}
	delay := time.Duration(m.state.LaunchDelayMs) * time.Millisecond
	mainRes, loaderRes := srv.D2GLMainResolution, srv.D2GLLoaderResolution
	m.mu.Unlock()

	dir := install.ServerDir(base, id)
	if c.SplitD2GL && d2glOn(p, c.Components) {
		if err := applyD2GLProfiles(dir, mainRes, loaderRes); err != nil {
			return err
		}
	}

	return m.launcher.Launch(ctx, base, dir, p, c, delay)
}
