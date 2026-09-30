package updates

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/d2org/launcher/internal/fetch"
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
	body := `{"tag_name":"v0.2.0","html_url":"https://github.com/d2org/launcher/releases/tag/v0.2.0","draft":false,"prerelease":false}`
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
