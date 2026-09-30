package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d2org/launcher/internal/fetch"
	"github.com/d2org/launcher/internal/store"
)

// servers/index.json must match servers/*.json, so the published listing is
// never behind the profiles merged into the repo.
func TestIndexUpToDate(t *testing.T) {
	want, err := BuildIndex("../../servers")
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile("../../servers/index.json")
	if err != nil {
		t.Fatalf("servers/index.json: %v (run: go run ./cmd/listing)", err)
	}

	if strings.ReplaceAll(string(got), "\r\n", "\n") != string(want) {
		t.Error("servers/index.json is out of date; run: go run ./cmd/listing")
	}
}

func TestBuildIndex(t *testing.T) {
	example, err := os.ReadFile("../../examples/slashdiablo/profile.json")
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "slashdiablo.json"), example, 0o644)

	data, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil || len(idx.Servers) != 1 || rawID(idx.Servers[0], "") != "slashdiablo" {
		t.Errorf("index = %s, %v", data, err)
	}

	// A file not named after its server's id is refused.
	os.WriteFile(filepath.Join(dir, "other.json"), example, 0o644)
	if _, err := BuildIndex(dir); err == nil || !strings.Contains(err.Error(), "file name") {
		t.Errorf("misnamed profile: %v", err)
	}
	os.Remove(filepath.Join(dir, "other.json"))

	// And so is an invalid profile.
	os.WriteFile(filepath.Join(dir, "broken.json"), []byte(`{"schema":1,"id":"broken"}`), 0o644)
	if _, err := BuildIndex(dir); err == nil {
		t.Error("invalid profile accepted into the index")
	}
}

func TestRemoteListing(t *testing.T) {
	example, err := os.ReadFile("../../examples/slashdiablo/profile.json")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "slashdiablo.json"), example, 0o644)
	body, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}

	down := false
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		w.Write(body)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	st, _ := store.Open(t.TempDir())
	l := RemoteListing{
		URL:    srv.URL + "/servers/index.json",
		Client: fetch.New([]string{u.Hostname()}, fetch.WithHTTPClient(srv.Client())),
		Store:  st,
	}

	profiles, err := l.Profiles(context.Background())
	if err != nil || len(profiles) != 1 {
		t.Fatalf("profiles = %d, %v", len(profiles), err)
	}

	down = true
	if profiles, err := l.Profiles(context.Background()); err != nil || len(profiles) != 1 {
		t.Errorf("offline = %d, %v; the last copy should be used", len(profiles), err)
	}
}
