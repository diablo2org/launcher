package install

import (
	"os"
	"path/filepath"
	"testing"
)

func base(t *testing.T, names ...string) string {
	t.Helper()

	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("archive "+n), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

func TestCheck(t *testing.T) {
	dir := base(t, "d2data.mpq", "d2sfx.mpq", "d2speech.mpq", "d2music.mpq")

	r, err := Check(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if r.OK() || len(r.Missing) != 1 || r.Missing[0] != "d2exp.mpq" {
		t.Errorf("expansion check: missing %v, want only d2exp.mpq", r.Missing)
	}
	if len(r.MissingOptional) != 5 {
		t.Errorf("optional missing = %v", r.MissingOptional)
	}

	// Classic doesn't need the expansion archives.
	if r, _ := Check(dir, false); !r.OK() {
		t.Errorf("classic check failed: %v", r.Missing)
	}

	if _, err := Check(filepath.Join(dir, "nope"), true); err == nil {
		t.Error("missing folder not reported")
	}
}

func TestLinkArchives(t *testing.T) {
	b := base(t, "d2data.mpq", "d2sfx.mpq", "d2speech.mpq", "d2exp.mpq")
	server := ServerDir(b, "slashdiablo")

	if err := LinkArchives(b, server, true); err != nil {
		t.Skipf("hard links not available: %v", err)
	}

	for _, n := range []string{"d2data.mpq", "d2exp.mpq"} {
		src, _ := os.Stat(filepath.Join(b, n))
		dst, err := os.Stat(filepath.Join(server, n))
		if err != nil || !os.SameFile(src, dst) {
			t.Errorf("%s is not linked: %v", n, err)
		}
	}

	if _, err := os.Stat(filepath.Join(server, "d2music.mpq")); !os.IsNotExist(err) {
		t.Error("linked an archive the base doesn't have")
	}

	// Running again is a no-op, and a stale file gets replaced by a link.
	os.Remove(filepath.Join(server, "d2sfx.mpq"))
	os.WriteFile(filepath.Join(server, "d2sfx.mpq"), []byte("stale"), 0o644)

	if err := LinkArchives(b, server, true); err != nil {
		t.Fatal(err)
	}

	src, _ := os.Stat(filepath.Join(b, "d2sfx.mpq"))
	dst, _ := os.Stat(filepath.Join(server, "d2sfx.mpq"))
	if !os.SameFile(src, dst) {
		t.Error("stale copy was not replaced with a link")
	}
}

func TestCopyArchives(t *testing.T) {
	b := base(t, "d2data.mpq", "d2sfx.mpq")
	server := filepath.Join(t.TempDir(), "srv")

	if err := CopyArchives(b, server, false); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(server, "d2data.mpq"))
	if err != nil || string(data) != "archive d2data.mpq" {
		t.Errorf("copy = %q, %v", data, err)
	}

	r, _ := Check(b, false)
	if CopySize(r) != int64(len("archive d2data.mpq")+len("archive d2sfx.mpq")) {
		t.Errorf("CopySize = %d", CopySize(r))
	}
}

func TestWarning(t *testing.T) {
	t.Setenv("ProgramFiles", `C:\Program Files`)
	t.Setenv("ProgramFiles(x86)", `C:\Program Files (x86)`)

	if Warning(`C:\Program Files (x86)\Diablo II`) == "" {
		t.Error("no warning for Program Files (x86)")
	}
	if Warning(`C:\Games\Diablo II`) != "" {
		t.Error("warning for a normal folder")
	}
	if Warning(`C:\Program Files Extra\Diablo II`) != "" {
		t.Error("prefix match on a similarly named folder")
	}
}
