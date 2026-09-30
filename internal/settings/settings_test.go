package settings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d2org/launcher/internal/spec"
)

const d2glINI = "; D2GL config\r\n" +
	"[Screen]\r\n" +
	"; Unlock the cursor\r\n" +
	"unlock_cursor=false\r\n" +
	"window_width=1280 ; comment kept\r\n" +
	"\r\n" +
	"[Feature]\r\n" +
	"unlock_cursor=true\r\n"

const bhCfg = "// BH settings\r\n" +
	"Reveal Map:             True, None\r\n" +
	"Show Monsters:          False, VK_M // toggles\r\n" +
	"Experience Meter:       False\r\n" +
	"Loot Filter:            default\r\n"

func setting(id, typ, format, section, key string) spec.Setting {
	return spec.Setting{
		ID:     id,
		Type:   typ,
		Target: spec.SettingTarget{File: "cfg/file.txt", Format: format, Section: section, Key: key},
	}
}

func fixture(t *testing.T, contents string) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cfg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cfg", "file.txt"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func contents(t *testing.T, dir string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(dir, "cfg", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}

func TestINIRespectsSections(t *testing.T) {
	dir := fixture(t, d2glINI)
	s := setting("unlock", "bool", "ini", "Screen", "unlock_cursor")

	if v, err := Read(dir, s); err != nil || v != false {
		t.Fatalf("Read = %v, %v; want false from [Screen], not [Feature]", v, err)
	}

	if err := Write(dir, s, true); err != nil {
		t.Fatal(err)
	}

	want := strings.Replace(d2glINI, "unlock_cursor=false", "unlock_cursor=true", 1)
	if got := contents(t, dir); got != want {
		t.Errorf("file changed beyond the one value:\n%q\nwant\n%q", got, want)
	}
}

func TestINIKeepsTrailingComment(t *testing.T) {
	dir := fixture(t, d2glINI)
	s := setting("width", "int", "ini", "screen", "WINDOW_WIDTH")

	if v, _ := Read(dir, s); v != 1280 {
		t.Fatalf("Read = %v, want 1280 (case-insensitive, comment stripped)", v)
	}

	if err := Write(dir, s, 1920); err != nil {
		t.Fatal(err)
	}

	if got := contents(t, dir); !strings.Contains(got, "window_width=1920 ; comment kept\r\n") {
		t.Errorf("comment or line ending lost:\n%q", got)
	}
}

func TestINIAddsMissingKeyToSection(t *testing.T) {
	dir := fixture(t, d2glINI)

	if err := Write(dir, setting("vsync", "bool", "ini", "Screen", "vsync"), true); err != nil {
		t.Fatal(err)
	}

	got := contents(t, dir)
	if !strings.Contains(got, "window_width=1280 ; comment kept\r\nvsync=true\r\n\r\n[Feature]") {
		t.Errorf("missing key not added at the end of [Screen]:\n%q", got)
	}
}

func TestINIAddsMissingSection(t *testing.T) {
	dir := fixture(t, "[Screen]\nfullscreen=true\n")

	if err := Write(dir, setting("x", "string", "ini", "Other", "name"), "abc"); err != nil {
		t.Fatal(err)
	}

	if got := contents(t, dir); got != "[Screen]\nfullscreen=true\n\n[Other]\nname=abc\n" {
		t.Errorf("got %q", got)
	}
}

func TestBHKeepsHotkeyAndAlignment(t *testing.T) {
	dir := fixture(t, bhCfg)

	reveal := setting("reveal", "bool", "bh", "", "Reveal Map")
	monsters := setting("monsters", "bool", "bh", "", "Show Monsters")

	if v, _ := Read(dir, reveal); v != true {
		t.Errorf("Reveal Map = %v, want true", v)
	}
	if v, _ := Read(dir, monsters); v != false {
		t.Errorf("Show Monsters = %v, want false", v)
	}

	if err := Write(dir, reveal, false); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, monsters, true); err != nil {
		t.Fatal(err)
	}

	want := strings.NewReplacer(
		"Reveal Map:             True, None", "Reveal Map:             False, None",
		"Show Monsters:          False, VK_M // toggles", "Show Monsters:          True, VK_M // toggles",
	).Replace(bhCfg)

	if got := contents(t, dir); got != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}

