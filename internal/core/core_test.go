package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	gosync "sync"
	"testing"
	"time"

	"github.com/diablo2org/launcher/internal/fetch"
	"github.com/diablo2org/launcher/internal/launch"
	"github.com/diablo2org/launcher/internal/spec"
	"github.com/diablo2org/launcher/internal/store"
)

// site is a fake server's web host: it serves documents and files.
type site struct {
	t    *testing.T
	srv  *httptest.Server
	host string

	mu    gosync.Mutex
	files map[string][]byte
	down  bool
}

func newSite(t *testing.T) *site {
	t.Helper()

	s := &site{t: t, files: map[string][]byte{}}
	s.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()

		body, ok := s.files[r.URL.Path]
		if s.down || !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(s.srv.Close)

	u, _ := url.Parse(s.srv.URL)
	s.host = u.Hostname()

	return s
}

func (s *site) url(path string) string { return s.srv.URL + path }

func (s *site) put(path string, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[path] = body
}

// json publishes a JSON document.
func (s *site) json(path string, doc interface{}) {
	data, err := json.Marshal(doc)
	if err != nil {
		s.t.Fatal(err)
	}
	s.put(path, data)
}

// file publishes a game file under a layer's folder and returns its entry.
func (s *site) file(layer, path, body string) spec.File {
	at := "/files/" + layer + "/" + path
	s.put(at, []byte(body))
	sum := sha256.Sum256([]byte(body))
	return spec.File{Path: path, Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), URLs: []string{s.url(at)}}
}

func (s *site) manifest(path string, files ...spec.File) string {
	s.json(path, spec.Manifest{Schema: 1, Server: "slash", Version: "1", Files: files})
	return s.url(path)
}

func once(f spec.File) spec.File {
	f.Mode = spec.ModeOnce
	return f
}

type fakeRegistry struct{ values map[string]string }

func (f *fakeRegistry) GatewayList() ([]string, error)     { return nil, nil }
func (f *fakeRegistry) SetGatewayList([]string) error      { return nil }
func (f *fakeRegistry) String(name string) (string, error) { return f.values[name], nil }
func (f *fakeRegistry) SetString(name, value string) error { f.values[name] = value; return nil }

type harness struct {
	site    *site
	m       *Manager
	base    string
	store   *store.Store
	listing string
	started []launch.Box
	profile map[string]interface{}
}

