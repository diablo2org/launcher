package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	for tag, want := range map[string][2]string{
		"v1.2.3":           {"1.2.3", "1.2.3"},
		"0.4.0":            {"0.4.0", "0.4.0"},
		"v1.2.3-beta.1":    {"1.2.3", "1.2.3-beta.1"},
		"v10.0.12+ci.7":    {"10.0.12", "10.0.12+ci.7"},
		"v1.0.0-0.3.7":     {"1.0.0", "1.0.0-0.3.7"},
		"v1.0.0-x-y.1+b.2": {"1.0.0", "1.0.0-x-y.1+b.2"},
	} {
		numeric, full, err := parse(tag)
		if err != nil || numeric != want[0] || full != want[1] {
			t.Errorf("parse(%q) = %q, %q, %v; want %q, %q", tag, numeric, full, err, want[0], want[1])
		}
	}

	for _, bad := range []string{
		"", "dev", "v1.2", "v1.2.3.4", "1.2.3 ", "v1.2.x",
		// Leading zeros, in the version or a numeric pre-release identifier.
		"v01.2.3", "v1.2.3-beta.01",
		// Empty suffixes and identifiers.
		"v1.2.3-", "v1.2.3+", "v1.2.3-beta..1",
		// Anything a shell would act on.
		"v1.2.3;whoami;#", "v1.2.3-$(id)", "v1.2.3-a`b`", `v1.2.3-"x"`,
	} {
		if _, _, err := parse(bad); err == nil {
			t.Errorf("parse(%q) accepted", bad)
		}
	}
}

func TestRun(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "info.json")
	out := filepath.Join(dir, "info.stamped.json")

	// The repository's own info.json, so a change to its shape is caught.
	data, err := os.ReadFile("../info.json")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(in, data, 0o644)

	if err := run(in, out, "v1.4.0-rc.2"); err != nil {
		t.Fatal(err)
	}

	var got struct {
		Fixed struct {
			FileVersion string `json:"file_version"`
		} `json:"fixed"`
		Info map[string]map[string]string `json:"info"`
	}
	stamped, _ := os.ReadFile(out)
	if err := json.Unmarshal(stamped, &got); err != nil {
		t.Fatal(err)
	}

	if got.Fixed.FileVersion != "1.4.0" {
		t.Errorf("file_version = %q", got.Fixed.FileVersion)
	}
	lang := got.Info["0000"]
	if lang["ProductVersion"] != "1.4.0-rc.2" || lang["FileVersion"] != "1.4.0-rc.2" {
		t.Errorf("versions = %v", lang)
	}
	if lang["CompanyName"] != "diablo2org" {
		t.Errorf("other fields lost: %v", lang)
	}

	// The input is left alone.
	if after, _ := os.ReadFile(in); string(after) != string(data) {
		t.Error("info.json changed")
	}
}

// Windows shows none of an exe's version strings, not even its company
// name, unless they include FileVersion. Wails' template for info.json
// leaves it out, so regenerating build assets drops it.
func TestInfoHasFileVersion(t *testing.T) {
	var info struct {
		Info map[string]map[string]string `json:"info"`
	}
	data, err := os.ReadFile("../info.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatal(err)
	}

	for lang, strings := range info.Info {
		if strings["FileVersion"] == "" {
			t.Errorf("build/windows/info.json, %s: no FileVersion, so Windows shows no version details", lang)
		}
	}
}
