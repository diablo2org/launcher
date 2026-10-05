package app

import (
	"os"
	"path/filepath"
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
