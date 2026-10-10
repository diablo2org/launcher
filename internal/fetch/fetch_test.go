package fetch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// server returns a test server plus a client allowed to reach it.
func server(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Client) {
	t.Helper()

	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)

	u, _ := url.Parse(srv.URL)
	return srv, New([]string{u.Hostname()}, WithHTTPClient(srv.Client()))
}

func TestAllowed(t *testing.T) {
	c := New([]string{"files.example.net"})

	for _, tt := range []struct {
		url string
		ok  bool
	}{
		{"https://files.example.net/a", true},
		{"https://FILES.example.net/a", true},
		{"http://files.example.net/a", false},
		{"https://evil.example/a", false},
		{"https://files.example.net.evil.example/a", false},
		{"https://user:pw@files.example.net/a", false},
		{"ftp://files.example.net/a", false},
	} {
		if err := c.Allowed(tt.url); (err == nil) != tt.ok {
			t.Errorf("Allowed(%q) = %v, want ok=%v", tt.url, err, tt.ok)
		}
	}

	if err := New([]string{"127.0.0.1"}).Allowed("http://127.0.0.1:8666/a"); err == nil {
		t.Error("plain http allowed on loopback")
	}
}

func TestDocumentChecksStatus(t *testing.T) {
	srv, c := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			http.Error(w, "<html>not found</html>", http.StatusNotFound)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	})

	if data, err := c.Document(context.Background(), srv.URL+"/doc"); err != nil || string(data) != `{"ok":true}` {
		t.Errorf("Document = %q, %v", data, err)
	}

	if _, err := c.Document(context.Background(), srv.URL+"/missing"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("an error page was returned as content: %v", err)
	}
}

func TestRedirectToOtherHostRefused(t *testing.T) {
	srv, c := server(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://evil.example/payload", http.StatusFound)
	})

	if _, err := c.Document(context.Background(), srv.URL+"/doc"); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("redirect followed: %v", err)
	}
}

// A CDN redirect carries a signed token in its query; errors leave it out.
func TestRedirectErrorHidesQuery(t *testing.T) {
	srv, c := server(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://cdn.example/asset?sig=SECRET&jwt=SECRET", http.StatusFound)
	})

	_, err := c.Document(context.Background(), srv.URL+"/doc")
	if err == nil || strings.Contains(err.Error(), "SECRET") || !strings.Contains(err.Error(), "https://cdn.example/asset") {
		t.Errorf("err = %v", err)
	}
	if !errors.Is(err, ErrNotAllowed) || Retryable(err) {
		t.Errorf("refused redirect: Is(ErrNotAllowed) = %v, Retryable = %v", errors.Is(err, ErrNotAllowed), Retryable(err))
	}
}

func TestBadURLHidesQuery(t *testing.T) {
	err := New([]string{"a.example"}).Allowed("https://a.example/%ZZ?sig=SECRET")
	if err == nil || strings.Contains(err.Error(), "SECRET") || !errors.Is(err, ErrNotAllowed) {
		t.Errorf("err = %v", err)
	}
}

func TestMalformedRedirectHidesQuery(t *testing.T) {
	srv, c := server(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://cdn.example/%ZZ?sig=SECRET")
		w.WriteHeader(http.StatusFound)
	})

	_, err := c.Document(context.Background(), srv.URL+"/doc")
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Errorf("err = %v", err)
	}
}

func TestRetryable(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		want bool
	}{
		{"busy", &StatusError{Code: http.StatusServiceUnavailable}, true},
		{"server error", &StatusError{Code: http.StatusInternalServerError}, true},
		{"rate limited", &StatusError{Code: http.StatusTooManyRequests}, true},
		{"missing", &StatusError{Code: http.StatusNotFound}, false},
		{"forbidden", &StatusError{Code: http.StatusForbidden}, false},
		{"cut short", fmt.Errorf("x: %w", io.ErrUnexpectedEOF), true},
		{"network", &net.OpError{Op: "dial", Err: errors.New("refused")}, true},
		{"refused URL", New(nil).Allowed("https://a.example/x"), false},
		{"cancelled", fmt.Errorf("x: %w", context.Canceled), false},
		{"hash", errors.New("x: SHA-256 0000 does not match the manifest"), false},
	} {
		if got := Retryable(tt.err); got != tt.want {
			t.Errorf("%s: Retryable(%v) = %v, want %v", tt.name, tt.err, got, tt.want)
		}
	}
}

func TestFileCutShortIsRetryable(t *testing.T) {
	body := []byte("the whole file")
	sum := sha256.Sum256(body)

	srv, c := server(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.Write(body[:4])
	})

	dest := filepath.Join(t.TempDir(), "f")
	err := c.File(context.Background(), srv.URL+"/f", dest, int64(len(body)), hex.EncodeToString(sum[:]), nil)
	if err == nil || !Retryable(err) {
		t.Errorf("cut-short download: err = %v, Retryable = %v", err, Retryable(err))
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Error("partial file left behind")
	}
}

