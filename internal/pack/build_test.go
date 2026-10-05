package pack

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/diablo2org/launcher/internal/spec"
)

const buildProfile = `{
  "schema": 1,
  "id": "myserver",
  "name": "My Server",
  "version": 1,
  "hosts": ["files.myserver.net"],
  "game": { "version": "1.13c", "expansion": true, "baseArchives": "link", "saves": "shared" },
  "gateways": [ { "name": "My Server", "host": "play.myserver.net", "timezone": 0 } ],
  "channels": [
    { "id": "live", "name": "Live", "manifest": "https://files.myserver.net/live/manifest.json" },
    { "id": "test", "name": "Test", "manifest": "https://files.myserver.net/test/manifest.json" }
  ],
  "components": [
    {
      "id": "maphack", "name": "Maphack", "kind": "bh",
      "versions": [
        { "id": "1.9.9", "manifest": "https://files.myserver.net/maphack/1.9.9/manifest.json" },
        { "id": "1.9.8", "manifest": "https://files.myserver.net/maphack/1.9.8/manifest.json" }
      ]
    }
  ],
  "launch": { "defaultFlags": [], "allowedFlags": ["-w"], "maxInstances": 1 }
}`

// buildSetup writes a profile and a game folder, and returns a plan for them.
func buildSetup(t *testing.T) (*Plan, string) {
	t.Helper()

	root := t.TempDir()
	write(t, root, "profile.json", buildProfile)

	game := filepath.Join(root, "game")
	write(t, game, "Game.exe", "exe")
	write(t, game, "Patch_D2.mpq", "patch")
	write(t, game, "UI.ini", "ui")
	write(t, game, "BH.dll", "bh")
	write(t, game, "BH.cfg", "cfg")
	write(t, game, "readme.txt", "junk")
	write(t, game, "d2data.mpq", "blizzard's")

	plan := &Plan{
		Profile: filepath.Join(root, "profile.json"),
		Source:  Sources{game},
		Exclude: []string{"*.txt"},
		Manifests: []PlanEntry{
			{Channel: "live", Once: []string{"UI.ini"}, Exclude: []string{"BH*"}},
			{Component: "maphack", Version: "1.9.9", Only: []string{"BH*"}, Once: []string{"BH.cfg"}},
		},
	}

	return plan, root
}

func readManifest(t *testing.T, file string) *spec.Manifest {
	t.Helper()

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	profile, _ := spec.ParseProfile([]byte(buildProfile))
	m, err := spec.ParseManifest(data, profile)
	if err != nil {
		t.Fatalf("%s: %v", file, err)
	}

	return m
}

func fileList(m *spec.Manifest) string {
	var p []string
	for _, f := range m.Files {
		p = append(p, f.Path)
	}

	return strings.Join(p, ",")
}

// noPartial fails if a build left its temporary folder behind.
func noPartial(t *testing.T, out string) {
	t.Helper()

	if left, _ := filepath.Glob(out + ".partial-*"); len(left) > 0 {
		t.Errorf("partial folder left behind: %v", left)
	}
}

