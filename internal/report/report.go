// Package report builds a bug report: a zip of the launcher's logs, crash
// traces and state, with the game's own crash logs, that a player can attach
// when asking for help. It is only ever saved where the player chooses;
// nothing is sent anywhere.
package report

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/diablo2org/launcher/internal/install"
	"github.com/diablo2org/launcher/internal/logs"
)

// maxFile is the most taken from any one file. Logs are capped well below
// this; it keeps a stray large game file from bloating the report.
const maxFile = 2 << 20

// gameCrashes is how many of the game's crash logs are taken per server, the
// newest first.
const gameCrashes = 5

// Input is where a report's contents come from.
type Input struct {
	// DataDir is the launcher's data folder: logs and state.json.
	DataDir string
	// Base is the player's Diablo II folder, for the server folders in it.
	Base string
	// Servers are the ids of servers that may have a folder.
	Servers []string
	// About is a few lines on the launcher and system, written first.
	About string
	// Home is replaced by %USERPROFILE% in every text file, so the report
	// doesn't carry the player's Windows user name.
	Home string
}

// Item is one file in a report.
type Item struct {
	// Name is the path in the zip.
	Name string `json:"name"`
	// What says what it is, for the player.
	What string `json:"what"`
	Size int64  `json:"size"`

	source string
	data   []byte
}

// Collect lists what a report would hold, without reading the files, so the
// player can see it before agreeing.
func Collect(in Input) []Item {
	items := []Item{{Name: "about.txt", What: "Launcher version and system", data: []byte(in.About), Size: int64(len(in.About))}}

	add := func(name, what, source string) {
		info, err := os.Stat(source)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return
		}
		size := info.Size()
		if size > maxFile {
			size = maxFile
		}
		items = append(items, Item{Name: name, What: what, Size: size, source: source})
	}

	logDir := logs.Dir(in.DataDir)
	for i := 0; i < 10; i++ {
		name := filepath.Base(logs.Old(logs.Current, i))
		add("logs/"+name, "Launcher log", filepath.Join(logDir, name))
	}
	add("logs/"+logs.Crash, "Launcher crashes", filepath.Join(logDir, logs.Crash))
	add("state.json", "Your launcher settings and choices", filepath.Join(in.DataDir, "state.json"))

	if in.Base != "" {
		ids := append([]string{}, in.Servers...)
		sort.Strings(ids)
		for _, id := range ids {
			for _, f := range gameCrashLogs(install.ServerDir(in.Base, id)) {
				add("game/"+id+"/"+filepath.Base(f), "Diablo II crash log", f)
			}
		}
	}

	return items
}

// gameCrashLogs finds the newest of the logs Diablo II writes when it
// crashes, D2YYMMDD.txt, in a server folder.
func gameCrashLogs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	type found struct {
		path string
		mod  time.Time
	}
	var all []found
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !isGameCrashLog(name) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		all = append(all, found{filepath.Join(dir, name), info.ModTime()})
	}

	sort.Slice(all, func(i, j int) bool { return all[i].mod.After(all[j].mod) })
	if len(all) > gameCrashes {
		all = all[:gameCrashes]
	}

	paths := make([]string, len(all))
	for i, f := range all {
		paths[i] = f.path
	}

	return paths
}

// isGameCrashLog matches D2 followed by six digits and .txt.
func isGameCrashLog(name string) bool {
	lower := strings.ToLower(name)
	if len(lower) != len("d2yymmdd.txt") || !strings.HasPrefix(lower, "d2") || !strings.HasSuffix(lower, ".txt") {
		return false
	}
	for _, c := range lower[2:8] {
		if c < '0' || c > '9' {
			return false
		}
	}

	return true
}

// Write writes the items as a zip. Text has the player's home folder
// replaced; a file that can't be read is noted in its place rather than
// failing the report.
func Write(w io.Writer, items []Item, home string) error {
	z := zip.NewWriter(w)

	for _, it := range items {
		data := it.data
		if it.source != "" {
			var err error
			data, err = readCapped(it.source)
			if err != nil {
				data = []byte(fmt.Sprintf("couldn't read this file: %v\n", err))
			}
		}

		if utf8.Valid(data) {
			data = redact(data, home)
		}

		f, err := z.CreateHeader(&zip.FileHeader{Name: it.Name, Method: zip.Deflate, Modified: time.Now()})
		if err != nil {
			return err
		}
		if _, err := f.Write(data); err != nil {
			return err
		}
	}

	return z.Close()
}

// readCapped reads the end of a file, up to maxFile: the newest part of a log
// is what matters.
func readCapped(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxFile {
		if _, err := f.Seek(info.Size()-maxFile, io.SeekStart); err != nil {
			return nil, err
		}
	}

	return io.ReadAll(io.LimitReader(f, maxFile))
}

// redact replaces the home folder, written either way round and as JSON
// escapes it, with %USERPROFILE%.
func redact(data []byte, home string) []byte {
	home = strings.TrimRight(home, `\/`)
	if len(home) < 4 {
		return data
	}

	forms := []string{
		home,
		strings.ReplaceAll(home, `\`, `/`),
		strings.ReplaceAll(home, `\`, `\\`),
	}
	for _, f := range forms {
		data = replaceFold(data, f, "%USERPROFILE%")
	}

	return data
}

// replaceFold replaces old case-insensitively, as Windows paths are.
func replaceFold(data []byte, old, new string) []byte {
	lower := bytes.ToLower(data)
	target := bytes.ToLower([]byte(old))
	if !bytes.Contains(lower, target) || len(lower) != len(data) {
		return data
	}

	var out bytes.Buffer
	for {
		i := bytes.Index(lower, target)
		if i < 0 {
			out.Write(data)
			return out.Bytes()
		}
		out.Write(data[:i])
		out.WriteString(new)
		data, lower = data[i+len(target):], lower[i+len(target):]
	}
}
