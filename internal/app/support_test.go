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
	"github.com/diablo2org/launcher/internal/report"
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

	path, err := s.SaveReport("")
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
	if path, err := s.SaveReport(""); path != "" || err != nil {
		t.Errorf("cancelled: %q, %v", path, err)
	}
}

// A launcher report holds no server's files; a server's holds only its own.
func TestReportScope(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m, err := core.New(st, core.DirListing(t.TempDir()), launch.New(nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	s := NewSupportService(st, m, Host{})

	if in := s.input(""); len(in.Servers) != 0 || in.State != nil {
		t.Errorf("launcher report: servers %v, state cut %v", in.Servers, in.State != nil)
	}
	if in := s.input("resurgence"); len(in.Servers) != 1 || in.Servers[0] != "resurgence" {
		t.Errorf("server report: servers %v", in.Servers)
	}

	// A state.json that can't be read can't be cut down to one server, so a
	// server's report leaves it out; the launcher's still takes it.
	os.WriteFile(filepath.Join(st.Dir(), "state.json"), []byte("{"), 0o644)
	has := func(items []report.Item) bool {
		for _, it := range items {
			if it.Name == "state.json" {
				return true
			}
		}
		return false
	}
	if has(report.Collect(s.input("resurgence"))) {
		t.Error("server report holds an unreadable state.json")
	}
	if !has(report.Collect(s.input(""))) {
		t.Error("launcher report lost state.json")
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.zip")
	os.WriteFile(path, []byte("old"), 0o644)

	if err := writeFileAtomic(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "new" {
		t.Errorf("file = %q", data)
	}

	// Into a folder that doesn't exist: an error, and nothing left behind.
	if err := writeFileAtomic(filepath.Join(dir, "missing", "r.zip"), []byte("x")); err == nil {
		t.Error("write into a missing folder succeeded")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("left behind: %v", entries)
	}
}
