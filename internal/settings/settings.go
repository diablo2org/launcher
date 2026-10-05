package settings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/diablo2org/launcher/internal/paths"
	"github.com/diablo2org/launcher/internal/spec"
)

// ErrFileMissing means the setting's file doesn't exist yet. The launcher
// never creates it: a missing file usually means its component hasn't been
// installed, and creating one could stop a "once" file ever arriving.
var ErrFileMissing = errors.New("settings file not installed")

// Read returns a file setting's current value in its typed form: bool, int
// or string. A key the file doesn't carry reports the setting's default.
func Read(dir string, s spec.Setting) (interface{}, error) {
	if s.Target.Flag != "" {
		return nil, fmt.Errorf("setting %q is a launch flag, not a file setting", s.ID)
	}

	contents, err := readFile(dir, s.Target.File)
	if err != nil {
		return nil, err
	}

	var raw string
	var found bool
	switch s.Target.Format {
	case "ini":
		raw, found = readINI(contents, s.Target.Section, s.Target.Key)
	case "bh":
		raw, found = readBH(contents, s.Target.Key, s.Type == "bool")
	default:
		return nil, fmt.Errorf("setting %q: unknown format %q", s.ID, s.Target.Format)
	}

	if !found {
		return defaultValue(s), nil
	}

	return typed(s, raw), nil
}

// Write sets a file setting to value, which must match the setting's type.
func Write(dir string, s spec.Setting, value interface{}) error {
	if s.Target.Flag != "" {
		return fmt.Errorf("setting %q is a launch flag, not a file setting", s.ID)
	}

	raw, err := format(s, value)
	if err != nil {
		return err
	}

	contents, err := readFile(dir, s.Target.File)
	if err != nil {
		return err
	}

	var updated string
	switch s.Target.Format {
	case "ini":
		updated = writeINI(contents, s.Target.Section, s.Target.Key, raw)
	case "bh":
		updated = writeBH(contents, s.Target.Key, raw, s.Type == "bool")
	default:
		return fmt.Errorf("setting %q: unknown format %q", s.ID, s.Target.Format)
	}

	if updated == contents {
		return nil
	}

	return writeFile(dir, s.Target.File, updated)
}

// SetINIKeys sets several keys in one section of an existing ini file, with
// the same surgical edits as Write. It is for launcher features such as d2gl
// box profiles, not for settings a server declares.
func SetINIKeys(dir, rel, section string, values map[string]string) error {
	contents, err := readFile(dir, rel)
	if err != nil {
		return err
	}

	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	updated := contents
	for _, k := range keys {
		updated = writeINI(updated, section, k, values[k])
	}

	if updated == contents {
		return nil
	}

	return writeFile(dir, rel, updated)
}

func readFile(dir, rel string) (string, error) {
	if err := paths.Check(rel); err != nil {
		return "", err
	}

	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrFileMissing
	}

	return string(data), err
}

// writeFile replaces the file through a temporary copy, so a crash midway
// never leaves a half-written settings file behind.
func writeFile(dir, rel, contents string) error {
	path := filepath.Join(dir, filepath.FromSlash(rel))

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	tmp := path + ".launcher-tmp"
	if err := os.WriteFile(tmp, []byte(contents), info.Mode().Perm()); err != nil {
		return err
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}

	return nil
}

func defaultValue(s spec.Setting) interface{} {
	switch s.Type {
	case "bool":
		if b, ok := s.Default.(bool); ok {
			return b
		}
		return false
	case "int":
		if n, ok := s.Default.(float64); ok {
			return int(n)
		}
		return 0
	default:
		if v, ok := s.Default.(string); ok {
			return v
		}
		return ""
	}
}

func typed(s spec.Setting, raw string) interface{} {
	switch s.Type {
	case "bool":
		return strings.EqualFold(raw, "true") || raw == "1"
	case "int":
		n, err := strconv.Atoi(raw)
		if err != nil {
			return defaultValue(s)
		}
		return n
	default:
		return raw
	}
}

// format checks value against the setting and renders it the way the file's
// format spells it.
func format(s spec.Setting, value interface{}) (string, error) {
	switch s.Type {
	case "bool":
		b, ok := value.(bool)
		if !ok {
			return "", fmt.Errorf("setting %q needs true or false", s.ID)
		}
		if s.Target.Format == "bh" {
			if b {
				return "True", nil
			}
			return "False", nil
		}
		return strconv.FormatBool(b), nil

	case "int":
		var n int
		switch v := value.(type) {
		case int:
			n = v
		case float64:
			if v != float64(int(v)) {
				return "", fmt.Errorf("setting %q needs a whole number", s.ID)
			}
			n = int(v)
		default:
			return "", fmt.Errorf("setting %q needs a whole number", s.ID)
		}
		if (s.Min != nil && n < *s.Min) || (s.Max != nil && n > *s.Max) {
			return "", fmt.Errorf("setting %q: %d is out of range", s.ID, n)
		}
		return strconv.Itoa(n), nil

	case "choice":
		v, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("setting %q needs one of its options", s.ID)
		}
		for _, o := range s.Options {
			if o.Value == v {
				return v, nil
			}
		}
		return "", fmt.Errorf("setting %q: %q is not one of its options", s.ID, v)

	case "string":
		v, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("setting %q needs text", s.ID)
		}
		if s.MaxLength > 0 && len(v) > s.MaxLength {
			return "", fmt.Errorf("setting %q: longer than %d characters", s.ID, s.MaxLength)
		}
		if strings.ContainsAny(v, "\r\n") || strings.Contains(v, bhComment) || strings.Contains(v, ";") {
			// Line breaks or comment markers would let a value rewrite the
			// rest of the file.
			return "", fmt.Errorf("setting %q: value contains a line break or comment marker", s.ID)
		}
		return v, nil
	}

	return "", fmt.Errorf("setting %q: unknown type %q", s.ID, s.Type)
}
