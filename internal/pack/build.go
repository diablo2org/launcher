package pack

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diablo2org/launcher/internal/fetch"
	"github.com/diablo2org/launcher/internal/paths"
	"github.com/diablo2org/launcher/internal/spec"
)

// Plan says how to build every manifest a server publishes in one go: the
// channels and each component version, from folders of game files. It lives
// in a JSON file next to the server's profile; see LoadPlan.
type Plan struct {
	// Profile is the server profile the manifests are for. The manifest URLs
	// come from it, so the plan never repeats them.
	Profile string `json:"profile"`
	// Source is the folder of game files, used by every entry that doesn't
	// name its own.
	Source Sources `json:"source,omitempty"`
	// Exclude applies to every entry, for junk such as logs and readmes.
	Exclude   []string    `json:"exclude,omitempty"`
	Manifests []PlanEntry `json:"manifests"`
}

// PlanEntry is one manifest to build: a channel, or a version of a component.
type PlanEntry struct {
	Channel   string `json:"channel,omitempty"`
	Component string `json:"component,omitempty"`
	Version   string `json:"version,omitempty"`
	// Source overrides the plan's Source, for a component kept in its own
	// folder.
	Source Sources `json:"source,omitempty"`
	// Files is the URL the files are published at, when that isn't the folder
	// the manifest is published in. A source with its own Files overrides it
	// for that source's files.
	Files   string   `json:"files,omitempty"`
	Once    []string `json:"once,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
	Only    []string `json:"only,omitempty"`
}

// Sources is one folder, or several layered in order: where two hold the
// same file, the later one's is used. In JSON it is a source or a list of
// them.
type Sources []Source

// Source is a folder of game files. In JSON it is the folder's path, or
// {"folder": ..., "files": ...} to publish its files at their own URL.
type Source struct {
	Folder string `json:"folder"`
	// Files is the URL this folder's files are published at, so a folder
	// several manifests layer, such as the base game files, can be uploaded
	// once and shared. It overrides the entry's Files.
	Files string `json:"files,omitempty"`
}

// errSource is how a source that isn't one of the accepted forms is
// reported.
var errSource = errors.New(`source must be a folder, {"folder": ..., "files": ...}, or a list of them`)

// UnmarshalJSON accepts a source or a list of sources.
func (s *Sources) UnmarshalJSON(data []byte) error {
	var one Source
	if err := one.UnmarshalJSON(data); err == nil {
		*s = Sources{one}
		return nil
	}

	var many []Source
	if err := json.Unmarshal(data, &many); err != nil {
		return errSource
	}
	*s = many

	return nil
}

// UnmarshalJSON accepts a folder's path, or an object naming the folder and
// where its files are published.
func (s *Source) UnmarshalJSON(data []byte) error {
	var folder string
	if err := json.Unmarshal(data, &folder); err == nil {
		*s = Source{Folder: folder}
		return nil
	}

	// A plain struct type, so decoding doesn't come back here.
	type source Source
	var v source
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil || v.Folder == "" {
		return errSource
	}
	*s = Source(v)

	return nil
}

func (e PlanEntry) name() string {
	if e.Channel != "" {
		return "channel " + e.Channel
	}

	return e.Component + " " + e.Version
}

// LoadPlan reads a plan. Relative paths in it are relative to the plan file.
func LoadPlan(file string) (*Plan, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	var p Plan
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("%s: unexpected text after the plan", file)
	}

	if p.Profile == "" {
		return nil, fmt.Errorf("%s: no profile", file)
	}
	if len(p.Manifests) == 0 {
		return nil, fmt.Errorf("%s: no manifests", file)
	}

	dir := filepath.Dir(file)
	resolve := func(s string) string {
		if s == "" || filepath.IsAbs(s) {
			return s
		}
		return filepath.Join(dir, s)
	}
	resolveAll := func(s Sources) {
		for i := range s {
			s[i].Folder = resolve(s[i].Folder)
		}
	}

	p.Profile = resolve(p.Profile)
	resolveAll(p.Source)
	for _, e := range p.Manifests {
		resolveAll(e.Source)
	}

	return &p, nil
}

// BuildOptions control Build.
type BuildOptions struct {
	// Version is written into every manifest.
	Version string
	// Out is the folder to build into. It must not exist yet.
	Out string
	// Key signs every manifest, and must be one of the profile's
	// signing.keys. It is needed exactly when the profile has keys.
	Key ed25519.PrivateKey
}

// Built is one manifest Build wrote.
type Built struct {
	Name     string
	Manifest string
	Files    int
	Bytes    int64
}

// BuildResult is what Build did.
type BuildResult struct {
	Built []Built
	// NotBuilt lists channels and component versions in the profile that the
	// plan has no entry for. Their published manifests are left as they are.
	NotBuilt []string
}

// Build builds every manifest in the plan into opts.Out, laid out as
// <host>/<path> mirroring their URLs, with each manifest's files next to it,
// ready to upload. Every manifest is checked against the profile. Out is
// written in full or not at all.
func Build(plan *Plan, opts BuildOptions) (*BuildResult, error) {
	if opts.Version == "" {
		return nil, errors.New("no version")
	}
	if opts.Out == "" {
		return nil, errors.New("no output folder")
	}
	if _, err := os.Stat(opts.Out); err == nil {
		return nil, fmt.Errorf("%s already exists; remove it or choose another folder", opts.Out)
	}

	data, err := os.ReadFile(plan.Profile)
	if err != nil {
		return nil, err
	}
	profile, err := spec.ParseProfile(data)
	if err != nil {
		return nil, fmt.Errorf("%s:\n%w", plan.Profile, err)
	}
	if err := checkKey(profile, opts.Key); err != nil {
		return nil, err
	}

	out, err := filepath.Abs(opts.Out)
	if err != nil {
		return nil, err
	}

	// Build in a new folder next to the output and rename at the end, so a
	// failed build never leaves something that looks ready to upload. Its
	// name is unique, so only this build's own folder is ever removed.
	parent := filepath.Dir(out)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp(parent, filepath.Base(out)+".partial-")
	if err != nil {
		return nil, err
	}

	b := &builder{plan: plan, profile: profile, out: out, tmp: tmp, version: opts.Version, key: opts.Key, written: map[string]string{}}
	result, err := b.run()
	if err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}

	if err := os.Rename(tmp, out); err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}

	return result, nil
}

type builder struct {
	plan    *Plan
	profile *spec.Profile
	out     string
	tmp     string
	version string
	key     ed25519.PrivateKey
	// written maps each output file, by its Windows key, to its SHA-256, so
	// two manifests sharing a folder can share a file but not clash.
	written map[string]string
}

func (b *builder) run() (*BuildResult, error) {
	result := &BuildResult{}
	done := map[string]bool{}

	for _, e := range b.plan.Manifests {
		manifestURL, err := b.manifestURL(e)
		if err != nil {
			return nil, err
		}
		if done[manifestURL] {
			return nil, fmt.Errorf("%s: listed twice in the plan", e.name())
		}
		done[manifestURL] = true

		built, err := b.build(e, manifestURL)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.name(), err)
		}
		result.Built = append(result.Built, *built)
	}

	for _, c := range b.profile.Channels {
		if !done[c.Manifest] {
			result.NotBuilt = append(result.NotBuilt, "channel "+c.ID)
		}
	}
	for _, c := range b.profile.Components {
		for _, v := range c.Versions {
			if !done[v.Manifest] {
				result.NotBuilt = append(result.NotBuilt, c.ID+" "+v.ID)
			}
		}
	}

	return result, nil
}

// manifestURL finds where the profile says an entry's manifest is published.
func (b *builder) manifestURL(e PlanEntry) (string, error) {
	switch {
	case e.Channel != "" && e.Component != "":
		return "", fmt.Errorf("an entry names both channel %q and component %q", e.Channel, e.Component)

	case e.Channel != "":
		if e.Version != "" {
			return "", fmt.Errorf("channel %s: version is only for components", e.Channel)
		}
		for _, c := range b.profile.Channels {
			if c.ID == e.Channel {
				return c.Manifest, nil
			}
		}
		return "", fmt.Errorf("the profile has no channel %q", e.Channel)

	case e.Component != "":
		if e.Version == "" {
			return "", fmt.Errorf("component %s: no version", e.Component)
		}
		for _, c := range b.profile.Components {
			if c.ID != e.Component {
				continue
			}
			for _, v := range c.Versions {
				if v.ID == e.Version {
					return v.Manifest, nil
				}
			}
			return "", fmt.Errorf("the profile's %s component has no version %q", e.Component, e.Version)
		}
		return "", fmt.Errorf("the profile has no component %q", e.Component)
	}

	return "", errors.New("an entry names neither a channel nor a component")
}

func (b *builder) build(e PlanEntry, manifestURL string) (*Built, error) {
	sources := e.Source
	if len(sources) == 0 {
		sources = b.plan.Source
	}
	if len(sources) == 0 {
		return nil, errors.New("no source folder")
	}
	folders := make([]string, len(sources))
	for i, source := range sources {
		folders[i] = source.Folder
		if info, err := os.Stat(source.Folder); err != nil || !info.IsDir() {
			return nil, fmt.Errorf("source %s is not a folder", source.Folder)
		}
		in, err := contains(source.Folder, b.tmp)
		if err != nil {
			return nil, err
		}
		if in {
			return nil, fmt.Errorf("the output folder can't be inside the source folder %s", source.Folder)
		}
	}

	manifestRel, err := urlPath(manifestURL)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(manifestURL, "/") {
		return nil, fmt.Errorf("manifest URL %s doesn't name a file", manifestURL)
	}

	// Files are published beside the manifest, unless the entry or the
	// source they come from says where.
	entryBase := e.Files
	if entryBase == "" {
		u, _ := url.Parse(manifestURL)
		u.Path = path.Dir(u.Path)
		u.RawPath = ""
		entryBase = u.String()
	}

	// Layer the sources: a later folder's file replaces an earlier one's.
	type layered struct {
		file    spec.File
		source  string
		baseRel string
	}
	var files []layered
	index := map[string]int{}

	for _, source := range sources {
		base := entryBase
		if source.Files != "" {
			base = source.Files
		}
		baseRel, err := urlPath(base)
		if err != nil {
			return nil, err
		}

		m, err := BuildManifest(source.Folder, Options{
			Server:  b.profile.ID,
			Version: b.version,
			BaseURL: base,
			Once:    e.Once,
			Exclude: append(append([]string{}, b.plan.Exclude...), e.Exclude...),
			Only:    e.Only,
		})
		if err != nil {
			return nil, err
		}

		for _, f := range m.Files {
			l := layered{f, source.Folder, baseRel}
			key := paths.Key(f.Path)
			if i, ok := index[key]; ok {
				files[i] = l
				continue
			}
			index[key] = len(files)
			files = append(files, l)
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no files in %s matched", strings.Join(folders, ", "))
	}

	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i].file.Path) < strings.ToLower(files[j].file.Path)
	})

	m := &spec.Manifest{Schema: 1, Server: b.profile.ID, Version: b.version, Files: make([]spec.File, 0, len(files))}
	built := &Built{Name: e.name(), Manifest: filepath.Join(b.out, filepath.FromSlash(manifestRel)), Files: len(files)}

	for _, l := range files {
		dst := path.Join(l.baseRel, l.file.Path)
		if err := b.place(filepath.Join(l.source, filepath.FromSlash(l.file.Path)), dst, l.file); err != nil {
			return nil, err
		}
		m.Files = append(m.Files, l.file)
		built.Bytes += l.file.Size
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')

	if _, err := spec.ParseManifest(data, b.profile); err != nil {
		return nil, err
	}

	key := paths.Key(manifestRel)
	if _, ok := b.written[key]; ok {
		return nil, fmt.Errorf("%s is also a file another manifest publishes", manifestRel)
	}
	b.written[key] = ""

	dst := filepath.Join(b.tmp, filepath.FromSlash(manifestRel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return nil, err
	}

	if b.key != nil {
		if _, ok := b.written[key+".sig"]; ok {
			return nil, fmt.Errorf("%s.sig is also a file another manifest publishes", manifestRel)
		}
		b.written[key+".sig"] = ""
		if err := os.WriteFile(dst+".sig", spec.SignManifest(data, b.key), 0o644); err != nil {
			return nil, err
		}
	}

	return built, nil
}

// place copies a file into the output, and checks the copy still matches the
// manifest, so a file changing during the build is caught here and not by
// players' launchers.
func (b *builder) place(src, rel string, f spec.File) error {
	key := paths.Key(rel)
	if sha, ok := b.written[key]; ok {
		if sha != f.SHA256 {
			return fmt.Errorf("%s: another manifest publishes a different file at the same URL", rel)
		}
		return nil
	}
	b.written[key] = f.SHA256

	dst := filepath.Join(b.tmp, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}

	size, sha, err := fetch.HashFile(dst)
	if err != nil {
		return err
	}
	if size != f.Size || sha != f.SHA256 {
		return fmt.Errorf("%s changed while building; run it again", f.Path)
	}

	return nil
}

// urlPath turns an https URL into the path it has in the output folder:
// its host, then its path.
func urlPath(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return "", fmt.Errorf("%q is not an https URL", raw)
	}
	// A query or fragment can't be mirrored by a folder, and file names
	// appended to it would end up in the query, not the path.
	if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", fmt.Errorf("%q can't have a query or fragment", raw)
	}

	rel := strings.ToLower(u.Hostname())
	if p := strings.Trim(path.Clean("/"+u.Path), "/"); p != "" {
		rel += "/" + p
	}
	if err := paths.Check(rel); err != nil {
		return "", fmt.Errorf("%s: %w", raw, err)
	}

	return rel, nil
}

// contains reports whether dir is under source. It walks source the way
// BuildManifest does, which doesn't follow links, and compares folders by
// identity, so dir can't hide behind a symlink or junction to any folder in
// the source.
func contains(source, dir string) (bool, error) {
	want, err := os.Stat(dir)
	if err != nil {
		return false, err
	}

	found := false
	err = filepath.WalkDir(source, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}

		info, err := os.Stat(p)
		if err != nil {
			return err
		}
		if os.SameFile(want, info) {
			found = true
			return filepath.SkipAll
		}

		return nil
	})

	return found, err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}

	return out.Close()
}
