package spec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return data
}

func exampleProfile(t *testing.T) *Profile {
	t.Helper()

	p, err := ParseProfile(readFile(t, "../../examples/slashdiablo/profile.json"))
	if err != nil {
		t.Fatalf("example profile: %v", err)
	}

	return p
}

// edit applies change to the example profile and returns it as JSON, so each
// test can break one thing.
func edit(t *testing.T, change func(map[string]interface{})) []byte {
	t.Helper()

	var doc map[string]interface{}
	if err := json.Unmarshal(readFile(t, "../../examples/slashdiablo/profile.json"), &doc); err != nil {
		t.Fatal(err)
	}

	change(doc)

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func TestExamples(t *testing.T) {
	p := exampleProfile(t)

	if p.ID != "slashdiablo" || !p.Game.NeedsExpansion() || p.Launch.ExePath() != "Game.exe" {
		t.Errorf("unexpected profile: %+v", p)
	}

	m, err := ParseManifest(readFile(t, "../../examples/slashdiablo/manifest.json"), p)
	if err != nil {
		t.Fatalf("example manifest: %v", err)
	}

	if m.Files[0].FileMode() != ModeReplace {
		t.Errorf("default mode = %q, want replace", m.Files[0].FileMode())
	}
}

// Every server in the listing must be a valid profile named after its id, so
// a bad pull request fails CI.
func TestListing(t *testing.T) {
	files, err := filepath.Glob("../../servers/*.json")
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range files {
		if filepath.Base(path) == "index.json" {
			continue
		}

		p, err := ParseProfile(readFile(t, path))
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}

		if want := strings.TrimSuffix(filepath.Base(path), ".json"); p.ID != want {
			t.Errorf("%s: id %q does not match the file name", path, p.ID)
		}
	}
}

