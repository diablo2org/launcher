package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
