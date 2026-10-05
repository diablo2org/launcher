package logs

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriterRotates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "launcher.log")

	w, err := Open(path, 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	line := strings.Repeat("x", 39) + "\n" // 40 bytes
	for i := 0; i < 10; i++ {
		if _, err := w.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}

	// 10 entries of 40 bytes, at most two per 100-byte file: the current
	// file and two old ones are kept, the rest dropped.
	for _, name := range []string{"launcher.log", "launcher.1.log", "launcher.2.log"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if info.Size() == 0 || info.Size() > 100 {
			t.Errorf("%s is %d bytes", name, info.Size())
		}
		if info.Size()%40 != 0 {
			t.Errorf("%s has a split entry: %d bytes", name, info.Size())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "launcher.3.log")); err == nil {
		t.Error("kept more old files than asked")
	}
}

func TestWriterAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "launcher.log")
	os.WriteFile(path, []byte("before\n"), 0o644)

	w, err := Open(path, 1<<20, 2)
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("after\n"))
	w.Close()

	if _, err := w.Write([]byte("closed\n")); err == nil {
		t.Error("write after close accepted")
	}

	data, _ := os.ReadFile(path)
	if string(data) != "before\nafter\n" {
		t.Errorf("log = %q", data)
	}
}

func TestOld(t *testing.T) {
	for n, want := range map[int]string{0: "a/launcher.log", 1: "a/launcher.1.log", 3: "a/launcher.3.log"} {
		if got := Old("a/launcher.log", n); got != want {
			t.Errorf("Old(%d) = %s, want %s", n, got, want)
		}
	}
}

func TestAtLeast(t *testing.T) {
	var buf strings.Builder
	l := slog.New(AtLeast(slog.NewTextHandler(&buf, nil), slog.LevelWarn)).With("from", "wails")
	l.Info("asset served")
	l.Warn("something odd")

	if out := buf.String(); strings.Contains(out, "asset served") || !strings.Contains(out, "something odd") || !strings.Contains(out, "from=wails") {
		t.Errorf("log = %q", out)
	}
}
