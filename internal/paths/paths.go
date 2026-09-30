// Package paths checks the relative paths servers use for files inside their
// folder. The schemas reject the obvious escapes; this adds the Windows rules
// a regular expression can't express well.
package paths

import (
	"errors"
	"fmt"
	"strings"
)

// reserved are the device names Windows refuses as a file name, with or
// without an extension.
var reserved = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// Check reports whether path is safe to use under a server folder: relative,
// forward slashes only, and no part that climbs out, is empty, names a device
// or that Windows would silently rename.
func Check(path string) error {
	if path == "" {
		return errors.New("empty path")
	}

	if strings.ContainsAny(path, `\:*?"<>|`) {
		return fmt.Errorf("%q: contains a character not allowed in a path", path)
	}

	if strings.HasPrefix(path, "/") {
		return fmt.Errorf("%q: must be relative", path)
	}

	for _, part := range strings.Split(path, "/") {
		switch {
		case part == "":
			return fmt.Errorf("%q: empty path segment", path)
		case part == "." || part == "..":
			return fmt.Errorf("%q: must not contain %q", path, part)
		case strings.HasSuffix(part, ".") || strings.HasSuffix(part, " "):
			// Windows drops trailing dots and spaces, so "a.dll." would land
			// on "a.dll" and slip past duplicate checks.
			return fmt.Errorf("%q: segment %q ends with a dot or space", path, part)
		}

		for _, r := range part {
			if r < 0x20 {
				return fmt.Errorf("%q: contains a control character", path)
			}
		}

		base := strings.ToUpper(part)
		if i := strings.IndexByte(base, '.'); i >= 0 {
			base = base[:i]
		}

		if reserved[base] {
			return fmt.Errorf("%q: %q is a reserved name on Windows", path, part)
		}
	}

	return nil
}

// Key is the form two paths are compared in. Windows paths are
// case-insensitive, so "Game.exe" and "game.exe" are the same file.
func Key(path string) string {
	return strings.ToLower(path)
}
