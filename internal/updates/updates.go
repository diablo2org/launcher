// Package updates checks whether a newer launcher has been released. It only
// reports; the player downloads the new installer from the release page.
package updates

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/diablo2org/launcher/internal/fetch"
)

// Release is a newer launcher release.
type Release struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// LatestURL is the GitHub API for the newest non-prerelease release.
const LatestURL = "https://api.github.com/repos/diablo2org/launcher/releases/latest"

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
		TagName    string `json:"tag_name"`
		HTMLURL    string `json:"html_url"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}

	if r.Draft || r.Prerelease || !strings.HasPrefix(r.HTMLURL, "https://github.com/") || !Newer(r.TagName, current) {
		return nil, nil
	}

	return &Release{Version: r.TagName, URL: r.HTMLURL}, nil
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
