package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRemoveOldUpdates(t *testing.T) {
	tmp := t.TempDir()
	old := filepath.Join(tmp, updateDirPrefix+"1")
	recent := filepath.Join(tmp, updateDirPrefix+"2")
	other := filepath.Join(tmp, "something-else")
	for _, d := range []string{old, recent, other} {
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "launcher-amd64-installer.exe"), []byte("x"), 0o644)
	}
	yesterday := time.Now().Add(-25 * time.Hour)
	os.Chtimes(old, yesterday, yesterday)
	os.Chtimes(other, yesterday, yesterday)

	removeOldUpdates(tmp)

	if _, err := os.Stat(old); err == nil {
		t.Error("day-old update folder kept")
	}
	if _, err := os.Stat(recent); err != nil {
		t.Error("recent update folder removed; its installer may be running")
	}
	if _, err := os.Stat(other); err != nil {
		t.Error("unrelated folder removed")
	}
}
