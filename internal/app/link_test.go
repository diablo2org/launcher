package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/diablo2org/launcher/internal/core"
	"github.com/diablo2org/launcher/internal/fetch"
	"github.com/diablo2org/launcher/internal/launch"
	"github.com/diablo2org/launcher/internal/store"
)

func TestParseAddLink(t *testing.T) {
	for _, tt := range []struct {
		link, want string
	}{
		{"diablo2org://add?profile=https%3A%2F%2Fexample.net%2Fresurgence.json", "https://example.net/resurgence.json"},
		{"diablo2org://add/?profile=https://example.net/p.json", "https://example.net/p.json"},
		{"DIABLO2ORG://ADD?profile=https://example.net/p.json", "https://example.net/p.json"},
		{"diablo2org:add?profile=https://example.net/p.json", "https://example.net/p.json"},

		{"diablo2org://add?profile=http://example.net/p.json", ""},
		{"diablo2org://add?profile=https://user:pw@example.net/p.json", ""},
		{"diablo2org://add?profile=file:///C:/Windows/win.ini", ""},
		{"diablo2org://add", ""},
		{"diablo2org://remove?profile=https://example.net/p.json", ""},
		{"https://example.net/p.json", ""},
		{"diablo2org://add?profile=https://example.net/" + string(make([]byte, maxLink)), ""},
	} {
		got, err := ParseAddLink(tt.link)
		if got != tt.want || (err == nil) != (tt.want != "") {
			t.Errorf("ParseAddLink(%.60q) = %q, %v; want %q", tt.link, got, err, tt.want)
		}
	}
}

func TestLinkArg(t *testing.T) {
	if got := LinkArg([]string{`C:\launcher.exe`, "-flag", "Diablo2org://add?profile=x"}); got != "Diablo2org://add?profile=x" {
		t.Errorf("LinkArg = %q", got)
	}
	if got := LinkArg([]string{`C:\launcher.exe`}); got != "" {
		t.Errorf("LinkArg = %q", got)
	}
}

// linkHarness serves two profiles: "listed", which is also in the listing,
// and "linked", which isn't.
type linkHarness struct {
	s       *ServerService
	m       *core.Manager
	srv     *httptest.Server
	emitted []string
	focused int
}

func newLinkHarness(t *testing.T) *linkHarness {
	t.Helper()

	h := &linkHarness{}
	profiles := map[string][]byte{}
	h.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, ok := profiles[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(data)
	}))
	t.Cleanup(h.srv.Close)
	u, _ := url.Parse(h.srv.URL)

	listing := t.TempDir()
	for _, id := range []string{"listed", "linked"} {
		data, _ := json.Marshal(map[string]any{
			"schema": 1, "id": id, "name": id, "version": 1,
			"hosts":    []string{u.Hostname()},
			"game":     map[string]any{"version": "1.13c", "baseArchives": "link"},
			"gateways": []any{map[string]any{"name": id, "host": "play." + id + ".test"}},
			"channels": []any{map[string]any{"id": "live", "name": "Live", "manifest": h.srv.URL + "/live.json"}},
		})
		profiles["/"+id+".json"] = data
		if id == "listed" {
			os.WriteFile(filepath.Join(listing, id+".json"), data, 0o644)
		}
	}

	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h.m, err = core.New(st, core.DirListing(listing), launch.New(nil, nil), fetch.WithHTTPClient(h.srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	h.m.Servers(t.Context())

	h.s = NewServerService(h.m, Host{
		Emit:  func(name string, data any) { h.emitted = append(h.emitted, name) },
		Focus: func() { h.focused++ },
	})
	return h
}

func (h *linkHarness) link(id string) string {
	return "diablo2org://add?profile=" + url.QueryEscape(h.srv.URL+"/"+id+".json")
}

func TestHandleLink(t *testing.T) {
	h := newLinkHarness(t)

	// At startup the link waits quietly for the frontend.
	HandleLink(h.s, h.link("linked"), false)
	if len(h.emitted) != 0 || h.focused != 0 {
		t.Errorf("startup link emitted %v, focused %d", h.emitted, h.focused)
	}
	req := h.s.PendingLink()
	if req == nil || req.URL != h.srv.URL+"/linked.json" || req.Host != "127.0.0.1" || req.Error != "" {
		t.Fatalf("PendingLink = %+v", req)
	}
	if h.s.PendingLink() != nil {
		t.Error("a link was handed out twice")
	}

	// While running, the frontend is told and the window comes forward.
	HandleLink(h.s, "diablo2org://add?profile=http://example.net/p.json", true)
	if !slices.Equal(h.emitted, []string{"link"}) || h.focused != 1 {
		t.Errorf("emitted %v, focused %d", h.emitted, h.focused)
	}
	if req := h.s.PendingLink(); req == nil || req.URL != "" || req.Error == "" {
		t.Errorf("bad link = %+v", req)
	}

	HandleLink(h.s, "", true)
	if h.s.PendingLink() != nil || h.focused != 1 {
		t.Error("an empty link did something")
	}
}

func TestAddFromLink(t *testing.T) {
	h := newLinkHarness(t)

	id, err := h.s.AddFromLink(t.Context(), h.srv.URL+"/linked.json")
	if err != nil || id != "linked" {
		t.Fatalf("AddFromLink = %q, %v", id, err)
	}
	if !slices.Contains(h.m.Favourites(), "linked") {
		t.Errorf("added server not pinned: %v", h.m.Favourites())
	}

	// A listed server is opened, not refused.
	id, err = h.s.AddFromLink(t.Context(), h.srv.URL+"/listed.json")
	if err != nil || id != "listed" {
		t.Errorf("listed server: %q, %v", id, err)
	}

	if _, err := h.s.AddFromLink(t.Context(), h.srv.URL+"/missing.json"); err == nil {
		t.Error("a missing profile was added")
	}
}

func TestHandleLinkQueues(t *testing.T) {
	h := newLinkHarness(t)

	// Two links during startup both wait, in order; a repeat isn't queued twice.
	HandleLink(h.s, h.link("linked"), false)
	HandleLink(h.s, h.link("linked"), false)
	HandleLink(h.s, h.link("listed"), false)
	for _, want := range []string{"linked", "listed"} {
		req := h.s.PendingLink()
		if req == nil || req.URL != h.srv.URL+"/"+want+".json" {
			t.Fatalf("want %s, got %+v", want, req)
		}
	}
	if req := h.s.PendingLink(); req != nil {
		t.Errorf("extra link %+v", req)
	}

	// Past the cap the oldest go.
	for i := 0; i < maxPendingLinks+2; i++ {
		HandleLink(h.s, fmt.Sprintf("diablo2org://add?profile=https://example.net/%d.json", i), false)
	}
	if req := h.s.PendingLink(); req == nil || req.URL != "https://example.net/2.json" {
		t.Errorf("oldest kept = %+v", req)
	}
}
