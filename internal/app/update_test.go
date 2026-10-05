package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRemoveOldUpdates(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "attempt-1")
	recent := filepath.Join(dir, "attempt-2")
	for _, d := range []string{old, recent} {
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "launcher-amd64-installer.exe"), []byte("x"), 0o644)
	}
	yesterday := time.Now().Add(-25 * time.Hour)
	os.Chtimes(old, yesterday, yesterday)

	removeOldUpdates(dir)

	if _, err := os.Stat(old); err == nil {
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
	root := t.TempDir()
	elsewhere := filepath.Join(root, "elsewhere")
	victim := filepath.Join(elsewhere, "old-folder")
	os.MkdirAll(victim, 0o755)
	yesterday := time.Now().Add(-25 * time.Hour)
	os.Chtimes(victim, yesterday, yesterday)

	link := filepath.Join(root, "updates")
	if err := os.Symlink(elsewhere, link); err != nil {
		if runtime.GOOS != "windows" {
			t.Skipf("can't make a symlink here: %v", err)
		}
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, elsewhere).CombinedOutput(); err != nil {
			t.Skipf("can't make a symlink or junction here: %v %s", err, out)
		}
	}

	if err := plainDir(link); err == nil {
		t.Error("linked updates folder accepted")
	}

	removeOldUpdates(link)
	if _, err := os.Stat(victim); err != nil {
		t.Error("cleanup followed the link and removed a folder elsewhere")
	}

	if err := plainDir(elsewhere); err != nil {
		t.Errorf("plain folder refused: %v", err)
	}
}