// list writes the profile into the listing, as a merged pull request would.
func (h *harness) list(t *testing.T) {
	t.Helper()

	data, err := json.Marshal(h.profile)
	if err != nil {
		t.Fatal(err)
	}

	os.RemoveAll(h.listing)
	os.MkdirAll(h.listing, 0o755)
	if err := os.WriteFile(filepath.Join(h.listing, h.profile["id"].(string)+".json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	h := &harness{site: newSite(t), base: t.TempDir(), listing: filepath.Join(t.TempDir(), "servers")}
	for _, a := range []string{"d2data.mpq", "d2sfx.mpq", "d2speech.mpq", "d2exp.mpq"} {
		os.WriteFile(filepath.Join(h.base, a), []byte("retail "+a), 0o644)
	}

	s := h.site
	live := s.manifest("/live.json",
		s.file("live", "Game.exe", "game 1.13c"),
		s.file("live", "glide3x.dll", "base glide"),
		once(s.file("live", "BH_settings.cfg", "Reveal Map: False\r\n\r\n")),
	)
	mh1 := s.manifest("/mh1.json", s.file("mh1", "BH.dll", "bh one"))
	mh2 := s.manifest("/mh2.json", s.file("mh2", "BH2.dll", "bh two"))
	d2gl := s.manifest("/d2gl.json", s.file("d2gl", "glide3x.dll", "d2gl glide"), s.file("d2gl", "d2gl.ini", "[Screen]\nunlock_cursor=false\n"))

	h.profile = map[string]interface{}{
		"schema": 1, "id": "slash", "name": "Slash", "version": 2,
		"hosts":    []string{s.host},
		"game":     map[string]interface{}{"version": "1.13c", "baseArchives": "link"},
		"gateways": []interface{}{map[string]interface{}{"name": "Slash", "host": "play.slash.test", "realm": "Slash"}},
		"channels": []interface{}{map[string]interface{}{"id": "live", "name": "Live", "manifest": live}},
		"components": []interface{}{
			map[string]interface{}{"id": "maphack", "name": "Maphack", "kind": "bh", "default": "1",
				"versions": []interface{}{
					map[string]interface{}{"id": "1", "manifest": mh1},
					map[string]interface{}{"id": "2", "manifest": mh2},
				}},
			map[string]interface{}{"id": "d2gl", "name": "D2GL", "kind": "d2gl", "flags": []string{"-3dfx"},
				"versions": []interface{}{map[string]interface{}{"id": "1.3.3", "manifest": d2gl}}},
		},
		"settings": []interface{}{
			map[string]interface{}{"id": "reveal", "label": "Reveal map", "type": "bool", "component": "maphack",
				"target": map[string]interface{}{"file": "BH_settings.cfg", "format": "bh", "key": "Reveal Map"}},
			map[string]interface{}{"id": "cursor", "label": "Unlock cursor", "type": "bool", "component": "d2gl",
				"target": map[string]interface{}{"file": "d2gl.ini", "format": "ini", "section": "Screen", "key": "unlock_cursor"}},
			map[string]interface{}{"id": "windowed", "label": "Windowed", "type": "bool",
				"target": map[string]interface{}{"flag": "-w"}},
		},
		"launch": map[string]interface{}{"defaultFlags": []string{"-skiptobnet"}, "allowedFlags": []string{"-w"}, "maxInstances": 2},
	}
	h.list(t)

	h.store, _ = store.Open(t.TempDir())
	st, _ := h.store.Load()
	st.BasePath = h.base
	st.LaunchDelayMs = 1
	h.store.Save(st)

	l := launch.New(&fakeRegistry{values: map[string]string{}}, func(b launch.Box) error {
		h.started = append(h.started, b)
		return nil
	})

	var err error
	h.m, err = New(h.store, DirListing(h.listing), l, fetch.WithHTTPClient(s.srv.Client()))
	if err != nil {
		t.Fatal(err)
	}

	return h
}

func (h *harness) read(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(h.base, "slash", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
	return string(data)
}

func (h *harness) exists(rel string) bool {
	_, err := os.Stat(filepath.Join(h.base, "slash", filepath.FromSlash(rel)))
	return err == nil
}

func TestEndToEnd(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	servers, err := h.m.Servers(ctx)
	if err != nil || len(servers) != 1 || servers[0].Error != "" || servers[0].Profile.Name != "Slash" {
		t.Fatalf("Servers = %+v, %v", servers, err)
	}

	if st := h.m.Status(ctx, "slash"); st.Installed || st.UpToDate || st.Error != "" {
		t.Errorf("before install: %+v", st)
	}

	if err := h.m.Update(ctx, "slash", false, nil); err != nil {
		t.Fatal(err)
	}

	if st := h.m.Status(ctx, "slash"); !st.Installed || !st.UpToDate {
		t.Errorf("after install: %+v", st)
	}

	// Base archives are linked, not copied.
	src, _ := os.Stat(filepath.Join(h.base, "d2data.mpq"))
	dst, err := os.Stat(filepath.Join(h.base, "slash", "d2data.mpq"))
	if err != nil || !os.SameFile(src, dst) {
		t.Errorf("d2data.mpq not linked: %v", err)
	}

	// The default maphack is installed; its settings work.
	if h.read(t, "BH.dll") != "bh one" {
		t.Error("default maphack not installed")
	}
	if err := h.m.SetSetting(ctx, "slash", "reveal", true); err != nil {
		t.Fatal(err)
	}
	if got := h.read(t, "BH_settings.cfg"); got != "Reveal Map: True\r\n\r\n" {
		t.Errorf("BH_settings.cfg = %q", got)
	}

	// D2GL is off, so its setting is unavailable.
	values, _ := h.m.Settings(ctx, "slash")
	for _, v := range values {
		if v.Setting.ID == "cursor" && v.Available {
			t.Error("d2gl setting available while d2gl is off")
		}
		if v.Setting.ID == "reveal" && v.Value != true {
			t.Errorf("reveal = %v", v.Value)
		}
	}

	// Switch maphack version and turn d2gl on.
	h.m.SetComponent(ctx, "slash", "maphack", "2")
	h.m.SetComponent(ctx, "slash", "d2gl", "1.3.3")
	if err := h.m.Update(ctx, "slash", false, nil); err != nil {
		t.Fatal(err)
	}

	if h.exists("BH.dll") || h.read(t, "BH2.dll") != "bh two" {
		t.Error("maphack switch did not remove version 1 and install version 2")
	}
	if got := h.read(t, "glide3x.dll"); got != "d2gl glide" {
		t.Errorf("glide3x.dll = %q; the later layer should win", got)
	}
	if got := h.read(t, "BH_settings.cfg"); got != "Reveal Map: True\r\n\r\n" {
		t.Errorf("the player's settings file was replaced: %q", got)
	}

	// And it stays settled: no fight between layers on the next check.
	if st := h.m.Status(ctx, "slash"); !st.UpToDate {
		t.Errorf("not up to date after the switch: %+v", st)
	}

	// Turning d2gl off restores the base glide on the next update.
	h.m.SetComponent(ctx, "slash", "d2gl", "")
	if err := h.m.Update(ctx, "slash", false, nil); err != nil {
		t.Fatal(err)
	}
	if got := h.read(t, "glide3x.dll"); got != "base glide" || h.exists("d2gl.ini") {
		t.Errorf("after turning d2gl off: glide3x.dll = %q, d2gl.ini present = %v", got, h.exists("d2gl.ini"))
	}

	// Play passes the flag setting and starts the requested boxes.
	h.m.SetSetting(ctx, "slash", "windowed", true)
	h.m.SetInstances(ctx, "slash", 2, false)
	if err := h.m.Play(ctx, "slash"); err != nil {
		t.Fatal(err)
	}
	if len(h.started) != 2 || strings.Join(h.started[0].Args, " ") != "-skiptobnet -w" {
		t.Errorf("started %+v", h.started)
	}
}

// A component installed without a record (an older layout, or before the
// server split it out) is still removed when turned off, but only files that
// exactly match it.
func TestTurnOffUnrecordedComponent(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.m.Servers(ctx)

	h.m.SetComponent(ctx, "slash", "d2gl", "1.3.3")
	if err := h.m.Update(ctx, "slash", false, nil); err != nil {
		t.Fatal(err)
	}

	// Forget the record, as if the files came from elsewhere, and edit one of
	// them by hand.
	h.m.mu.Lock()
	h.m.state.Server("slash").Applied = map[string]string{}
	h.m.mu.Unlock()
	os.WriteFile(filepath.Join(h.base, "slash", "d2gl.ini"), []byte("[Screen]\nunlock_cursor=true\n"), 0o644)

	h.m.SetComponent(ctx, "slash", "d2gl", "")
	if st := h.m.Status(ctx, "slash"); st.UpToDate {
		t.Fatalf("turning d2gl off shows nothing to do: %+v", st)
	}
	if err := h.m.Update(ctx, "slash", false, nil); err != nil {
		t.Fatal(err)
	}

	if got := h.read(t, "glide3x.dll"); got != "base glide" {
		t.Errorf("glide3x.dll = %q, want the base layer's", got)
	}
	if !h.exists("d2gl.ini") {
		t.Error("the hand-edited d2gl.ini was removed; without a record only exact matches may go")
	}
}

// Verify finds a file damaged in a way the remembered hashes can't see: same
// size, same modification time. The player's own files aren't flagged.
func TestVerify(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.m.Servers(ctx)

	if err := h.m.Update(ctx, "slash", false, nil); err != nil {
		t.Fatal(err)
	}

	// Remember the hashes, with times old enough to be trusted.
	game := filepath.Join(h.base, "slash", "Game.exe")
	old := time.Now().Add(-time.Hour)
	for _, f := range []string{"Game.exe", "glide3x.dll", "BH.dll", "BH_settings.cfg"} {
		os.Chtimes(filepath.Join(h.base, "slash", f), old, old)
	}
	if st := h.m.Status(ctx, "slash"); !st.UpToDate {
		t.Fatalf("after install: %+v", st)
	}

	os.WriteFile(game, []byte("game 1.13X"), 0o644)
	os.Chtimes(game, old, old)
	os.WriteFile(filepath.Join(h.base, "slash", "BH_settings.cfg"), []byte("Reveal Map: True\r\n"), 0o644)

	if st := h.m.Status(ctx, "slash"); !st.UpToDate {
		t.Fatalf("the cached check saw the damage, so this test proves nothing: %+v", st)
	}

	st := h.m.Verify(ctx, "slash")
	if st.UpToDate || st.UpdateFiles != 1 || st.UpdateBytes != int64(len("game 1.13c")) {
		t.Fatalf("Verify = %+v, want Game.exe to repair", st)
	}

	if err := h.m.Update(ctx, "slash", false, nil); err != nil {
		t.Fatal(err)
	}
	if h.read(t, "Game.exe") != "game 1.13c" || h.read(t, "BH_settings.cfg") != "Reveal Map: True\r\n" {
		t.Error("repair didn't restore Game.exe, or replaced the player's settings")
	}
	if st := h.m.Verify(ctx, "slash"); !st.UpToDate {
		t.Errorf("after repair: %+v", st)
	}
}

func TestSetFavouriteOrder(t *testing.T) {
	h := newHarness(t)
	for _, id := range []string{"a", "b", "c"} {
		h.m.SetFavourite(id, true)
	}

	if err := h.m.SetFavouriteOrder([]string{"c", "a", "b"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(h.m.Favourites(), ","); got != "c,a,b" {
		t.Errorf("order = %s", got)
	}

	for _, bad := range [][]string{
		{"a", "b"},           // one missing
		{"a", "b", "b"},      // duplicate
		{"a", "b", "x"},      // not pinned
		{"a", "b", "c", "d"}, // extra
	} {
		if err := h.m.SetFavouriteOrder(bad); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
	if got := strings.Join(h.m.Favourites(), ","); got != "c,a,b" {
		t.Errorf("a rejected order changed the pins: %s", got)
	}
}

func TestFavouritesCapped(t *testing.T) {
	h := newHarness(t)

	for _, id := range []string{"a", "b", "c"} {
		if err := h.m.SetFavourite(id, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.m.SetFavourite("d", true); !errors.Is(err, ErrTooManyFavourites) {
		t.Errorf("fourth favourite: %v", err)
	}

	// Re-pinning one already pinned keeps it, moved to the end.
	if err := h.m.SetFavourite("a", true); err != nil {
		t.Errorf("re-pin: %v", err)
	}
	if got := strings.Join(h.m.Favourites(), ","); got != "b,c,a" {
		t.Errorf("favourites = %s", got)
	}

	h.m.SetFavourite("b", false)
	if err := h.m.SetFavourite("d", true); err != nil {
		t.Errorf("pin after unpinning: %v", err)
	}
}

// A broken profile in the listing shows its error without hiding the rest.
func TestBrokenListedProfile(t *testing.T) {
	h := newHarness(t)

	os.WriteFile(filepath.Join(h.listing, "broken.json"), []byte(`{"schema":1,"id":"broken","name":"Broken"}`), 0o644)

	servers, err := h.m.Servers(context.Background())
	if err != nil || len(servers) != 2 {
		t.Fatalf("servers = %+v, %v", servers, err)
	}

	for _, s := range servers {
		switch s.ID {
		case "broken":
			if s.Error == "" || s.Profile != nil {
				t.Errorf("broken profile: %+v", s)
			}
		case "slash":
			if s.Error != "" || !s.Verified {
				t.Errorf("good profile: %+v", s)
			}
		}
	}
}

func TestOfflineUsesCachedProfile(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if _, err := h.m.Servers(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.m.Update(ctx, "slash", false, nil); err != nil {
		t.Fatal(err)
	}

	h.site.mu.Lock()
	h.site.down = true
	h.site.mu.Unlock()

	if _, err := h.m.Profile(ctx, "slash"); err != nil {
		t.Errorf("offline profile: %v", err)
	}

	// Still playable offline from what's installed.
	if err := h.m.Play(ctx, "slash"); err != nil {
		t.Errorf("offline play: %v", err)
	}
}

func TestMissingBaseArchive(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.m.Servers(ctx)

	os.Remove(filepath.Join(h.base, "d2exp.mpq"))

	err := h.m.Update(ctx, "slash", false, nil)
	if err == nil || !strings.Contains(err.Error(), "d2exp.mpq") {
		t.Errorf("missing d2exp.mpq: %v", err)
	}

	var copyErr ErrNeedsCopy
	if errors.As(err, &copyErr) {
		t.Error("reported as needing a copy")
	}
}

func TestFirstRun(t *testing.T) {
	h := newHarness(t)

	if !h.m.FirstRun() {
		t.Fatal("a new launcher isn't on its first run")
	}

	// Anyone with a pinned server has used the launcher before.
	h.m.SetFavourite("a", true)
	if h.m.FirstRun() {
		t.Error("first run with a server pinned")
	}
	h.m.SetFavourite("a", false)

	// More than can be pinned changes nothing.
	if err := h.m.FinishWelcome([]string{"a", "b", "c", "d"}); !errors.Is(err, ErrTooManyFavourites) {
		t.Errorf("four pins: %v", err)
	}
	if !h.m.FirstRun() || len(h.m.Favourites()) != 0 {
		t.Error("a refused welcome changed the state")
	}

	if err := h.m.FinishWelcome([]string{"b", "a", "b"}); err != nil {
		t.Fatal(err)
	}
	if h.m.FirstRun() {
		t.Error("first run after the welcome")
	}
	if got := strings.Join(h.m.Favourites(), ","); got != "b,a" {
		t.Errorf("favourites = %s, want the picks in order", got)
	}

	// Both are remembered.
	m, err := New(h.store, DirListing(h.listing), launch.New(nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	if m.FirstRun() || strings.Join(m.Favourites(), ",") != "b,a" {
		t.Errorf("after a restart: first run %v, favourites %v", m.FirstRun(), m.Favourites())
	}
}

func TestFinishWelcomeKeepsStateWhenSaveFails(t *testing.T) {
	h := newHarness(t)

	// A folder where state.json goes makes every save fail.
	state := filepath.Join(h.store.Dir(), "state.json")
	os.Remove(state)
	if err := os.Mkdir(state, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := h.m.FinishWelcome([]string{"a"}); err == nil {
		t.Fatal("save into a folder succeeded")
	}
	if !h.m.FirstRun() || len(h.m.Favourites()) != 0 {
		t.Errorf("failed welcome left first run %v, favourites %v", h.m.FirstRun(), h.m.Favourites())
	}
}
