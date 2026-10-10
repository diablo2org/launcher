// Package fetch is the only code that talks to the network. Every request is
// HTTPS to a host the server's profile allows, including any redirect, and a
// response is only used after its status and size have been checked.
package fetch

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// GitHubAssetHosts are where github.com redirects a release asset download.
// Redirects are checked against the allowed hosts, so files published as
// release assets need these allowed too.
var GitHubAssetHosts = []string{"release-assets.githubusercontent.com", "objects.githubusercontent.com"}

// MaxDocument is the largest profile, manifest or listing entry accepted.
const MaxDocument = 4 << 20

// Client downloads from a fixed set of hosts.
type Client struct {
	hosts map[string]bool
	http  *http.Client
}

// Option changes how a Client behaves.
type Option func(*Client)

// TrustCertificate adds a PEM certificate to the trusted roots, alongside the
// system's. Only for the local development server (cmd/devserve).
func TrustCertificate(pemFile string) (Option, error) {
	data, err := os.ReadFile(pemFile)
	if err != nil {
		return nil, err
	}

	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("%s: no certificate found", pemFile)
	}

	return func(c *Client) {
		if t, ok := c.http.Transport.(*http.Transport); ok {
			t.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
		}
	}, nil
}

// WithHTTPClient replaces the underlying HTTP client, for tests.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// New returns a client that only fetches from hosts.
func New(hosts []string, opts ...Option) *Client {
	c := &Client{hosts: make(map[string]bool, len(hosts))}
	for _, h := range hosts {
		c.hosts[strings.ToLower(h)] = true
	}

	c.http = &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 15 * time.Second}).DialContext,
			TLSHandshakeTimeout:   15 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			MaxIdleConnsPerHost:   4,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	// Redirects are checked against the same rules as the first request, so
	// an allowed host can't bounce a download somewhere else.
	c.http.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return c.Allowed(req.URL.String())
	}

	return c
}

// Allowed returns an error unless rawURL may be fetched by this client.
func (c *Client) Allowed(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("bad URL %q: %w", rawURL, err)
	}

	if u.User != nil {
		return fmt.Errorf("%s: URLs must not carry credentials", u.Redacted())
	}

	host := strings.ToLower(u.Hostname())

	if u.Scheme != "https" {
		return fmt.Errorf("%s: only https is allowed", rawURL)
	}

	if !c.hosts[host] {
		return fmt.Errorf("%s: host %s is not allowed for this server", rawURL, host)
	}

	return nil
}

func (c *Client) get(ctx context.Context, rawURL string) (*http.Response, error) {
	if err := c.Allowed(rawURL); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "diablo2org-launcher")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s: %s", rawURL, resp.Status)
	}

	return resp, nil
}

// Document downloads a small document such as a profile or manifest.
func (c *Client) Document(ctx context.Context, rawURL string) ([]byte, error) {
	resp, err := c.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxDocument+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", rawURL, err)
	}

	if len(data) > MaxDocument {
		return nil, fmt.Errorf("%s: larger than %d bytes", rawURL, MaxDocument)
	}

	return data, nil
}

// Progress is called as a file downloads with the bytes written so far.
type Progress func(written int64)

// File downloads rawURL to dest, which must not exist yet, and checks it is
// exactly size bytes with the given SHA-256. On any failure dest is removed,
// so a partial or wrong file is never left behind.
func (c *Client) File(ctx context.Context, rawURL, dest string, size int64, sha string, progress Progress) (err error) {
	resp, err := c.get(ctx, rawURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.ContentLength >= 0 && resp.ContentLength != size {
		return fmt.Errorf("%s: server reports %d bytes, manifest says %d", rawURL, resp.ContentLength, size)
	}

	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(dest)
		}
	}()

	h := sha256.New()
	w := &counter{w: io.MultiWriter(f, h), progress: progress}

	// Read one byte past the expected size so an oversized body is caught
	// rather than silently truncated.
	n, err := io.Copy(w, io.LimitReader(resp.Body, size+1))
	if err != nil {
		return fmt.Errorf("%s: %w", rawURL, err)
	}

	if n != size {
		return fmt.Errorf("%s: got %d bytes, manifest says %d", rawURL, n, size)
	}

	if got := hex.EncodeToString(h.Sum(nil)); got != sha {
		return fmt.Errorf("%s: SHA-256 %s does not match the manifest", rawURL, got)
	}

	return nil
}

type counter struct {
	w        io.Writer
	n        int64
	progress Progress
}

func (c *counter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	if c.progress != nil {
		c.progress(c.n)
	}
	return n, err
}

// HashFile returns the size and SHA-256 of a file on disk.
func HashFile(path string) (int64, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()

	var h hash.Hash = sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return 0, "", err
	}

	return n, hex.EncodeToString(h.Sum(nil)), nil
}
