package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diablo2org/launcher/internal/core"
	"github.com/diablo2org/launcher/internal/fetch"
	"github.com/diablo2org/launcher/internal/install"
	"github.com/diablo2org/launcher/internal/launch"
	"github.com/diablo2org/launcher/internal/store"
)

// sendHarness is a launcher with two servers, "slash" taking reports at a
// test server and "other" not.
type sendHarness struct {
	s       *SupportService
	data    string
	shown   string
	handler http.HandlerFunc
}

func newSendHarness(t *testing.T) *sendHarness {
	t.Helper()

	h := &sendHarness{data: t.TempDir()}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.handler(w, r) }))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)

	listing := t.TempDir()
	for _, id := range []string{"slash", "other"} {
		profile := map[string]any{
			"schema": 1, "id": id, "name": id, "version": 1,
			"hosts":    []string{u.Hostname()},
			"game":     map[string]any{"version": "1.13c", "baseArchives": "link"},
			"gateways": []any{map[string]any{"name": id, "host": "play." + id + ".test"}},
			"channels": []any{map[string]any{"id": "live", "name": "Live", "manifest": srv.URL + "/live.json"}},
		}
		if id == "slash" {
			profile["report"] = map[string]any{"url": srv.URL + "/report", "files": []string{"Mod_Debug*.log"}}
		}
		data, _ := json.Marshal(profile)
		os.WriteFile(filepath.Join(listing, id+".json"), data, 0o644)
	}

	base := t.TempDir()
	os.MkdirAll(install.ServerDir(base, "slash"), 0o755)
	os.MkdirAll(install.ServerDir(base, "other"), 0o755)
	os.WriteFile(filepath.Join(install.ServerDir(base, "slash"), "Mod_Debug.log"), []byte("mod log"), 0o644)
	os.WriteFile(filepath.Join(install.ServerDir(base, "other"), "D2260101.txt"), []byte("other crash"), 0o644)

	st, err := store.Open(h.data)
	if err != nil {
		t.Fatal(err)
	}
	state, _ := st.Load()
	state.BasePath = base
	state.Servers = map[string]*store.Server{"slash": {}, "other": {}}
	state.Favourites = []string{"other", "slash"}
	st.Save(state)

	m, err := core.New(st, core.DirListing(listing), launch.New(nil, nil), fetch.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}

	h.s = NewSupportService(st, m, Host{ShowFile: func(path string) error { h.shown = path; return nil }})
	return h
}

func TestSendReport(t *testing.T) {
	h := newSendHarness(t)

	var fields map[string]string
	var files map[string]string
	h.handler = func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/report" {
			http.Error(w, "wrong request", http.StatusBadRequest)
			return
		}
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fields = map[string]string{}
		for k, v := range r.MultipartForm.Value {
			fields[k] = v[0]
		}

		f, _, err := r.FormFile("report")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		data, _ := io.ReadAll(f)
		z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		files = map[string]string{}
		for _, zf := range z.File {
			rc, _ := zf.Open()
			body, _ := io.ReadAll(rc)
			rc.Close()
			files[zf.Name] = string(body)
		}

		w.Write([]byte(`{"id": "R-0412", "message": "Thanks"}`))
	}

	sr, err := h.s.ServerReport(t.Context(), "slash")
	if err != nil || sr == nil || sr.Host != "127.0.0.1" || sr.MaxBytes != 8<<20 {
		t.Fatalf("ServerReport = %+v, %v", sr, err)
	}

	res := h.s.SendReport(t.Context(), "slash", "  crashed in act 2  ", "player#1234")
	if res.Error != "" || res.ID != "R-0412" || res.Message != "Thanks" || res.SavedTo != "" {
		t.Fatalf("SendReport = %+v", res)
	}

	if fields["server"] != "slash" || fields["message"] != "crashed in act 2" || fields["contact"] != "player#1234" || fields["launcher"] != Version {
		t.Errorf("fields = %v", fields)
	}
	if files["game/slash/Mod_Debug.log"] != "mod log" {
		t.Errorf("server's own file missing: %v", keys(files))
	}
	// Nothing about the other server goes to this one.
	for name, body := range files {
		if strings.Contains(name, "other") || (name == "state.json" && strings.Contains(body, "other")) {
			t.Errorf("%s mentions the other server: %s", name, body)
		}
	}
	if _, ok := files["state.json"]; !ok {
		t.Error("state.json missing")
	}
}

func TestSendReportFailureSaves(t *testing.T) {
	h := newSendHarness(t)
	h.handler = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error": "Too many reports from you today."}`))
	}

	res := h.s.SendReport(t.Context(), "slash", "", "")
	if res.Error != "Too many reports from you today." || res.ID != "" {
		t.Fatalf("SendReport = %+v", res)
	}
	if filepath.Dir(res.SavedTo) != filepath.Join(h.data, "reports") {
		t.Fatalf("saved to %q", res.SavedTo)
	}
	if z, err := zip.OpenReader(res.SavedTo); err != nil {
		t.Errorf("saved report: %v", err)
	} else {
		z.Close()
	}

	if err := h.s.ShowSavedReport(res.SavedTo); err != nil || h.shown != res.SavedTo {
		t.Errorf("show = %v, shown %q", err, h.shown)
	}
	if err := h.s.ShowSavedReport(filepath.Join(h.data, "state.json")); err == nil {
		t.Error("showed a file outside the reports folder")
	}
}

func TestSendReportRefusesRedirect(t *testing.T) {
	h := newSendHarness(t)
	h.handler = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/report" {
			http.Redirect(w, r, "/elsewhere", http.StatusTemporaryRedirect)
			return
		}
		t.Errorf("followed the redirect to %s", r.URL.Path)
	}

	res := h.s.SendReport(t.Context(), "slash", "", "")
	if !strings.HasSuffix(res.Error, "/report: redirected, which uploads don't follow") || res.SavedTo == "" {
		t.Errorf("SendReport = %+v", res)
	}
}

func TestServerReportWithoutURL(t *testing.T) {
	h := newSendHarness(t)

	sr, err := h.s.ServerReport(t.Context(), "other")
	if err != nil || sr != nil {
		t.Errorf("ServerReport = %+v, %v", sr, err)
	}
	if res := h.s.SendReport(t.Context(), "other", "", ""); res.Error == "" {
		t.Error("sent to a server without a report URL")
	}
}

func TestSaveUnsentKeepsTheNewest(t *testing.T) {
	h := newSendHarness(t)
	dir := filepath.Join(h.data, "reports")
	os.MkdirAll(dir, 0o755)
	for i := 0; i < keptReports+3; i++ {
		f := filepath.Join(dir, "slash-report-old"+string(rune('a'+i))+".zip")
		os.WriteFile(f, []byte("old"), 0o644)
		past := time.Now().Add(-time.Duration(i+1) * time.Hour)
		os.Chtimes(f, past, past)
	}

	path, err := h.s.saveUnsent("slash", []byte("new"))
	if err != nil {
		t.Fatal(err)
	}
	all, _ := filepath.Glob(filepath.Join(dir, "*.zip"))
	if len(all) != keptReports {
		t.Errorf("%d reports kept", len(all))
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the new report was removed: %v", err)
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
