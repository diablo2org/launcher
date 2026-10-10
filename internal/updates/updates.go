// Package updates checks whether a newer launcher has been released, and
// finds its installer so the launcher can download and run it.
package updates

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/diablo2org/launcher/internal/fetch"
)

// Release is a newer launcher release.
type Release struct {
	Version string `json:"version"`
	// URL is the release page.
	URL string `json:"url"`
	// Installer is nil when the release has no installer for this machine
	// with a known hash; then the player is sent to the release page.
	Installer *Installer `json:"installer"`
}

// Installer is a release's installer, with what it must match once
// downloaded.
type Installer struct {
	Name   string `json:"name"`
	URL    string `json:"-"`
	Size   int64  `json:"size"`
	SHA256 string `json:"-"`
}

// Hosts are where releases and their files are fetched from: the API, then
// release downloads, which redirect to GitHub's file hosts.
var Hosts = []string{"api.github.com", "github.com", "objects.githubusercontent.com", "release-assets.githubusercontent.com"}

// downloadPrefix is where this repository's release files are published;
// installers anywhere else are ignored.
var downloadPrefix = "https://github.com/diablo2org/launcher/releases/download/"

// UseSource points update checks at base instead of GitHub, for testing
// against a local server: base/latest is the release, and its files are
// under base/releases/download/.
func UseSource(base string) {
	base = strings.TrimSuffix(base, "/")
	LatestURL = base + "/latest"
	downloadPrefix = base + "/releases/download/"
}

// MaxInstaller is the largest installer accepted.
const MaxInstaller = 200 << 20

// checksums is the file CI publishes with each release: "<sha256>  <name>".
const checksums = "SHA256SUMS.txt"

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// LatestURL is the GitHub API for the newest non-prerelease release.
var LatestURL = "https://api.github.com/repos/diablo2org/launcher/releases/latest"

// Check returns the newest release if it is newer than current, or nil.
// Development builds ("dev") never report an update.
func Check(ctx context.Context, c *fetch.Client, latestURL, current string) (*Release, error) {
	if _, ok := parse(current); !ok {
		return nil, nil
	}

	data, err := c.Document(ctx, latestURL)
	if err != nil {
		return nil, err
	}

	var r struct {
		TagName    string  `json:"tag_name"`
		HTMLURL    string  `json:"html_url"`
		Draft      bool    `json:"draft"`
		Prerelease bool    `json:"prerelease"`
		Assets     []asset `json:"assets"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}

	if r.Draft || r.Prerelease || !strings.HasPrefix(r.HTMLURL, "https://github.com/") || !Newer(r.TagName, current) {
		return nil, nil
	}

	rel := &Release{Version: r.TagName, URL: r.HTMLURL}

	// Without an installer there's still the release page, so a problem
	// finding one isn't an error.
	rel.Installer, _ = installer(ctx, c, r.TagName, r.Assets)

	return rel, nil
}

type asset struct {
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	DownloadURL string `json:"browser_download_url"`
	// Digest is "sha256:<hex>", which GitHub computes for uploaded files.
	Digest string `json:"digest"`
}

// installer picks this machine's installer, as CI names it, and finds its
// SHA-256: GitHub's digest, else the release's checksums file.
func installer(ctx context.Context, c *fetch.Client, tag string, assets []asset) (*Installer, error) {
	want := "-" + runtime.GOARCH + "-installer.exe"
	base := downloadPrefix + tag + "/"

	var found, sums *asset
	for i := range assets {
		a := &assets[i]
		if !strings.HasPrefix(a.DownloadURL, base) || a.DownloadURL != base+a.Name {
			continue
		}
		switch {
		case strings.HasSuffix(a.Name, want) && !strings.ContainsAny(a.Name, `/\`):
			found = a
		case a.Name == checksums:
			sums = a
		}
	}
	if found == nil {
		return nil, fmt.Errorf("no %s installer in %s", runtime.GOARCH, tag)
	}
	if found.Size <= 0 || found.Size > MaxInstaller {
		return nil, fmt.Errorf("%s: unexpected size %d", found.Name, found.Size)
	}

	sha := strings.TrimPrefix(found.Digest, "sha256:")
	if !sha256Pattern.MatchString(sha) {
		if sums == nil {
			return nil, fmt.Errorf("%s has no checksum", found.Name)
		}
		data, err := c.Document(ctx, sums.DownloadURL)
		if err != nil {
			return nil, err
		}
		sha = sumFor(data, found.Name)
		if sha == "" {
			return nil, fmt.Errorf("%s isn't in %s", found.Name, checksums)
		}
	}

	return &Installer{Name: found.Name, URL: found.DownloadURL, Size: found.Size, SHA256: sha}, nil
}

// sumFor finds name's hash in a sha256sum-style file.
func sumFor(data []byte, name string) string {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			sha := strings.ToLower(fields[0])
			if sha256Pattern.MatchString(sha) {
				return sha
			}
		}
	}

	return ""
}

// Newer reports whether version a is newer than b. Both are "vX.Y.Z";
// anything else is never newer.
func Newer(a, b string) bool {
	va, ok := parse(a)
	if !ok {
		return false
	}
	vb, ok := parse(b)
	if !ok {
		return false
	}

	for i := range va {
		if va[i] != vb[i] {
			return va[i] > vb[i]
		}
	}

	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int

	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return out, false
	}

	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}

	return out, true
}

// Download fetches an installer into dir, checking its size and SHA-256, and
// returns its path. progress may be nil.
//
// The hash comes from the same release, so this catches a corrupted or
// swapped download, not a compromised release. Once installers are signed,
// their signature should be checked here too.
func Download(ctx context.Context, c *fetch.Client, inst *Installer, dir string, progress fetch.Progress) (string, error) {
	if inst == nil {
		return "", fmt.Errorf("no installer")
	}
	if strings.ContainsAny(inst.Name, `/\:`) || !strings.HasSuffix(inst.Name, "-installer.exe") {
		return "", fmt.Errorf("unexpected installer name %q", inst.Name)
	}

	dest := filepath.Join(dir, inst.Name)
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		return "", err
	}

	if err := c.File(ctx, inst.URL, dest, inst.Size, inst.SHA256, progress); err != nil {
		return "", err
	}

	return dest, nil
}