func TestFile(t *testing.T) {
	body := []byte(strings.Repeat("d2", 5000))
	sum := sha256.Sum256(body)
	sha := hex.EncodeToString(sum[:])

	srv, c := server(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	})

	dir := t.TempDir()
	ctx := context.Background()

	var seen int64
	dest := filepath.Join(dir, "ok.dll")
	if err := c.File(ctx, srv.URL+"/f", dest, int64(len(body)), sha, func(n int64) { seen = n }); err != nil {
		t.Fatal(err)
	}
	if seen != int64(len(body)) {
		t.Errorf("progress ended at %d, want %d", seen, len(body))
	}

	for name, tt := range map[string]struct {
		size int64
		sha  string
	}{
		"wrong hash":  {int64(len(body)), strings.Repeat("0", 64)},
		"wrong size":  {int64(len(body)) - 1, sha},
		"bigger size": {int64(len(body)) + 1, sha},
	} {
		dest := filepath.Join(dir, strings.ReplaceAll(name, " ", "-"))
		if err := c.File(ctx, srv.URL+"/f", dest, tt.size, tt.sha, nil); err == nil {
			t.Errorf("%s: no error", name)
		}
		if _, err := os.Stat(dest); !os.IsNotExist(err) {
			t.Errorf("%s: bad download left on disk", name)
		}
	}

	// An existing file is never overwritten in place.
	if err := c.File(ctx, srv.URL+"/f", filepath.Join(dir, "ok.dll"), int64(len(body)), sha, nil); err == nil {
		t.Error("overwrote an existing file")
	}
}

func TestTrustCertificate(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	hosts := []string{u.Hostname()}

	// Without the certificate the self-signed server is refused.
	if _, err := New(hosts).Document(context.Background(), srv.URL); err == nil {
		t.Error("untrusted certificate accepted")
	}

	pemFile := filepath.Join(t.TempDir(), "dev.pem")
	os.WriteFile(pemFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o644)

	trust, err := TrustCertificate(pemFile)
	if err != nil {
		t.Fatal(err)
	}

	if data, err := New(hosts, trust).Document(context.Background(), srv.URL); err != nil || string(data) != "ok" {
		t.Errorf("with the certificate: %q, %v", data, err)
	}
}

func TestHashFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x")
	os.WriteFile(path, []byte("abc"), 0o644)

	n, sha, err := HashFile(path)
	if err != nil || n != 3 || sha != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Errorf("HashFile = %d, %s, %v", n, sha, err)
	}
}

func TestResume(t *testing.T) {
	body := []byte(strings.Repeat("0123456789", 1000))
	sum := sha256.Sum256(body)
	sha := hex.EncodeToString(sum[:])
	size := int64(len(body))

	// mode picks how the server behaves.
	var mode string
	var ranges []string
	srv, c := server(t, func(w http.ResponseWriter, r *http.Request) {
		ranges = append(ranges, r.Header.Get("Range"))
		switch mode {
		case "no ranges":
			w.Write(body)
		case "cut":
			// Says the whole rest is coming, then stops at 6000 bytes.
			w.Header().Set("Content-Length", fmt.Sprint(size))
			w.Write(body[:6000])
		case "wrong part":
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", size-1, size))
			w.WriteHeader(http.StatusPartialContent)
			w.Write(body)
		default:
			http.ServeContent(w, r, "f", time.Time{}, bytes.NewReader(body))
		}
	})
	ctx := context.Background()
	dir := t.TempDir()

	run := func(t *testing.T, name, server string, partial []byte) (string, []string, int64) {
		t.Helper()
		mode, ranges = server, nil
		dest := filepath.Join(dir, name)
		if partial != nil {
			os.WriteFile(dest, partial, 0o644)
		}
		var first int64 = -1
		err := c.Resume(ctx, srv.URL+"/f", dest, size, sha, func(n int64) {
			if first < 0 {
				first = n
			}
		})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if data, _ := os.ReadFile(dest); !bytes.Equal(data, body) {
			t.Errorf("%s: file is wrong", name)
		}
		return dest, ranges, first
	}

	// Carries on from the partial, asking for only the rest, and counts the
	// partial in the progress.
	if _, r, first := run(t, "half", "", body[:4000]); strings.Join(r, ",") != "bytes=4000-" || first <= 4000 {
		t.Errorf("half: ranges %q, first progress %d", r, first)
	}

	// A server that ignores ranges sends it all; that's used instead.
	if _, r, _ := run(t, "ignored", "no ranges", body[:4000]); len(r) != 1 {
		t.Errorf("ignored: ranges %q", r)
	}

	// A partial of some other version of the file fails the hash, and the
	// download starts once more from nothing.
	other := append([]byte("XXXX"), body[4:4000]...)
	if _, r, _ := run(t, "stale", "", other); strings.Join(r, ",") != "bytes=4000-," {
		t.Errorf("stale: ranges %q", r)
	}

	// A partial longer than the file, or a reply for the wrong part, is
	// thrown away.
	if _, r, _ := run(t, "long", "", append(append([]byte{}, body...), "extra"...)); strings.Join(r, ",") != "" {
		t.Errorf("long: ranges %q", r)
	}

	// Already complete: nothing to fetch.
	if _, r, _ := run(t, "complete", "", body); len(r) != 0 {
		t.Errorf("complete: ranges %q", r)
	}

	// A download cut short keeps what arrived, and the next try carries on.
	mode = "cut"
	dest := filepath.Join(dir, "cut")
	if err := c.Resume(ctx, srv.URL+"/f", dest, size, sha, nil); err == nil || !Retryable(err) {
		t.Fatalf("cut: err = %v, Retryable = %v", err, Retryable(err))
	}
	if info, err := os.Stat(dest); err != nil || info.Size() != 6000 {
		t.Fatalf("cut: partial %v, %v", info, err)
	}
	if _, r, _ := run(t, "cut", "", nil); strings.Join(r, ",") != "bytes=6000-" {
		t.Errorf("cut, resumed: ranges %q", r)
	}

	// Wrong part sent for a resumed download: start again in full.
	mode = "wrong part"
	dest = filepath.Join(dir, "wrong")
	os.WriteFile(dest, body[:4000], 0o644)
	if err := c.Resume(ctx, srv.URL+"/f", dest, size, sha, nil); err == nil {
		t.Error("wrong part: no error")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("wrong part: bad download left on disk")
	}
}