func TestBuild(t *testing.T) {
	plan, root := buildSetup(t)
	out := filepath.Join(root, "upload")

	result, err := Build(plan, BuildOptions{Version: "2026.10.05", Out: out})
	if err != nil {
		t.Fatal(err)
	}

	host := filepath.Join(out, "files.myserver.net")

	live := readManifest(t, filepath.Join(host, "live", "manifest.json"))
	if got := fileList(live); got != "Game.exe,Patch_D2.mpq,UI.ini" {
		t.Errorf("live files = %s", got)
	}
	if live.Version != "2026.10.05" || live.Server != "myserver" {
		t.Errorf("live = %+v", live)
	}
	for _, f := range live.Files {
		if want := "https://files.myserver.net/live/" + f.Path; f.URLs[0] != want {
			t.Errorf("%s URL = %s, want %s", f.Path, f.URLs[0], want)
		}
		if (f.Path == "UI.ini") != (f.Mode == spec.ModeOnce) {
			t.Errorf("%s mode = %q", f.Path, f.Mode)
		}
	}

	maphack := readManifest(t, filepath.Join(host, "maphack", "1.9.9", "manifest.json"))
	if got := fileList(maphack); got != "BH.cfg,BH.dll" {
		t.Errorf("maphack files = %s", got)
	}

	// Each manifest's files sit next to it, as its URLs say.
	for _, rel := range []string{"live/Game.exe", "live/UI.ini", "maphack/1.9.9/BH.dll"} {
		if _, err := os.Stat(filepath.Join(host, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s not in the output: %v", rel, err)
		}
	}
	for _, rel := range []string{"live/BH.dll", "live/readme.txt", "live/d2data.mpq"} {
		if _, err := os.Stat(filepath.Join(host, filepath.FromSlash(rel))); err == nil {
			t.Errorf("%s should not be in the output", rel)
		}
	}

	if len(result.Built) != 2 || result.Built[0].Files != 3 {
		t.Errorf("built = %+v", result.Built)
	}
	if got := strings.Join(result.NotBuilt, ","); got != "channel test,maphack 1.9.8" {
		t.Errorf("not built = %s", got)
	}

	noPartial(t, out)
}

func TestBuildFilesURL(t *testing.T) {
	plan, root := buildSetup(t)
	plan.Manifests = plan.Manifests[:1]
	plan.Manifests[0].Files = "https://files.myserver.net/files/2026"
	out := filepath.Join(root, "upload")

	if _, err := Build(plan, BuildOptions{Version: "1", Out: out}); err != nil {
		t.Fatal(err)
	}

	host := filepath.Join(out, "files.myserver.net")
	live := readManifest(t, filepath.Join(host, "live", "manifest.json"))
	if want := "https://files.myserver.net/files/2026/Game.exe"; live.Files[0].URLs[0] != want {
		t.Errorf("URL = %s, want %s", live.Files[0].URLs[0], want)
	}
	if _, err := os.Stat(filepath.Join(host, "files", "2026", "Game.exe")); err != nil {
		t.Error(err)
	}
}

func TestBuildLayersSources(t *testing.T) {
	plan, root := buildSetup(t)
	patch := filepath.Join(root, "current")
	write(t, patch, "game.exe", "patched exe")
	write(t, patch, "SlashDiablo.dll", "slash")
	plan.Manifests[0].Source = Sources{plan.Source[0], patch}
	out := filepath.Join(root, "upload")

	if _, err := Build(plan, BuildOptions{Version: "1", Out: out}); err != nil {
		t.Fatal(err)
	}

	live := filepath.Join(out, "files.myserver.net", "live")
	m := readManifest(t, filepath.Join(live, "manifest.json"))
	if got := fileList(m); got != "game.exe,Patch_D2.mpq,SlashDiablo.dll,UI.ini" {
		t.Errorf("files = %s", got)
	}

	data, err := os.ReadFile(filepath.Join(live, "game.exe"))
	if err != nil || string(data) != "patched exe" {
		t.Errorf("game.exe = %q, %v; want the later folder's", data, err)
	}
	if m.Files[0].Size != int64(len("patched exe")) {
		t.Errorf("manifest has the earlier folder's game.exe: %+v", m.Files[0])
	}
}

func TestBuildRejects(t *testing.T) {
	tests := []struct {
		name   string
		change func(p *Plan, root string) string
		want   string
	}{
		{"unknown channel", func(p *Plan, _ string) string {
			p.Manifests[0].Channel = "beta"
			return ""
		}, "no channel"},
		{"unknown version", func(p *Plan, _ string) string {
			p.Manifests[1].Version = "2.0"
			return ""
		}, "no version"},
		{"component without version", func(p *Plan, _ string) string {
			p.Manifests[1].Version = ""
			return ""
		}, "no version"},
		{"listed twice", func(p *Plan, _ string) string {
			p.Manifests = append(p.Manifests, p.Manifests[0])
			return ""
		}, "twice"},
		{"nothing matched", func(p *Plan, _ string) string {
			p.Manifests[1].Only = []string{"nothing*"}
			return ""
		}, "no files"},
		{"output exists", func(_ *Plan, root string) string {
			os.MkdirAll(filepath.Join(root, "upload"), 0o755)
			return ""
		}, "already exists"},
		{"output inside source", func(p *Plan, _ string) string {
			return filepath.Join(p.Source[0], "upload")
		}, "inside the source"},
		{"different files at one URL", func(p *Plan, root string) string {
			other := filepath.Join(root, "other")
			write(t, other, "BH.dll", "another bh")
			p.Manifests[0].Exclude = nil
			p.Manifests[1].Source = Sources{other}
			p.Manifests[1].Files = "https://files.myserver.net/live"
			return ""
		}, "different file"},
		{"query in files URL", func(p *Plan, _ string) string {
			p.Manifests[1].Files = "https://files.myserver.net/maphack?token=x"
			return ""
		}, "query"},
		{"fragment in files URL", func(p *Plan, _ string) string {
			p.Manifests[1].Files = "https://files.myserver.net/maphack#x"
			return ""
		}, "fragment"},
		{"files off the profile's hosts", func(p *Plan, _ string) string {
			p.Manifests[1].Files = "https://elsewhere.net/maphack"
			return ""
		}, "hosts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, root := buildSetup(t)
			out := tt.change(plan, root)
			if out == "" {
				out = filepath.Join(root, "upload")
			}

			_, err := Build(plan, BuildOptions{Version: "1", Out: out})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want one mentioning %q", err, tt.want)
			}

			// A failed build leaves nothing that looks ready to upload.
			noPartial(t, out)
			if entries, _ := os.ReadDir(out); len(entries) > 0 {
				t.Errorf("output written: %v", entries)
			}
		})
	}
}

