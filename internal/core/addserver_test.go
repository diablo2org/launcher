package core

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/diablo2org/launcher/internal/fetch"
	"github.com/diablo2org/launcher/internal/launch"
	"github.com/diablo2org/launcher/internal/store"
)

// unlisted returns a manager with an empty listing, talking to h's site, and
// publishes h's profile on the site for adding by URL.
func unlisted(t *testing.T, h *harness) *Manager {
	t.Helper()

	h.site.json("/profile.json", h.profile)

	st, _ := store.Open(t.TempDir())
	m, err := New(st, DirListing(t.TempDir()), launch.New(&fakeRegistry{values: map[string]string{}}, func(launch.Box) error { return nil }),
		fetch.WithHTTPClient(h.site.srv.Client()))
	if err != nil {
		t.Fatal(err)
	}

	return m
}

func TestAddServerByURL(t *testing.T) {
	h := newHarness(t)
	m := unlisted(t, h)
	ctx := context.Background()

	p, err := m.AddServer(ctx, h.site.url("/profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "slash" {
		t.Errorf("added %q", p.ID)
	}

	servers, err := m.Servers(ctx)
	if err != nil || len(servers) != 1 || servers[0].Verified || servers[0].Profile == nil {
		t.Fatalf("servers = %+v, %v; want one unverified server", servers, err)
	}

	// Survives a restart through the saved state.
	m2, _ := New(m.store, DirListing(t.TempDir()), m.launcher, fetch.WithHTTPClient(h.site.srv.Client()))
	if servers, _ := m2.Servers(ctx); len(servers) != 1 {
		t.Errorf("after restart: %+v", servers)
	}

	if err := m.RemoveServer("slash"); err != nil {
		t.Fatal(err)
	}
	if servers, _ := m.Servers(ctx); len(servers) != 0 {
		t.Errorf("after remove: %+v", servers)
	}
}

// A profile published as a GitHub release asset is fetched through a redirect
// to GitHub's asset CDN, so those hosts are allowed for a github.com URL only.
func TestProfileHosts(t *testing.T) {
	for raw, want := range map[string]string{
		"https://github.com/o/r/releases/download/live/p.json":  "github.com release-assets.githubusercontent.com objects.githubusercontent.com",
		"https://GitHub.com/o/r/releases/download/live/p.json":  "github.com release-assets.githubusercontent.com objects.githubusercontent.com",
		"https://files.example.net/p.json":                      "files.example.net",
		"https://raw.githubusercontent.com/o/r/main/p.json":     "raw.githubusercontent.com",
		"https://github.com.evil.example/o/r/releases/x/p.json": "github.com.evil.example",
	} {
		u, _ := url.Parse(raw)
		if got := strings.Join(profileHosts(u), " "); got != want {
			t.Errorf("profileHosts(%s) = %s, want %s", raw, got, want)
		}
	}
}

func TestAddServerRejectsBadURLs(t *testing.T) {
	h := newHarness(t)
	m := unlisted(t, h)
	ctx := context.Background()

	if _, err := m.AddServer(ctx, strings.Replace(h.site.url("/profile.json"), "https", "http", 1)); err == nil {
		t.Error("http URL accepted")
	}
	if _, err := m.AddServer(ctx, h.site.url("/missing.json")); err == nil {
		t.Error("missing profile accepted")
	}
}

// A server added by URL can't be rolled back to an older profile by whoever
// controls its host.
func TestAddedServerRollbackRefused(t *testing.T) {
	h := newHarness(t)
	m := unlisted(t, h)
	ctx := context.Background()

	if _, err := m.AddServer(ctx, h.site.url("/profile.json")); err != nil {
		t.Fatal(err)
	}

	h.profile["version"] = 1
	h.site.json("/profile.json", h.profile)

	servers, _ := m.Servers(ctx)
	if len(servers) != 1 || !strings.Contains(servers[0].Error, "rollback") {
		t.Errorf("older profile accepted: %+v", servers)
	}
}

func TestAddServerAlreadyListed(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.m.Servers(ctx)
	h.site.json("/profile.json", h.profile)

	if _, err := h.m.AddServer(ctx, h.site.url("/profile.json")); !errors.Is(err, ErrAlreadyListed) {
		t.Errorf("adding a listed server: %v", err)
	}
	if err := h.m.RemoveServer("slash"); err == nil {
		t.Error("removed a listed server")
	}
}
