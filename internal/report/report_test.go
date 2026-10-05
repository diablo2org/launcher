package report

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diablo2org/launcher/internal/install"
)

func write(t *testing.T, path, body string) {
	t.Helper()

	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReport(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	base := filepath.Join(root, "Diablo II")
	home := `C:\Users\Player`

	write(t, filepath.Join(data, "logs", "launcher.log"), `level=ERROR msg="update failed" dir=C:\Users\Player\Games\Diablo II`+"\n")
	write(t, filepath.Join(data, "logs", "launcher.1.log"), "older\n")
	write(t, filepath.Join(data, "logs", "crash.log"), "panic: boom\n")
	write(t, filepath.Join(data, "state.json"), `{"basePath": "c:\users\player\Games\Diablo II"}`)

	slash := install.ServerDir(base, "slashdiablo")
	for i := 1; i <= 7; i++ {
		f := filepath.Join(slash, "D2260"+string(rune('0'+i))+"01.txt")
		write(t, f, "crash")
		mod := time.Now().Add(time.Duration(i) * time.Hour)
		os.Chtimes(f, mod, mod)
	}
	write(t, filepath.Join(slash, "D2Client.dll"), "not a log")
	write(t, filepath.Join(slash, "Patch.txt"), "not a log")

	items := Collect(Input{DataDir: data, Base: base, Servers: []string{"slashdiablo", "gone"}, About: "Launcher v1", Home: home})

	names := map[string]bool{}
	for _, it := range items {
		names[it.Name] = true
		if it.What == "" || it.Size == 0 {
			t.Errorf("%s: what %q, size %d", it.Name, it.What, it.Size)
		}
	}
	for _, want := range []string{"about.txt", "logs/launcher.log", "logs/launcher.1.log", "logs/crash.log", "state.json", "game/slashdiablo/D2260701.txt"} {
		if !names[want] {
			t.Errorf("%s missing from %v", want, names)
		}
	}
	// Only the five newest game crash logs, and nothing else from the folder.
	if names["game/slashdiablo/D2260201.txt"] || names["game/slashdiablo/D2260101.txt"] {
		t.Error("older game crash logs included")
	}
	if names["game/slashdiablo/Patch.txt"] || names["game/slashdiablo/D2Client.dll"] {
		t.Error("game files that aren't crash logs included")
	}

	var buf bytes.Buffer
	if err := Write(&buf, items, home); err != nil {
		t.Fatal(err)
	}

	z, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(z.File) != len(items) {
		t.Errorf("zip has %d files, want %d", len(z.File), len(items))
	}
	for _, f := range z.File {
		r, _ := f.Open()
		body, _ := io.ReadAll(r)
		r.Close()
		if strings.Contains(strings.ToLower(string(body)), "player") {
			t.Errorf("%s still names the user: %s", f.Name, body)
		}
		if f.Name == "logs/launcher.log" && !strings.Contains(string(body), `%USERPROFILE%\Games`) {
			t.Errorf("log = %s", body)
		}
	}
}

func TestReadCappedKeepsTheEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.log")
	write(t, path, strings.Repeat("a", maxFile)+"newest")

	data, err := readCapped(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != maxFile || !strings.HasSuffix(string(data), "newest") {
		t.Errorf("read %d bytes ending %q", len(data), data[len(data)-6:])
	}
}

func TestIsGameCrashLog(t *testing.T) {
	for name, want := range map[string]bool{
		"D2260929.txt": true, "d2260929.TXT": true,
		"D2.LNG": false, "D2Client.txt": false, "D226092.txt": false, "Patch.txt": false,
	} {
		if got := isGameCrashLog(name); got != want {
			t.Errorf("%s: %v, want %v", name, got, want)
		}
	}
}

func TestRedactUnicode(t *testing.T) {
	// İ lower-cases to a shorter sequence, which once made the whole file
	// skip redaction.
	data := []byte(`İstanbul save at C:\USERS\player\Saved Games and c:/users/Player/x`)
	got := string(redact(data, `C:\Users\Player`))
	if strings.Contains(strings.ToLower(got), "player") || !strings.HasPrefix(got, "İstanbul") {
		t.Errorf("redact = %q", got)
	}

	// Non-ASCII user names match regardless of case too.
	got = string(redact([]byte(`C:\Users\ÉLODIE\x`), `C:\Users\élodie`))
	if got != `%USERPROFILE%\x` {
		t.Errorf("redact = %q", got)
	}
}

func TestCollectSkipsUnsafeServerIDs(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "Diablo II")
	write(t, filepath.Join(root, "outside", "D2260101.txt"), "not ours")
	write(t, filepath.Join(install.ServerDir(base, "ok"), "D2260101.txt"), "ours")

	items := Collect(Input{DataDir: filepath.Join(root, "data"), Base: base, Servers: []string{"../outside", `..\outside`, "ok"}})
	for _, it := range items {
		if strings.Contains(it.Name, "..") || strings.Contains(it.Name, "outside") {
			t.Errorf("collected %s", it.Name)
		}
	}
	if items[len(items)-1].Name != "game/ok/D2260101.txt" {
		t.Errorf("items = %+v", items)
	}
}