func TestBuildLeavesOtherFoldersAlone(t *testing.T) {
	plan, root := buildSetup(t)
	out := filepath.Join(root, "upload")

	// Folders that look like an earlier build's leftovers aren't this build's
	// to remove.
	write(t, root, "upload.partial/keep", "someone's")
	write(t, root, "upload.partial-123/keep", "someone's")

	if _, err := Build(plan, BuildOptions{Version: "1", Out: out}); err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{"upload.partial", "upload.partial-123"} {
		if _, err := os.Stat(filepath.Join(root, dir, "keep")); err != nil {
			t.Errorf("%s was touched: %v", dir, err)
		}
	}
}

// link makes link point at target, as a symlink or, on Windows without the
// privilege for one, a junction.
func link(t *testing.T, target, link string) {
	t.Helper()

	err := os.Symlink(target, link)
	if err == nil {
		return
	}
	if runtime.GOOS != "windows" {
		t.Skipf("can't make a symlink here: %v", err)
	}
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Skipf("can't make a symlink or junction here: %v %s", err, out)
	}
}

func TestBuildRejectsOutputLinkedIntoSource(t *testing.T) {
	for _, nested := range []string{"", "data/global"} {
		t.Run("source/"+nested, func(t *testing.T) {
			plan, root := buildSetup(t)

			// root/link points into the game folder, so root/link/upload is
			// inside it although its path doesn't say so.
			target := filepath.Join(plan.Source[0], filepath.FromSlash(nested))
			os.MkdirAll(target, 0o755)
			link(t, target, filepath.Join(root, "link"))
			out := filepath.Join(root, "link", "upload")

			_, err := Build(plan, BuildOptions{Version: "1", Out: out})
			if err == nil || !strings.Contains(err.Error(), "inside the source") {
				t.Fatalf("err = %v, want one about the source", err)
			}
			noPartial(t, out)
		})
	}
}

func TestLoadPlan(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "build.json", `{
  "profile": "profile.json",
  "source": "game",
  "manifests": [
    { "channel": "live", "source": ["1.13c", "current"] },
    { "component": "d2gl", "version": "1.3.3", "source": "C:/d2gl" }
  ]
}`)

	p, err := LoadPlan(filepath.Join(dir, "build.json"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Profile != filepath.Join(dir, "profile.json") || p.Source[0] != filepath.Join(dir, "game") {
		t.Errorf("paths not resolved against the plan: %+v", p)
	}
	if !filepath.IsAbs(p.Manifests[1].Source[0]) || p.Manifests[0].Source[1] != filepath.Join(dir, "current") {
		t.Errorf("source = %s", p.Manifests[1].Source)
	}

	write(t, dir, "typo.json", `{ "profile": "p.json", "manifests": [ { "chanel": "live" } ] }`)
	if _, err := LoadPlan(filepath.Join(dir, "typo.json")); err == nil {
		t.Error("unknown field accepted")
	}

	write(t, dir, "trailing.json", `{ "profile": "p.json", "manifests": [ { "channel": "live" } ] } { "oops": 1 }`)
	if _, err := LoadPlan(filepath.Join(dir, "trailing.json")); err == nil {
		t.Error("text after the plan accepted")
	}

	write(t, dir, "empty.json", `{ "profile": "p.json", "manifests": [] }`)
	if _, err := LoadPlan(filepath.Join(dir, "empty.json")); err == nil {
		t.Error("plan without manifests accepted")
	}
}
