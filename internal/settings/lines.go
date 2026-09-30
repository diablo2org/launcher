// Package settings reads and writes the file-backed settings a server
// declares in its profile. Edits are surgical: only the value of the declared
// key changes, and every other byte of the file, including comments, alignment
// and line endings, is left as it was.
package settings

import "strings"

// document is a text file split into lines, remembering the line ending it
// used so a rewritten file keeps it.
type document struct {
	lines []string
	eol   string
	// trailing records whether the file ended with a line ending.
	trailing bool
}

func parse(contents string) *document {
	eol := "\n"
	if strings.Contains(contents, "\r\n") {
		eol = "\r\n"
	}

	trailing := strings.HasSuffix(contents, "\n")
	body := strings.TrimSuffix(strings.TrimSuffix(contents, "\n"), "\r")

	lines := strings.Split(body, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSuffix(line, "\r")
	}

	if contents == "" {
		lines = nil
	}

	return &document{lines: lines, eol: eol, trailing: trailing}
}

func (d *document) String() string {
	out := strings.Join(d.lines, d.eol)
	if d.trailing {
		out += d.eol
	}

	return out
}

// insert puts new lines after index at (use -1 for the start).
func (d *document) insert(at int, add ...string) {
	rest := append([]string{}, d.lines[at+1:]...)
	d.lines = append(append(d.lines[:at+1], add...), rest...)
}