func TestLinks(t *testing.T) {
	p, err := ParseProfile(edit(t, func(d map[string]interface{}) {
		d["links"] = map[string]interface{}{"donate": "https://slashdiablo.net/donate", "support": "https://slashdiablo.net/help"}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if p.Links.Donate != "https://slashdiablo.net/donate" || p.Links.Support != "https://slashdiablo.net/help" {
		t.Errorf("links = %+v", p.Links)
	}

	// Links open in the browser, so they must be https; unknown ones are
	// refused rather than silently never shown.
	for _, links := range []map[string]interface{}{
		{"donate": "http://slashdiablo.net/donate"},
		{"donate": "javascript:alert(1)"},
		{"patreon": "https://patreon.com/slashdiablo"},
	} {
		if _, err := ParseProfile(edit(t, func(d map[string]interface{}) { d["links"] = links })); err == nil {
			t.Errorf("links %v accepted", links)
		}
	}
}

func TestProfileProblems(t *testing.T) {
	components := func(doc map[string]interface{}) []interface{} {
		return doc["components"].([]interface{})
	}
	settings := func(doc map[string]interface{}) []interface{} {
		return doc["settings"].([]interface{})
	}

	tests := []struct {
		name   string
		change func(map[string]interface{})
		want   string
	}{
		{
			"manifest on another host",
			func(d map[string]interface{}) {
				d["channels"].([]interface{})[0].(map[string]interface{})["manifest"] = "https://evil.example/manifest.json"
			},
			"not on one of the profile's hosts",
		},
		{
			"http news feed",
			func(d map[string]interface{}) { d["news"] = "http://slashdiablo.net/feed.json" },
			"",
		},
		{
			"unknown default version",
			func(d map[string]interface{}) { components(d)[0].(map[string]interface{})["default"] = "9.9.9" },
			"is not one of its versions",
		},
		{
			"conflict with unknown component",
			func(d map[string]interface{}) {
				components(d)[1].(map[string]interface{})["conflicts"] = []interface{}{"nope"}
			},
			"unknown component",
		},
		{
			"flag setting not allowed",
			func(d map[string]interface{}) {
				settings(d)[0].(map[string]interface{})["target"] = map[string]interface{}{"flag": "-nopk"}
			},
			"not in launch.allowedFlags",
		},
		{
			"setting on unknown component",
			func(d map[string]interface{}) { settings(d)[2].(map[string]interface{})["component"] = "nope" },
			"unknown component",
		},
		{
			"setting file with reserved name",
			func(d map[string]interface{}) {
				settings(d)[2].(map[string]interface{})["target"] = map[string]interface{}{"file": "con.cfg", "format": "bh", "key": "k"}
			},
			"reserved name",
		},
		{
			"ini setting without section",
			func(d map[string]interface{}) {
				target := settings(d)[6].(map[string]interface{})["target"].(map[string]interface{})
				delete(target, "section")
			},
			"need a section",
		},
		{
			"bool default of the wrong type",
			func(d map[string]interface{}) { settings(d)[6].(map[string]interface{})["default"] = "yes" },
			"true or false",
		},
		{
			"duplicate setting id",
			func(d map[string]interface{}) {
				settings(d)[1].(map[string]interface{})["id"] = settings(d)[0].(map[string]interface{})["id"]
			},
			"declared twice",
		},
		{
			"version named custom",
			func(d map[string]interface{}) {
				versions := components(d)[0].(map[string]interface{})["versions"].([]interface{})
				versions[0].(map[string]interface{})["id"] = "Custom"
				components(d)[0].(map[string]interface{})["default"] = "Custom"
			},
			"reserved",
		},
		{
			"report pattern taking a whole folder",
			func(d map[string]interface{}) { d["report"] = map[string]interface{}{"files": []interface{}{"logs/*"}} },
			"letter or digit",
		},
		{
			"report pattern with a wildcard folder",
			func(d map[string]interface{}) {
				d["report"] = map[string]interface{}{"files": []interface{}{"*/client.log"}}
			},
			"",
		},
		{
			"report pattern climbing out",
			func(d map[string]interface{}) {
				d["report"] = map[string]interface{}{"files": []interface{}{"../client.log"}}
			},
			"",
		},
		{
			"unknown field",
			func(d map[string]interface{}) { d["script"] = "calc.exe" },
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseProfile(edit(t, tt.change))
			if err == nil {
				t.Fatal("expected an error")
			}

			if tt.want != "" && !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not mention %q", err, tt.want)
			}
		})
	}
}

func TestProfileReportFiles(t *testing.T) {
	p, err := ParseProfile(edit(t, func(d map[string]interface{}) {
		d["report"] = map[string]interface{}{"files": []interface{}{"Mod_Debug*.log", "version.txt", "logs/net-??.log"}}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Report.Files) != 3 || p.Report.Files[2] != "logs/net-??.log" {
		t.Errorf("report.files = %v", p.Report.Files)
	}
}

func TestManifestProblems(t *testing.T) {
	p := exampleProfile(t)

	tests := []struct {
		name  string
		files string
		want  string
	}{
		{
			"case-insensitive duplicate",
			`[{"path":"Game.exe","size":1,"sha256":"` + strings.Repeat("0", 64) + `","urls":["https://slashdiablo.net/a"]},
			  {"path":"game.EXE","size":1,"sha256":"` + strings.Repeat("0", 64) + `","urls":["https://slashdiablo.net/b"]}]`,
			"same file on Windows",
		},
		{
			"download from another host",
			`[{"path":"Game.exe","size":1,"sha256":"` + strings.Repeat("0", 64) + `","urls":["https://evil.example/Game.exe"]}]`,
			"not on one of the profile's hosts",
		},
		{
			"reserved name",
			`[{"path":"data/aux.dll","mode":"delete"}]`,
			"reserved name",
		},
		{
			"trailing dot",
			`[{"path":"Game.exe.","mode":"delete"}]`,
			"ends with a dot",
		},
		{
			"climbs out",
			`[{"path":"../Game.exe","mode":"delete"}]`,
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte(`{"schema":1,"server":"slashdiablo","version":"1","files":` + tt.files + `}`)

			_, err := ParseManifest(data, p)
			if err == nil {
				t.Fatal("expected an error")
			}

			if tt.want != "" && !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not mention %q", err, tt.want)
			}
		})
	}

	other := []byte(`{"schema":1,"server":"resurgence","version":"1","files":[]}`)
	if _, err := ParseManifest(other, p); err == nil || !strings.Contains(err.Error(), "not \"slashdiablo\"") {
		t.Errorf("manifest for another server: %v", err)
	}
}