// A partial at least as long as the server's file gets a 416 for its range,
// so it isn't of that file: it goes, and the whole file is asked for.
func TestResumeRangeNotSatisfiable(t *testing.T) {
	body := []byte("the file, now")
	var ranges []string
	srv, c := server(t, func(w http.ResponseWriter, r *http.Request) {
		ranges = append(ranges, r.Header.Get("Range"))
		http.ServeContent(w, r, "f", time.Time{}, bytes.NewReader(body))
	})

	// The manifest says the file is longer than the server's copy, as when
	// the server hasn't finished uploading a new version.
	want := append(append([]byte{}, body...), " and more"...)
	sum := sha256.Sum256(want)
	dest := filepath.Join(t.TempDir(), "f")
	os.WriteFile(dest, want[:len(body)+2], 0o644)

	if err := c.Resume(context.Background(), srv.URL+"/f", dest, int64(len(want)), hex.EncodeToString(sum[:]), nil); err == nil {
		t.Fatal("download of a file the server doesn't have succeeded")
	}
	if strings.Join(ranges, ",") != fmt.Sprintf("bytes=%d-,", len(body)+2) {
		t.Errorf("ranges = %q, want the range then the whole file", ranges)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("partial of the wrong file kept")
	}
}

// A link at the partial's name is replaced, never written through.
func TestResumeReplacesLink(t *testing.T) {
	body := []byte("the real file")
	sum := sha256.Sum256(body)
	srv, c := server(t, func(w http.ResponseWriter, r *http.Request) { w.Write(body) })

	dir := t.TempDir()
	target := filepath.Join(dir, "retail.mpq")
	os.WriteFile(target, []byte("retail"), 0o644)
	dest := filepath.Join(dir, "f.launcher-download")
	if err := os.Symlink(target, dest); err != nil {
		t.Skipf("can't make a symlink here: %v", err)
	}

	if err := c.Resume(context.Background(), srv.URL+"/f", dest, int64(len(body)), hex.EncodeToString(sum[:]), nil); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(target); string(data) != "retail" {
		t.Errorf("wrote through the link: %q", data)
	}
	if info, _ := os.Lstat(dest); !info.Mode().IsRegular() {
		t.Error("link still there")
	}
}

func TestPost(t *testing.T) {
	srv, c := server(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.URL.Path == "/moved":
			http.Redirect(w, r, "/report", http.StatusTemporaryRedirect)
		case r.Method != http.MethodPost || r.Header.Get("Content-Type") != "text/plain" || string(body) != "hello":
			http.Error(w, "bad request", http.StatusBadRequest)
		case r.URL.Path == "/full":
			http.Error(w, `{"error": "full"}`, http.StatusRequestEntityTooLarge)
		default:
			w.Write([]byte(`{"id": "1"}`))
		}
	})

	reply, err := c.Post(context.Background(), srv.URL+"/report", "text/plain", []byte("hello"))
	if err != nil || string(reply) != `{"id": "1"}` {
		t.Errorf("Post = %q, %v", reply, err)
	}

	// A refusal still returns what the server said.
	reply, err = c.Post(context.Background(), srv.URL+"/full", "text/plain", []byte("hello"))
	var status *StatusError
	if !errors.As(err, &status) || status.Code != http.StatusRequestEntityTooLarge || !strings.Contains(string(reply), "full") {
		t.Errorf("Post = %q, %v", reply, err)
	}

	// A redirect isn't followed, even to the same host.
	if _, err := c.Post(context.Background(), srv.URL+"/moved", "text/plain", []byte("hello")); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("redirect: %v", err)
	}

	if _, err := c.Post(context.Background(), "https://evil.example/report", "text/plain", []byte("hello")); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("other host: %v", err)
	}
}
