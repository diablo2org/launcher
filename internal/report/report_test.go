package report

import (
	"archive/zip"
	"bytes"
	"fmt"
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

	data, err := readCapped(filepath.Dir(path), filepath.Base(path), maxFile)
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

// A crash log that is a link, which could lead to any file, is left out, and
// reading can't follow one out of its folder.
func TestCrashLogLinksIgnored(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "Diablo II")
	outside := filepath.Join(root, "secret.txt")
	write(t, outside, "not for the report")

	dir := install.ServerDir(base, "ok")
	os.MkdirAll(dir, 0o755)
	link := filepath.Join(dir, "D2260101.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("can't make a symlink here: %v", err)
	}

	items := Collect(Input{DataDir: filepath.Join(root, "data"), Base: base, Servers: []string{"ok"}})
	for _, it := range items {
		if strings.HasPrefix(it.Name, "game/") {
			t.Errorf("linked crash log collected: %s", it.Name)
		}
	}

	if _, err := readCapped(dir, "D2260101.txt", maxFile); err == nil {
		t.Error("read followed a link out of its folder")
	}
}

func TestServerFiles(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "Diablo II")
	dir := install.ServerDir(base, "ok")

	for i := 1; i <= 12; i++ {
		f := filepath.Join(dir, fmt.Sprintf("Mod_Debug.w%d.log", i))
		write(t, f, "log")
		mod := time.Now().Add(time.Duration(i) * time.Minute)
		os.Chtimes(f, mod, mod)
	}
	write(t, filepath.Join(dir, "version.txt"), "build 7")
	write(t, filepath.Join(dir, "logs", "net.log"), "net")
	write(t, filepath.Join(dir, "D2260101.txt"), "crash")
	write(t, filepath.Join(dir, "Mod.dll"), "code")

	items := Collect(Input{
		DataDir: filepath.Join(root, "data"),
		Base:    base,
		Servers: []string{"ok", "other"},
		Files: map[string][]string{
			"ok":    {"mod_debug*.log", "VERSION.TXT", "logs/*.log", "D2*.txt", "missing.txt"},
			"other": {"*.log"},
		},
	})

	got := map[string]string{}
	for _, it := range items {
		got[it.Name] = it.What
	}

	// Ten newest per pattern, matched without regard to case.
	for i := 3; i <= 12; i++ {
		if got[fmt.Sprintf("game/ok/Mod_Debug.w%d.log", i)] != "Asked for by the server" {
			t.Errorf("w%d log missing: %v", i, got)
		}
	}
	if _, ok := got["game/ok/Mod_Debug.w1.log"]; ok {
		t.Error("more than ten files taken for one pattern")
	}
	if _, ok := got["game/ok/version.txt"]; !ok {
		t.Error("version.txt missing")
	}
	if _, ok := got["game/ok/logs/net.log"]; !ok {
		t.Error("file in a subfolder missing")
	}
	// A crash log the report takes anyway is listed once, as a crash log.
	if got["game/ok/D2260101.txt"] != "Diablo II crash log" {
		t.Errorf("crash log = %q", got["game/ok/D2260101.txt"])
	}
	if _, ok := got["game/ok/Mod.dll"]; ok {
		t.Error("unmatched file taken")
	}

	var buf bytes.Buffer
	if err := Write(&buf, items, ""); err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range z.File {
		if f.Name != "game/ok/logs/net.log" {
			continue
		}
		r, _ := f.Open()
		body, _ := io.ReadAll(r)
		r.Close()
		if string(body) != "net" {
			t.Errorf("net.log = %q", body)
		}
	}
}

func TestServerFilesBudget(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "Diablo II")
	dir := install.ServerDir(base, "ok")

	// Each file is cut to maxFile, so nine of them overrun the budget.
	big := strings.Repeat("a", maxFile+10)
	for i := 1; i <= 9; i++ {
		write(t, filepath.Join(dir, fmt.Sprintf("big%d.log", i)), big)
	}

	items := Collect(Input{DataDir: filepath.Join(root, "data"), Base: base, Servers: []string{"ok"}, Files: map[string][]string{"ok": {"big*.log"}}})

	var total int64
	n := 0
	for _, it := range items {
		if strings.HasPrefix(it.Name, "game/") {
			total += it.Size
			n++
		}
	}
	if total > serverBudget || n != serverBudget/maxFile {
		t.Errorf("took %d files, %d bytes", n, total)
	}
}

// A pattern can't reach outside the server folder through a linked folder.
func TestServerFilesStayInFolder(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "Diablo II")
	write(t, filepath.Join(root, "outside", "secret.log"), "not for the report")

	dir := install.ServerDir(base, "ok")
	os.MkdirAll(dir, 0o755)
	if err := os.Symlink(filepath.Join(root, "outside"), filepath.Join(dir, "logs")); err != nil {
		t.Skipf("can't make a symlink here: %v", err)
	}

	items := Collect(Input{DataDir: filepath.Join(root, "data"), Base: base, Servers: []string{"ok"}, Files: map[string][]string{"ok": {"logs/*.log", "../outside/*.log"}}})
	for _, it := range items {
		if strings.HasPrefix(it.Name, "game/") {
			t.Errorf("collected %s", it.Name)
		}
	}
}

func TestFit(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	base := filepath.Join(root, "Diablo II")
	dir := install.ServerDir(base, "ok")

	// Random-looking text, so the zip can't shrink it much.
	noise := func(n int) string {
		var b strings.Builder
		x := uint32(1)
		for b.Len() < n {
			x = x*1664525 + 1013904223
			fmt.Fprintf(&b, "%08x", x)
		}
		return b.String()[:n]
	}
	write(t, filepath.Join(data, "logs", "launcher.log"), noise(1<<20)+"newest launcher line")
	write(t, filepath.Join(data, "logs", "launcher.1.log"), noise(1<<20))
	write(t, filepath.Join(data, "state.json"), "{}")
	write(t, filepath.Join(dir, "Mod_Debug.log"), noise(3<<20)+"newest mod line")
	write(t, filepath.Join(dir, "Mod_Debug.w2.log"), noise(3<<20))

	items := Collect(Input{DataDir: data, Base: base, Servers: []string{"ok"}, About: "about", Files: map[string][]string{"ok": {"Mod_Debug*.log"}}})

	// Everything fits once each file is cut down.
	zipped, kept, err := Fit(items, "", 2<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(zipped) > 2<<20 || len(kept) != len(items) {
		t.Errorf("%d bytes, kept %d of %d", len(zipped), len(kept), len(items))
	}

	// Tighter, files are left out: the older launcher log first, and the
	// launcher log, about.txt and state.json never.
	zipped, kept, err = Fit(items, "", 100<<10)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, it := range kept {
		names[it.Name] = true
	}
	if len(zipped) > 100<<10 || names["logs/launcher.1.log"] || !names["logs/launcher.log"] || !names["about.txt"] || !names["state.json"] {
		t.Errorf("%d bytes, kept %v", len(zipped), names)
	}

	z, err := zip.NewReader(bytes.NewReader(zipped), int64(len(zipped)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range z.File {
		if f.Name != "logs/launcher.log" {
			continue
		}
		r, _ := f.Open()
		body, _ := io.ReadAll(r)
		r.Close()
		if !strings.HasSuffix(string(body), "newest launcher line") {
			t.Error("a cut file lost its end")
		}
	}

	if _, _, err := Fit(items, "", 1<<10); err == nil {
		t.Error("fitted a report into 1 KB")
	}
}
