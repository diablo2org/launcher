package app

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diablo2org/launcher/internal/core"
	"github.com/diablo2org/launcher/internal/launch"
	"github.com/diablo2org/launcher/internal/logs"
	"github.com/diablo2org/launcher/internal/store"
)

func TestSaveReport(t *testing.T) {
	data := t.TempDir()
	os.MkdirAll(logs.Dir(data), 0o755)
	os.WriteFile(filepath.Join(logs.Dir(data), logs.Current), []byte("level=INFO msg=hello\n"), 0o644)

	st, err := store.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	m, err := core.New(st, core.DirListing(t.TempDir()), launch.New(nil, nil))
	if err != nil {
		t.Fatal(err)
	}

	// The player types a name without .zip.
	target := filepath.Join(t.TempDir(), "my report")
	var shown string
	s := NewSupportService(st, m, Host{
		SaveFile: func(title, name string) (string, error) {
			if !strings.HasSuffix(name, ".zip") {
				t.Errorf("suggested name %q", name)
			}
			return target, nil
		},
		ShowFile: func(path string) error { shown = path; return nil },
	})

	path, err := s.SaveReport()
	if err != nil {
		t.Fatal(err)
	}
	if path != target+".zip" || shown != path {
		t.Errorf("saved %q, shown %q", path, shown)
	}

	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()

	var names []string
	for _, f := range z.File {
		names = append(names, f.Name)
	}
	if got := strings.Join(names, ","); got != "about.txt,logs/launcher.log" {
		t.Errorf("report holds %s", got)
	}

	// Cancelling saves nothing.
	s.host.SaveFile = func(string, string) (string, error) { return "", nil }
	if path, err := s.SaveReport(); path != "" || err != nil {
		t.Errorf("cancelled: %q, %v", path, err)
	}
}