func TestBHStringValue(t *testing.T) {
	dir := fixture(t, bhCfg)
	s := setting("filter", "string", "bh", "", "Loot Filter")

	if err := Write(dir, s, "strict"); err != nil {
		t.Fatal(err)
	}

	if got := contents(t, dir); !strings.Contains(got, "Loot Filter:            strict\r\n") {
		t.Errorf("got %q", got)
	}
}

func TestBHAppendsMissingKey(t *testing.T) {
	dir := fixture(t, bhCfg)

	if err := Write(dir, setting("ilvl", "bool", "bh", "", "Show iLvl"), true); err != nil {
		t.Fatal(err)
	}

	if got := contents(t, dir); !strings.HasSuffix(got, "Loot Filter:            default\r\n\r\n// Added by the launcher\r\nShow iLvl: True\r\n") {
		t.Errorf("got %q", got)
	}

	// A second missing key joins the same block rather than adding a marker.
	if err := Write(dir, setting("eth", "bool", "bh", "", "Show Ethereal"), false); err != nil {
		t.Fatal(err)
	}
	if got := contents(t, dir); !strings.HasSuffix(got, "// Added by the launcher\r\nShow iLvl: True\r\nShow Ethereal: False\r\n") ||
		strings.Count(got, "Added by the launcher") != 1 {
		t.Errorf("got %q", got)
	}
}

func TestNeverCreatesFile(t *testing.T) {
	dir := t.TempDir()
	s := setting("reveal", "bool", "bh", "", "Reveal Map")

	if _, err := Read(dir, s); !errors.Is(err, ErrFileMissing) {
		t.Errorf("Read on missing file: %v", err)
	}

	if err := Write(dir, s, true); !errors.Is(err, ErrFileMissing) {
		t.Errorf("Write on missing file: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "cfg", "file.txt")); !os.IsNotExist(err) {
		t.Error("file was created")
	}
}

func TestMissingKeyReadsDefault(t *testing.T) {
	dir := fixture(t, bhCfg)
	s := setting("ilvl", "bool", "bh", "", "Show iLvl")
	s.Default = true

	if v, _ := Read(dir, s); v != true {
		t.Errorf("Read = %v, want the default true", v)
	}
}

func TestRejectsBadValues(t *testing.T) {
	dir := fixture(t, bhCfg)
	min, max := 1, 10

	intSetting := setting("n", "int", "ini", "Screen", "n")
	intSetting.Min, intSetting.Max = &min, &max

	choice := setting("c", "choice", "ini", "Screen", "c")
	choice.Options = []spec.Option{{Value: "a", Label: "A"}}

	tests := []struct {
		name  string
		s     spec.Setting
		value interface{}
	}{
		{"bool given a string", setting("b", "bool", "bh", "", "Reveal Map"), "yes"},
		{"int out of range", intSetting, 11},
		{"int with a fraction", intSetting, 2.5},
		{"unknown choice", choice, "b"},
		{"line break injection", setting("s", "string", "bh", "", "Loot Filter"), "x\r\nReveal Map: True"},
		{"comment injection", setting("s", "string", "ini", "Screen", "k"), "x ; y"},
		{"flag setting", spec.Setting{ID: "w", Type: "bool", Target: spec.SettingTarget{Flag: "-w"}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Write(dir, tt.s, tt.value); err == nil {
				t.Error("expected an error")
			}
		})
	}

	if got := contents(t, dir); got != bhCfg {
		t.Errorf("a rejected write changed the file:\n%q", got)
	}
}

func TestRejectsEscapingPath(t *testing.T) {
	s := setting("x", "bool", "bh", "", "k")
	s.Target.File = "../outside.cfg"

	if _, err := Read(t.TempDir(), s); err == nil || errors.Is(err, ErrFileMissing) {
		t.Errorf("Read outside the folder: %v", err)
	}
}
