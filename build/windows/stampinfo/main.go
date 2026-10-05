// Command stampinfo writes a copy of build/windows/info.json with a release's
// version, so a tagged build's exe shows that version in its file properties
// rather than the 0.1.0 in config.yml. It also prints the numeric version
// that NSIS and Windows file versions need.
//
//	go run ./build/windows/stampinfo -in info.json -out info.stamped.json -version v1.2.3
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// A tag such as v1.2.3, optionally with a pre-release or build suffix.
var tagPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)([-+][0-9A-Za-z.+-]+)?$`)

func main() {
	in := flag.String("in", "", "info.json to read")
	out := flag.String("out", "", "file to write")
	version := flag.String("version", "", "release tag, such as v1.2.3")
	flag.Parse()

	if err := run(*in, *out, *version); err != nil {
		fmt.Fprintln(os.Stderr, "stampinfo:", err)
		os.Exit(1)
	}
}

func run(in, out, version string) error {
	numeric, full, err := parse(version)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(in)
	if err != nil {
		return err
	}

	stamped, err := stamp(data, numeric, full)
	if err != nil {
		return fmt.Errorf("%s: %w", in, err)
	}

	if err := os.WriteFile(out, stamped, 0o644); err != nil {
		return err
	}

	fmt.Println(numeric)
	return nil
}

// parse splits a tag into the numeric version Windows needs, 1.2.3, and the
// version shown to people, 1.2.3-beta.1.
func parse(tag string) (numeric, full string, err error) {
	m := tagPattern.FindStringSubmatch(tag)
	if m == nil {
		return "", "", fmt.Errorf("version %q is not like v1.2.3", tag)
	}

	return m[1] + "." + m[2] + "." + m[3], strings.TrimPrefix(tag, "v"), nil
}

// stamp sets the fixed file version and every language's ProductVersion and
// FileVersion, leaving the rest of info.json as it is.
func stamp(data []byte, numeric, full string) ([]byte, error) {
	var info map[string]any
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}

	fixed, _ := info["fixed"].(map[string]any)
	if fixed == nil {
		fixed = map[string]any{}
		info["fixed"] = fixed
	}
	fixed["file_version"] = numeric

	langs, _ := info["info"].(map[string]any)
	if len(langs) == 0 {
		return nil, fmt.Errorf("no \"info\" section")
	}
	for _, v := range langs {
		lang, ok := v.(map[string]any)
		if !ok {
			continue
		}
		lang["ProductVersion"] = full
		lang["FileVersion"] = full
	}

	return json.MarshalIndent(info, "", "\t")
}
