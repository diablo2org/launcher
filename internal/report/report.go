// Package report builds a bug report: a zip of the launcher's logs, crash
// traces and state, with the game's own crash logs and any files a server
// asks for, that a player can attach when asking for help. It is only ever saved where the player chooses;
// nothing is sent anywhere.
package report

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/diablo2org/launcher/internal/install"
	"github.com/diablo2org/launcher/internal/logs"
	"github.com/diablo2org/launcher/internal/paths"
)

// maxFile is the most taken from any one file. Logs are capped well below
// this; it keeps a stray large game file from bloating the report.
const maxFile = 2 << 20

// serverID is the profile spec's id pattern.
var serverID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,31}$`)

// gameCrashes is how many of the game's crash logs are taken per server, the
// newest first.
const gameCrashes = 5

// perPattern is how many files one of a server's report patterns takes, the
// newest first.
const perPattern = 10

// serverBudget caps what a server's own patterns add to a report, counted
// after the per-file cap.
const serverBudget = 16 << 20

// Input is where a report's contents come from.
type Input struct {
	// DataDir is the launcher's data folder: logs and state.json.
	DataDir string
	// Base is the player's Diablo II folder, for the server folders in it.
	Base string
	// Servers are the ids of servers that may have a folder.
	Servers []string
	// Files are the report patterns each server's profile declares, by id.
	Files map[string][]string
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

	// root and rel locate the file; the read never leaves root.
	root string
	rel  string
	data []byte
}

// Collect lists what a report would hold, without reading the files, so the
// player can see it before agreeing.
func Collect(in Input) []Item {
	items := []Item{{Name: "about.txt", What: "Launcher version and system", data: []byte(in.About), Size: int64(len(in.About))}}

	add := func(name, what, source string) {
		// Lstat, so a link, which could lead outside the folder, is never
		// taken.
		info, err := os.Lstat(source)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return
		}
		items = append(items, Item{Name: name, What: what, Size: capped(info.Size()), root: filepath.Dir(source), rel: filepath.Base(source)})
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
			// Ids come from state.json; one that isn't a plain server id
			// could point outside the Diablo II folder.
			if !serverID.MatchString(id) {
				continue
			}
			dir := install.ServerDir(in.Base, id)
			taken := map[string]bool{}
			for _, f := range gameCrashLogs(dir) {
				taken[strings.ToLower(filepath.Base(f))] = true
				add("game/"+id+"/"+filepath.Base(f), "Diablo II crash log", f)
			}
			items = append(items, serverFiles(id, dir, in.Files[id], taken)...)
		}
	}

	return items
}

// serverFiles finds the files a server's report patterns match, newest first
// per pattern, skipping any already taken. It looks through an os.Root, so
// a linked folder can't lead outside the server folder.
func serverFiles(id, dir string, patterns []string, taken map[string]bool) []Item {
	if len(patterns) == 0 {
		return nil
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil
	}
	defer root.Close()

	var items []Item
	var total int64
	for _, pattern := range patterns {
		// The profile was checked when it was loaded; this guards the
		// filesystem against a pattern that somehow wasn't.
		if paths.CheckPattern(pattern) != nil {
			continue
		}

		sub, name := path.Split(pattern)
		for _, rel := range matchNewest(root, strings.TrimSuffix(sub, "/"), name) {
			key := strings.ToLower(rel)
			if taken[key] {
				continue
			}
			info, err := root.Lstat(rel)
			if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
				continue
			}
			size := capped(info.Size())
			if total+size > serverBudget {
				continue
			}

			taken[key] = true
			total += size
			items = append(items, Item{Name: "game/" + id + "/" + rel, What: "Asked for by the server", Size: size, root: dir, rel: filepath.FromSlash(rel)})
		}
	}

	return items
}

// matchNewest lists the regular files in sub, a folder under root, whose
// names match pattern without regard to case, newest first, up to
// perPattern. The paths returned are relative to root, with forward slashes.
func matchNewest(root *os.Root, sub, pattern string) []string {
	folder := "."
	if sub != "" {
		folder = sub
	}

	f, err := root.Open(folder)
	if err != nil {
		return nil
	}
	entries, err := f.ReadDir(-1)
	f.Close()
	if err != nil {
		return nil
	}

	type found struct {
		rel string
		mod time.Time
	}
	var all []found
	pattern = strings.ToLower(pattern)
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		if ok, _ := path.Match(pattern, strings.ToLower(e.Name())); !ok {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		all = append(all, found{path.Join(sub, e.Name()), info.ModTime()})
	}

	sort.Slice(all, func(i, j int) bool { return all[i].mod.After(all[j].mod) })
	if len(all) > perPattern {
		all = all[:perPattern]
	}

	rels := make([]string, len(all))
	for i, f := range all {
		rels[i] = f.rel
	}

	return rels
}

// capped is how much of a file of size bytes a report takes.
func capped(size int64) int64 {
	if size > maxFile {
		return maxFile
	}

	return size
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
		if !e.Type().IsRegular() || !isGameCrashLog(name) {
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
		if it.root != "" {
			var err error
			data, err = readCapped(it.root, it.rel)
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
// is what matters. It opens the file within root, so even if it has been
// swapped for a link since it was listed, the read can't leave that folder.
func readCapped(root, rel string) ([]byte, error) {
	f, err := os.OpenInRoot(root, rel)
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

// replaceFold replaces old case-insensitively, as Windows paths are. The
// regexp folds case on the text as it is, so characters whose lower case is
// a different length, such as İ, don't throw the match off.
func replaceFold(data []byte, old, new string) []byte {
	re := regexp.MustCompile("(?i)" + regexp.QuoteMeta(old))
	return re.ReplaceAllLiteral(data, []byte(new))
}
