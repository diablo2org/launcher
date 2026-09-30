package settings

import "strings"

// bh files are the BH maphack's "Key: Value" format, with "//" starting a
// comment. Boolean values come in three shapes, "True", "True, None" and
// "True, VK_K", so for a bool only the leading word is replaced and a hotkey
// after it survives.

const bhComment = "//"

// bhAddedMarker heads the keys the launcher added to a file that lacked them.
const bhAddedMarker = bhComment + " Added by the launcher"

func hasLine(d *document, line string) bool {
	for _, l := range d.lines {
		if strings.TrimSpace(l) == line {
			return true
		}
	}

	return false
}

func bhKey(line string) (key string, value string, ok bool) {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, bhComment) {
		return "", "", false
	}

	// Only the first colon separates key from value; some values carry more.
	colon := strings.IndexByte(t, ':')
	if colon <= 0 {
		return "", "", false
	}

	return strings.TrimSpace(t[:colon]), strings.TrimSpace(t[colon+1:]), true
}

func bhFind(d *document, key string) int {
	for i, line := range d.lines {
		if k, _, ok := bhKey(line); ok && k == key {
			return i
		}
	}

	return -1
}

func stripBHComment(value string) string {
	if i := strings.Index(value, bhComment); i >= 0 {
		return strings.TrimSpace(value[:i])
	}

	return value
}

func readBH(contents, key string, isBool bool) (string, bool) {
	d := parse(contents)

	i := bhFind(d, key)
	if i == -1 {
		return "", false
	}

	_, value, _ := bhKey(d.lines[i])
	value = stripBHComment(value)

	if isBool {
		if c := strings.IndexByte(value, ','); c >= 0 {
			value = strings.TrimSpace(value[:c])
		}
	}

	return value, true
}

func writeBH(contents, key, value string, isBool bool) string {
	d := parse(contents)

	i := bhFind(d, key)
	if i == -1 {
		// Keys the file doesn't carry go at the end under one marker, so a
		// trimmed down settings file still takes the player's choice.
		for len(d.lines) > 0 && strings.TrimSpace(d.lines[len(d.lines)-1]) == "" {
			d.lines = d.lines[:len(d.lines)-1]
		}
		if !hasLine(d, bhAddedMarker) {
			if len(d.lines) > 0 {
				d.lines = append(d.lines, "")
			}
			d.lines = append(d.lines, bhAddedMarker)
		}
		d.lines = append(d.lines, key+": "+value)
		d.trailing = true

		return d.String()
	}

	line := d.lines[i]
	colon := strings.IndexByte(line, ':')
	head, tail := line[:colon+1], line[colon+1:]

	// Hold a trailing comment aside so it isn't mistaken for part of the value.
	comment := ""
	if c := strings.Index(tail, bhComment); c >= 0 {
		comment = strings.TrimRight(tail[:c], " \t")
		comment = tail[len(comment):]
		tail = tail[:len(tail)-len(comment)]
	}

	// Keep the run of spaces that lines values up in columns.
	lead := tail[:len(tail)-len(strings.TrimLeft(tail, " \t"))]

	rest := ""
	if isBool {
		if c := strings.IndexByte(tail, ','); c >= 0 {
			rest = strings.TrimRight(tail[c:], " \t")
		}
	}

	d.lines[i] = head + lead + value + rest + comment

	return d.String()
}
