package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/diablo2org/launcher/internal/fetch"
)

func TestNewer(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want bool
	}{
		{"v1.2.0", "v1.1.9", true},
		{"v1.10.0", "v1.9.0", true},
		{"v2.0.0", "v1.99.99", true},
		{"v1.2.0", "v1.2.0", false},
		{"v1.1.0", "v1.2.0", false},
		{"v1.2.0-rc1", "v1.1.0", false},
		{"v1.2.0", "dev", false},
		{"latest", "v1.0.0", false},
	} {
		if got := Newer(tt.a, tt.b); got != tt.want {
			t.Errorf("Newer(%q, %q) = %v", tt.a, tt.b, got)
		}
	}
}

func TestCheck(t *testing.T) {
	body := `{"tag_name":"v0.2.0","html_url":"https://github.com/diablo2org/launcher/releases/tag/v0.2.0","draft":false,"prerelease":false}`
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := fetch.New([]string{u.Hostname()}, fetch.WithHTTPClient(srv.Client()))
	ctx := context.Background()

	r, err := Check(ctx, c, srv.URL, "v0.1.0")
	if err != nil || r == nil || r.Version != "v0.2.0" {
		t.Errorf("Check = %+v, %v", r, err)
	}

	if r, _ := Check(ctx, c, srv.URL, "v0.2.0"); r != nil {
		t.Errorf("same version reported as an update: %+v", r)
	}

	if r, _ := Check(ctx, c, srv.URL, "dev"); r != nil {
		t.Errorf("dev build reported an update: %+v", r)
	}

	body = `{"tag_name":"v9.9.9","html_url":"https://evil.example/installer.exe"}`
	if r, _ := Check(ctx, c, srv.URL, "v0.1.0"); r != nil {
		t.Errorf("non-GitHub release page accepted: %+v", r)
	}
}

// releaseServer serves a release whose installer is body, with the given
// digest, and a checksums file.
func releaseServer(t *testing.T, body, digest, sums string) (*fetch.Client, string) {
	t.Helper()

	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := srv.URL + "/releases/download/v0.2.0/"
		name := "launcher-" + runtime.GOARCH + "-installer.exe"
		switch r.URL.Path {
		case "/latest":
			fmt.Fprintf(w, `{"tag_name":"v0.2.0","html_url":"https://github.com/diablo2org/launcher/releases/tag/v0.2.0","assets":[
				{"name":%q,"size":%d,"browser_download_url":%q,"digest":%q},
				{"name":"SHA256SUMS.txt","size":%d,"browser_download_url":%q}]}`,
				name, len(body), base+name, digest, len(sums), base+"SHA256SUMS.txt")
		case "/releases/download/v0.2.0/" + name:
			w.Write([]byte(body))
		case "/releases/download/v0.2.0/SHA256SUMS.txt":
			w.Write([]byte(sums))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	oldLatest, oldPrefix := LatestURL, downloadPrefix
	UseSource(srv.URL)
	t.Cleanup(func() { LatestURL, downloadPrefix = oldLatest, oldPrefix })

	u, _ := url.Parse(srv.URL)
	return fetch.New([]string{u.Hostname()}, fetch.WithHTTPClient(srv.Client())), LatestURL
}

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func TestCheckFindsInstaller(t *testing.T) {
	ctx := context.Background()
	body := "new installer"
	name := "launcher-" + runtime.GOARCH + "-installer.exe"

	// GitHub's digest.
	c, latest := releaseServer(t, body, "sha256:"+sha(body), "")
	r, err := Check(ctx, c, latest, "v0.1.0")
	if err != nil || r == nil || r.Installer == nil {
		t.Fatalf("Check = %+v, %v", r, err)
	}
	if r.Installer.Name != name || r.Installer.SHA256 != sha(body) || r.Installer.Size != int64(len(body)) {
		t.Errorf("installer = %+v", r.Installer)
	}

	// No digest: the checksums file.
	c, latest = releaseServer(t, body, "", sha("other")+"  launcher-other.exe\n"+sha(body)+" *"+name+"\n")
	r, _ = Check(ctx, c, latest, "v0.1.0")
	if r == nil || r.Installer == nil || r.Installer.SHA256 != sha(body) {
		t.Errorf("from checksums: %+v", r)
	}

	// No hash anywhere: still an update, but only the release page.
	c, latest = releaseServer(t, body, "", "")
	r, _ = Check(ctx, c, latest, "v0.1.0")
	if r == nil || r.Installer != nil {
		t.Errorf("without a hash: %+v", r)
	}
}

func TestCheckIgnoresInstallersElsewhere(t *testing.T) {
	c, latest := releaseServer(t, "x", "sha256:"+sha("x"), "")
	downloadPrefix = "https://github.com/diablo2org/launcher/releases/download/"

	r, _ := Check(context.Background(), c, latest, "v0.1.0")
	if r == nil || r.Installer != nil {
		t.Errorf("installer from another place accepted: %+v", r)
	}
}

func TestDownload(t *testing.T) {
	ctx := context.Background()
	body := "new installer"
	c, latest := releaseServer(t, body, "sha256:"+sha(body), "")
	r, _ := Check(ctx, c, latest, "v0.1.0")
	dir := t.TempDir()

	path, err := Download(ctx, c, r.Installer, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != body || filepath.Dir(path) != dir {
		t.Errorf("downloaded %q to %s", data, path)
	}

	// Again, over the earlier download.
	if _, err := Download(ctx, c, r.Installer, dir, nil); err != nil {
		t.Errorf("second download: %v", err)
	}

	// A file that doesn't match its hash is refused.
	bad := *r.Installer
	bad.SHA256 = sha("something else")
	if _, err := Download(ctx, c, &bad, t.TempDir(), nil); err == nil {
		t.Error("mismatched installer accepted")
	}

	bad = *r.Installer
	bad.Name = `..\launcher-installer.exe`
	if _, err := Download(ctx, c, &bad, t.TempDir(), nil); err == nil {
		t.Error("installer name with a path accepted")
	}
}
