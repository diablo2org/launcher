package pack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diablo2org/launcher/internal/fetch"
	"github.com/diablo2org/launcher/internal/spec"
)

func write(t *testing.T, dir, rel, body string) {
	t.Helper()

	p := filepath.Join(dir, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildManifest(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "Game.exe", "exe")
	write(t, dir, "BH_settings.cfg", "settings")
	write(t, dir, "data/global/My File.txt", "nested")
	write(t, dir, "d2data.mpq", "blizzard's")
	write(t, dir, "D2EXP.MPQ", "blizzard's")
	write(t, dir, "Save/char.d2s", "a character")
	write(t, dir, "debug.log", "noise")
	write(t, dir, "manifest.json", "{}")

	m, err := BuildManifest(dir, Options{
		Server:  "slashdiablo",
		Version: "1",
		BaseURL: "https://slashdiablo.net/files/live",
		Once:    []string{"BH_settings.cfg"},
		Exclude: []string{"*.log"},
	})
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]spec.File{}
	for _, f := range m.Files {
		got[f.Path] = f
	}

	if len(got) != 3 {
		t.Fatalf("files = %v, want Game.exe, BH_settings.cfg and the nested file only", got)
	}

	if f := got["BH_settings.cfg"]; f.Mode != spec.ModeOnce {
		t.Errorf("BH_settings.cfg mode = %q, want once", f.Mode)
	}

	nested := got["data/global/My File.txt"]
	if want := "https://slashdiablo.net/files/live/data/global/My%20File.txt"; nested.URLs[0] != want {
		t.Errorf("URL = %q, want %q", nested.URLs[0], want)
	}

	if _, sha, _ := fetch.HashFile(filepath.Join(dir, "Game.exe")); got["Game.exe"].SHA256 != sha || got["Game.exe"].Size != 3 {
		t.Errorf("Game.exe = %+v", got["Game.exe"])
	}

	// The result must be a valid manifest for a profile on that host.
	file := filepath.Join(t.TempDir(), "manifest.json")
	if err := WriteManifest(m, file); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	profile := &spec.Profile{ID: "slashdiablo", Hosts: []string{"slashdiablo.net"}}
	if _, err := spec.ParseManifest(data, profile); err != nil {
		t.Errorf("generated manifest is not valid: %v", err)
	}
}

func TestBuildManifestOnly(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "Game.exe", "exe")
	write(t, dir, "BH.dll", "bh")
	write(t, dir, "BH.cfg", "cfg")

	m, err := BuildManifest(dir, Options{Server: "x", Version: "1", BaseURL: "https://a.net", Only: []string{"bh*"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Files) != 2 || m.Files[0].Path != "BH.cfg" || m.Files[1].Path != "BH.dll" {
		t.Errorf("files = %+v", m.Files)
	}
}

func TestBuildManifestRejectsClashes(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "nul.txt", "x")

	if _, err := BuildManifest(dir, Options{Server: "x", Version: "1", BaseURL: "https://a.net"}); err == nil {
		t.Error("reserved file name accepted")
	}

	for _, base := range []string{"ftp://a.net", "http://a.net/live", "https:///live"} {
		if _, err := BuildManifest(t.TempDir(), Options{BaseURL: base}); err == nil {
			t.Errorf("base URL %s accepted", base)
		}
	}

	for _, base := range []string{"https://a.net/live?token=x", "https://a.net/live#x", "https://a.net/live?"} {
		if _, err := BuildManifest(t.TempDir(), Options{BaseURL: base}); err == nil {
			t.Errorf("base URL %s accepted", base)
		}
	}
}

func TestInitProfile(t *testing.T) {
	data, err := InitProfile(InitOptions{
		ID: "myserver", Name: "My Server", Gateway: "play.myserver.net",
		ManifestURL: "https://files.myserver.net/live/manifest.json",
	})
	if err != nil {
		t.Fatal(err)
	}

	p, err := spec.ParseProfile(data)
	if err != nil {
		t.Fatalf("starter profile is not valid: %v", err)
	}
	if p.Hosts[0] != "files.myserver.net" || p.Gateways[0].Host != "play.myserver.net" || p.Game.Version != "1.13c" {
		t.Errorf("profile = %+v", p)
	}

	data, err = InitProfile(InitOptions{
		ID: "myserver", Name: "My Server", Gateway: "play.myserver.net",
		ManifestURL: "https://github.com/me/files/releases/download/v1/manifest.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err = spec.ParseProfile(data)
	if err != nil {
		t.Fatalf("starter profile is not valid: %v", err)
	}
	if strings.Join(p.Hosts, " ") != "github.com release-assets.githubusercontent.com objects.githubusercontent.com" {
		t.Errorf("hosts for a GitHub release = %v", p.Hosts)
	}

	if _, err := InitProfile(InitOptions{ID: "Bad Id", Name: "x", Gateway: "a.net", ManifestURL: "https://a.net/m.json"}); err == nil {
		t.Error("invalid id accepted")
	}
	if _, err := InitProfile(InitOptions{ID: "ok", Name: "x", Gateway: "a.net", ManifestURL: "http://a.net/m.json"}); err == nil {
		t.Error("http manifest URL accepted")
	}
}
