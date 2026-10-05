package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/diablo2org/launcher/internal/fetch"
	"github.com/diablo2org/launcher/internal/spec"
)

type fixture struct {
	dir    string
	client *fetch.Client
	url    string
	served map[string][]byte
	hits   map[string]int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	f := &fixture{dir: t.TempDir(), served: map[string][]byte{}, hits: map[string]int{}}

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits[r.URL.Path]++
		body, ok := f.served[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(srv.Close)

	u, _ := url.Parse(srv.URL)
	f.url = srv.URL
	f.client = fetch.New([]string{u.Hostname()}, fetch.WithHTTPClient(srv.Client()))

	return f
}

// serve publishes body at /path and returns its manifest entry.
func (f *fixture) serve(path string, body string, mode string) spec.File {
	f.served["/"+path] = []byte(body)
	sum := sha256.Sum256([]byte(body))

	return spec.File{
		Path:   path,
		Size:   int64(len(body)),
		SHA256: hex.EncodeToString(sum[:]),
		URLs:   []string{f.url + "/" + path},
		Mode:   mode,
	}
}

func (f *fixture) write(t *testing.T, rel, body string) {
	t.Helper()

	path := filepath.Join(f.dir, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) read(t *testing.T, rel string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(f.dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}

func (f *fixture) sync(t *testing.T, m *spec.Manifest, cache Cache) *Plan {
	t.Helper()

	plan, err := PlanManifest(f.dir, m, cache)
	if err != nil {
		t.Fatal(err)
	}

	if err := Apply(context.Background(), f.client, f.dir, plan, cache, nil); err != nil {
		t.Fatal(err)
	}

	return plan
}

func TestSync(t *testing.T) {
	f := newFixture(t)

	f.write(t, "Game.exe", "old game")
	f.write(t, "BH.cfg", "player's own config")
	f.write(t, "Old.dll", "stale")
	f.write(t, "Unlisted.txt", "not ours")

	m := &spec.Manifest{Server: "test", Files: []spec.File{
		f.serve("Game.exe", "new game", ""),
		f.serve("data/global/new.txt", "nested", ""),
		f.serve("BH.cfg", "server default config", spec.ModeOnce),
		f.serve("BH_settings.cfg", "fresh settings", spec.ModeOnce),
		{Path: "Old.dll", Mode: spec.ModeDelete},
	}}

	cache := Cache{}
	plan := f.sync(t, m, cache)

	if len(plan.Actions) != 4 {
		t.Errorf("planned %d actions, want 4: %+v", len(plan.Actions), plan.Actions)
	}
	if want := int64(len("new game") + len("nested") + len("fresh settings")); plan.Bytes != want {
		t.Errorf("plan bytes = %d, want %d", plan.Bytes, want)
	}

	for rel, want := range map[string]string{
		"Game.exe":            "new game",
		"data/global/new.txt": "nested",
		"BH.cfg":              "player's own config",
		"BH_settings.cfg":     "fresh settings",
		"Unlisted.txt":        "not ours",
	} {
		if got := f.read(t, rel); got != want {
			t.Errorf("%s = %q, want %q", rel, got, want)
		}
	}

	if _, err := os.Stat(filepath.Join(f.dir, "Old.dll")); !os.IsNotExist(err) {
		t.Error("Old.dll not deleted")
	}

	// Nothing left to do, and nothing downloaded again.
	before := f.hits["/Game.exe"]
	if again := f.sync(t, m, cache); len(again.Actions) != 0 {
		t.Errorf("second run planned %+v", again.Actions)
	}
	if f.hits["/Game.exe"] != before {
		t.Error("an up-to-date file was downloaded again")
	}

	// No temporary files left behind.
	filepath.Walk(f.dir, func(path string, info os.FileInfo, err error) error {
		if filepath.Ext(path) == ".launcher-download" {
			t.Errorf("left behind %s", path)
		}
		return nil
	})
}

func TestCacheInvalidatedByChange(t *testing.T) {
	f := newFixture(t)
	m := &spec.Manifest{Files: []spec.File{f.serve("Game.exe", "good", "")}}

	cache := Cache{}
	f.sync(t, m, cache)

	// Tamper with the file behind the cache's back, same size.
	f.write(t, "Game.exe", "evil")

	plan, err := PlanManifest(f.dir, m, cache)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Reason != "changed" {
		t.Errorf("tampered file not caught: %+v", plan.Actions)
	}
}

// Only files that haven't been touched for a moment are cached; a fresh one
// is hashed again, since its modification time can't be trusted yet.
func TestCacheOnlySettledFiles(t *testing.T) {
	f := newFixture(t)
	f.write(t, "fresh.dll", "a")
	f.write(t, "old.dll", "b")

	old := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(f.dir, "old.dll"), old, old)

	m := &spec.Manifest{Files: []spec.File{f.serve("fresh.dll", "a", ""), f.serve("old.dll", "b", "")}}

	cache := Cache{}
	if _, err := PlanManifest(f.dir, m, cache); err != nil {
		t.Fatal(err)
	}

	if _, ok := cache["old.dll"]; !ok {
		t.Error("settled file not cached")
	}
	if _, ok := cache["fresh.dll"]; ok {
		t.Error("freshly modified file cached")
	}
}

func TestMirrorFallback(t *testing.T) {
	f := newFixture(t)

	file := f.serve("Game.exe", "from mirror", "")
	file.URLs = append([]string{f.url + "/missing/Game.exe"}, file.URLs...)

	f.sync(t, &spec.Manifest{Files: []spec.File{file}}, nil)

	if got := f.read(t, "Game.exe"); got != "from mirror" {
		t.Errorf("Game.exe = %q", got)
	}
}

func TestBadDownloadLeavesOldFile(t *testing.T) {
	f := newFixture(t)
	f.write(t, "Game.exe", "old but working")

	file := f.serve("Game.exe", "new", "")
	file.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"

	plan, _ := PlanManifest(f.dir, &spec.Manifest{Files: []spec.File{file}}, nil)
	if err := Apply(context.Background(), f.client, f.dir, plan, nil, nil); err == nil {
		t.Fatal("expected a hash error")
	}

	if got := f.read(t, "Game.exe"); got != "old but working" {
		t.Errorf("Game.exe = %q, the old file should be untouched", got)
	}
}

// Replacing a hard-linked file must not change the file it links to, which
// is how base archives are shared with the retail install.
func TestReplacesHardLinkWithoutWritingThrough(t *testing.T) {
	f := newFixture(t)

	base := filepath.Join(t.TempDir(), "d2data.mpq")
	os.WriteFile(base, []byte("retail archive"), 0o644)
	if err := os.Link(base, filepath.Join(f.dir, "d2data.mpq")); err != nil {
		t.Skipf("hard links not supported here: %v", err)
	}

	f.sync(t, &spec.Manifest{Files: []spec.File{f.serve("d2data.mpq", "server copy", "")}}, nil)

	if data, _ := os.ReadFile(base); string(data) != "retail archive" {
		t.Errorf("retail file changed through the link: %q", data)
	}
}

func TestPlanRemoval(t *testing.T) {
	f := newFixture(t)
	f.write(t, "BH.dll", "x")
	f.write(t, "BH.cfg", "player config")

	m := &spec.Manifest{Files: []spec.File{
		f.serve("BH.dll", "x", ""),
		f.serve("BH.cfg", "default", spec.ModeOnce),
		f.serve("BH_extra.dll", "not installed", ""),
	}}

	plan, err := PlanRemoval(f.dir, m)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Actions) != 1 || plan.Actions[0].File.Path != "BH.dll" {
		t.Errorf("removal plan = %+v, want only BH.dll", plan.Actions)
	}
}

func TestPlanMatchingRemoval(t *testing.T) {
	f := newFixture(t)
	f.write(t, "BH.dll", "bh one")
	f.write(t, "BH_patched.dll", "changed by hand")

	m := &spec.Manifest{Files: []spec.File{
		f.serve("BH.dll", "bh one", ""),
		f.serve("BH_patched.dll", "original", ""),
		f.serve("missing.dll", "x", ""),
	}}

	plan, err := PlanMatchingRemoval(f.dir, m, Cache{})
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Actions) != 1 || plan.Actions[0].File.Path != "BH.dll" {
		t.Errorf("plan = %+v; only the exact match should be removed", plan.Actions)
	}
}

func TestInUse(t *testing.T) {
	if !errors.Is(inUse("Game.exe", os.ErrPermission), ErrInUse) {
		t.Error("permission error not reported as in use")
	}
}
