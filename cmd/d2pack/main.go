// Command d2pack is for server teams: it writes a starter server profile and
// builds the file manifests the launcher downloads from.
//
//	d2pack init -id <id> -name <name> -gateway <host> -manifest <url>
//	d2pack manifest -server <id> -version <v> -url <base url> [-key <file>] [-once <pattern>]... [-exclude <pattern>]... [-only <pattern>]... <folder>
//	d2pack build [-version <v>] [-out <folder>] [-key <file>] <plan.json>
//	d2pack check -profile <profile.json> [<manifest.json>...]
//	d2pack keygen -out <file>
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diablo2org/launcher/internal/pack"
	"github.com/diablo2org/launcher/internal/spec"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "init":
		err = initProfile(os.Args[2:])
	case "manifest":
		err = manifest(os.Args[2:])
	case "build":
		err = build(os.Args[2:])
	case "check":
		err = check(os.Args[2:])
	case "keygen":
		err = keygen(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "d2pack:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `d2pack gets a Diablo II server ready for the launcher.

  d2pack init -id <id> -name <name> -gateway <host> -manifest <url>
      Write <id>.json, a starter server profile. Add it to the launcher
      repository's servers/ folder in a pull request.

  d2pack manifest -server <id> -version <v> -url <base url> [-key <file>] [-once <pattern>] [-exclude <pattern>] [-only <pattern>] <folder>
      Write <folder>/manifest.json listing every file in <folder> with its
      size and SHA-256, to be published at <base url> next to the files.
      Run it again whenever you patch. Blizzard's base archives are always
      left out. -once marks files the player owns after the first install;
      -only keeps just matching files, to build a component's manifest.
      -key signs it, writing manifest.json.sig to upload beside it.

  d2pack build [-version <v>] [-out <folder>] [-key <file>] <plan.json>
      Build every manifest in a plan at once: each channel and component
      version, with its files, laid out by URL in <folder> ready to upload.
      The URLs come from the profile the plan names. -version defaults to
      today's date, -out to upload/<version> next to the plan. A profile
      with signing keys needs -key, and every manifest gets a .sig.

  d2pack check -profile <profile.json> [<manifest.json>...]
      Check a profile, and manifests against it, with their signatures
      when the profile has signing keys.

  d2pack keygen -out <file>
      Make a manifest signing key. Keep <file> secret and out of the
      repository; add the public key it prints to the profile's
      signing.keys.
`)
}

type list []string

func (l *list) String() string     { return strings.Join(*l, ",") }
func (l *list) Set(v string) error { *l = append(*l, v); return nil }

func initProfile(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	id := fs.String("id", "", "server id: lowercase letters, digits and -, e.g. slashdiablo")
	name := fs.String("name", "", "display name, e.g. SlashDiablo")
	gateway := fs.String("gateway", "", "Battle.net gateway host, e.g. play.slashdiablo.net")
	manifestURL := fs.String("manifest", "", "https URL your live manifest.json will be published at")
	game := fs.String("game", "1.13c", "game version: 1.13c, 1.13d or 1.14d")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" || *name == "" || *gateway == "" || *manifestURL == "" {
		return errors.New("usage: d2pack init -id <id> -name <name> -gateway <host> -manifest <url>")
	}

	out := *id + ".json"
	if _, err := os.Stat(out); err == nil {
		return fmt.Errorf("%s already exists; not overwriting it", out)
	}

	data, err := pack.InitProfile(pack.InitOptions{
		ID: *id, Name: *name, Gateway: *gateway, GameVersion: *game, ManifestURL: *manifestURL,
	})
	if err != nil {
		return err
	}

	if err := os.WriteFile(out, data, 0o644); err != nil {
		return err
	}

	fmt.Printf("Wrote %s. Add your summary, links and branding, check it with\n  d2pack check -profile %s\nthen open a pull request adding it to servers/ in the launcher repository.\n", out, out)
	return nil
}

func manifest(args []string) error {
	fs := flag.NewFlagSet("manifest", flag.ContinueOnError)
	server := fs.String("server", "", "server id, as in the profile")
	version := fs.String("version", "", "manifest version, e.g. 2026.09.29")
	base := fs.String("url", "", "https URL the folder is published at")
	var once, exclude, only list
	fs.Var(&once, "once", "pattern for files the player owns after install (repeatable)")
	fs.Var(&exclude, "exclude", "pattern for files to leave out (repeatable)")
	fs.Var(&only, "only", "keep just files matching this pattern, for a component (repeatable)")
	keyFile := fs.String("key", "", "signing key from d2pack keygen, to sign the manifest")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 || *server == "" || *version == "" || *base == "" {
		return errors.New("usage: d2pack manifest -server <id> -version <v> -url <base url> <folder>")
	}

	dir := fs.Arg(0)
	m, err := pack.BuildManifest(dir, pack.Options{
		Server: *server, Version: *version, BaseURL: *base, Once: once, Exclude: exclude, Only: only,
	})
	if err != nil {
		return err
	}

	out := filepath.Join(dir, "manifest.json")
	if err := pack.WriteManifest(m, out); err != nil {
		return err
	}
	if *keyFile != "" {
		key, err := pack.LoadKey(*keyFile)
		if err != nil {
			return err
		}
		if err := pack.SignManifestFile(out, key); err != nil {
			return err
		}
		fmt.Printf("Signed it: upload %s.sig beside it.\n", out)
	}

	var total int64
	for _, f := range m.Files {
		total += f.Size
	}
	fmt.Printf("Wrote %s: %d files, %.1f MB. Upload it with the files, to %s/manifest.json\n",
		out, len(m.Files), float64(total)/(1<<20), strings.TrimSuffix(*base, "/"))

	return nil
}

func build(args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	version := fs.String("version", time.Now().Format("2006.01.02"), "manifest version")
	out := fs.String("out", "", "folder to build into; must not exist yet")
	keyFile := fs.String("key", "", "signing key from d2pack keygen; needed when the profile has signing keys")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: d2pack build [-version <v>] [-out <folder>] [-key <file>] <plan.json>")
	}

	opts := pack.BuildOptions{Version: *version}
	if *keyFile != "" {
		key, err := pack.LoadKey(*keyFile)
		if err != nil {
			return err
		}
		opts.Key = key
	}

	planFile := fs.Arg(0)
	plan, err := pack.LoadPlan(planFile)
	if err != nil {
		return err
	}

	if *out == "" {
		*out = filepath.Join(filepath.Dir(planFile), "upload", *version)
	}

	opts.Out = *out
	result, err := pack.Build(plan, opts)
	if err != nil {
		return err
	}

	for _, b := range result.Built {
		fmt.Printf("%-20s %4d files, %7.1f MB  %s\n", b.Name, b.Files, float64(b.Bytes)/(1<<20), b.Manifest)
	}
	for _, n := range result.NotBuilt {
		fmt.Printf("%-20s not in the plan; its published manifest is unchanged\n", n)
	}
	fmt.Printf("\nUpload the contents of each host folder in %s to the root of that host.\n", *out)

	return nil
}

func check(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	profileFile := fs.String("profile", "", "profile.json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *profileFile == "" {
		return errors.New("usage: d2pack check -profile <profile.json> [<manifest.json>...]")
	}

	data, err := os.ReadFile(*profileFile)
	if err != nil {
		return err
	}

	p, err := spec.ParseProfile(data)
	if err != nil {
		return fmt.Errorf("%s:\n%w", *profileFile, err)
	}
	fmt.Printf("%s: OK (%s)\n", *profileFile, p.Name)

	keys, err := p.ManifestKeys()
	if err != nil {
		return err
	}

	failed := false
	for _, mf := range fs.Args() {
		data, err := os.ReadFile(mf)
		if err != nil {
			return err
		}

		if _, err := spec.ParseManifest(data, p); err != nil {
			fmt.Printf("%s:\n%v\n", mf, err)
			failed = true
			continue
		}
		if keys != nil {
			sig, err := os.ReadFile(mf + ".sig")
			if err == nil {
				err = spec.VerifyManifest(data, sig, keys)
			}
			if err != nil {
				fmt.Printf("%s: %v\n", mf, err)
				failed = true
				continue
			}
			fmt.Printf("%s: OK, signed\n", mf)
			continue
		}
		fmt.Printf("%s: OK\n", mf)
	}

	if failed {
		return errors.New("some manifests have problems")
	}

	return nil
}

func keygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	out := fs.String("out", "", "file to write the private key to")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("usage: d2pack keygen -out <file>")
	}

	private, public, err := pack.GenerateKey()
	if err != nil {
		return err
	}

	// O_EXCL, so an existing key is never replaced: losing one means
	// re-signing everything under a new key.
	f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(private); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	fmt.Printf(`Wrote the private key to %s. Keep it secret and out of your repository:
whoever has it can sign manifests the launcher will trust.

Add the public key to your profile, in a pull request:

  "signing": { "keys": [%q] }

Then sign every manifest: d2pack build -key %s <plan.json>
`, *out, public, *out)

	return nil
}
