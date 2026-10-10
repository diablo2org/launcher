package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// old makes a folder with a file in it, a day old.
func old(t *testing.T, dir string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "launcher-amd64-installer.exe"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	yesterday := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(dir, yesterday, yesterday); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveOldUpdates(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "updates")
	root, err := openUpdatesDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	stale := filepath.Join(dir, "attempt-1")
	old(t, stale)
	recent, err := newAttempt(root, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Base(recent), "attempt-") || filepath.Dir(recent) != dir {
		t.Errorf("new attempt at %s", recent)
	}

	removeOldUpdates(root)

	if _, err := os.Stat(stale); err == nil {
		t.Error("day-old attempt kept")
	}
	if _, err := os.Stat(recent); err != nil {
		t.Error("recent attempt removed; its installer may be running")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Error("the updates folder itself was removed")
	}
}

func TestUpdatesDirMustNotBeALink(t *testing.T) {
	tmp := t.TempDir()
	elsewhere := filepath.Join(tmp, "elsewhere")
	victim := filepath.Join(elsewhere, "old-folder")
	old(t, victim)

	link := filepath.Join(tmp, "updates")
	if err := os.Symlink(elsewhere, link); err != nil {
		if runtime.GOOS != "windows" {
			t.Skipf("can't make a symlink here: %v", err)
		}
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, elsewhere).CombinedOutput(); err != nil {
			t.Skipf("can't make a symlink or junction here: %v %s", err, out)
		}
	}

	if root, err := openUpdatesDir(link); err == nil {
		root.Close()
		t.Fatal("linked updates folder accepted")
	}
	if _, err := os.Stat(victim); err != nil {
		t.Error("a folder the link points to was removed")
	}
}
