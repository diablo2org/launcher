package settings

import "strings"

// ini files are "key=value" lines under "[section]" headers, with ";" or "#"
// starting a comment line. Keys and sections compare case-insensitively.

func iniSection(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if len(t) < 2 || t[0] != '[' || t[len(t)-1] != ']' {
		return "", false
	}

	return strings.TrimSpace(t[1 : len(t)-1]), true
}

func iniKey(line string) (key string, value string, ok bool) {
	t := strings.TrimSpace(line)
	if t == "" || t[0] == ';' || t[0] == '#' {
		return "", "", false
	}

	eq := strings.IndexByte(t, '=')
	if eq <= 0 {
		return "", "", false
	}

	return strings.TrimSpace(t[:eq]), strings.TrimSpace(t[eq+1:]), true
}

// iniFind returns the line index of key within section, and the index of the
// last line belonging to the section (its header when it has no keys).
// Either is -1 when absent.
func iniFind(d *document, section, key string) (keyLine, sectionEnd int) {
	keyLine, sectionEnd = -1, -1
	in := false

	for i, line := range d.lines {
		if name, ok := iniSection(line); ok {
			in = strings.EqualFold(name, section)
			if in {
				sectionEnd = i
			}
			continue
		}

		if !in {
			continue
		}

		if k, _, ok := iniKey(line); ok {
			sectionEnd = i
			if keyLine == -1 && strings.EqualFold(k, key) {
				keyLine = i
			}
		}
	}

	return keyLine, sectionEnd
}

func readINI(contents, section, key string) (string, bool) {
	d := parse(contents)

	i, _ := iniFind(d, section, key)
	if i == -1 {
		return "", false
	}

	_, value, _ := iniKey(d.lines[i])

	return stripINIComment(value), true
}

// stripINIComment drops a trailing "; comment" from a value.
func stripINIComment(value string) string {
	if i := strings.Index(value, " ;"); i >= 0 {
		return strings.TrimSpace(value[:i])
	}

	return value
}

func writeINI(contents, section, key, value string) string {
	d := parse(contents)

	i, end := iniFind(d, section, key)
	switch {
	case i >= 0:
		// Keep the key as spelled in the file, and any indentation or
		// trailing comment around the value.
		line := d.lines[i]
		eq := strings.IndexByte(line, '=')
		rest := line[eq+1:]

		lead := rest[:len(rest)-len(strings.TrimLeft(rest, " \t"))]
		comment := ""
		if c := strings.Index(rest, " ;"); c >= 0 {
			comment = rest[c:]
		}

		d.lines[i] = line[:eq+1] + lead + value + comment
	case end >= 0:
		d.insert(end, key+"="+value)
	default:
		if len(d.lines) > 0 && strings.TrimSpace(d.lines[len(d.lines)-1]) != "" {
			d.lines = append(d.lines, "")
		}
		d.lines = append(d.lines, "["+section+"]", key+"="+value)
		d.trailing = true
	}

	return d.String()
}
