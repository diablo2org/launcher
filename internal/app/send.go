package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/diablo2org/launcher/internal/report"
	"github.com/diablo2org/launcher/internal/store"
)

// ServerReport is a bug report that goes to a server rather than being saved.
type ServerReport struct {
	// Host is where it is sent, for the player to see.
	Host string `json:"host"`
	// MaxBytes is the most the server accepts; the files are trimmed to fit.
	MaxBytes int           `json:"maxBytes"`
	Items    []report.Item `json:"items"`
}

// ServerReport lists what a report sent to server id would hold. It returns
// nil when the server takes no reports, so the player saves one instead.
func (s *SupportService) ServerReport(ctx context.Context, id string) (*ServerReport, error) {
	p, err := s.m.Profile(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.Report.URL == "" {
		return nil, nil
	}

	u, err := url.Parse(p.Report.URL)
	if err != nil {
		return nil, err
	}

	return &ServerReport{Host: u.Hostname(), MaxBytes: p.Report.Limit(), Items: report.Collect(s.input(id))}, nil
}

// SendResult is how sending a report went.
type SendResult struct {
	// ID is the server's reference for the report, to quote when asking
	// about it.
	ID string `json:"id"`
	// Message is anything else the server said.
	Message string `json:"message"`
	// Error says why the report wasn't sent.
	Error string `json:"error"`
	// SavedTo is where the report was saved instead, so the player can send
	// it by hand.
	SavedTo string `json:"savedTo"`
}

// Limits on what the player types, so a server knows what to expect.
const (
	maxReportMessage = 2000
	maxReportContact = 100
)

// formRoom is what the upload's other fields may take beside the zip, which
// is fitted to the server's limit less this.
const formRoom = 16 << 10

// SendReport sends a bug report to server id, with what the player wrote. If
// it can't be sent, it is saved in the launcher's reports folder instead.
func (s *SupportService) SendReport(ctx context.Context, id, message, contact string) SendResult {
	p, err := s.m.Profile(ctx, id)
	if err != nil {
		return SendResult{Error: err.Error()}
	}

	in := s.input(id)
	zip, kept, err := report.Fit(report.Collect(in), in.Home, p.Report.Limit()-formRoom)
	if err != nil {
		return SendResult{Error: err.Error()}
	}

	body, contentType, err := reportForm(zip, id, clip(message, maxReportMessage), clip(contact, maxReportContact))
	if err != nil {
		return SendResult{Error: err.Error()}
	}

	sendCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	reply, err := s.m.PostReport(sendCtx, id, contentType, body)
	var answer struct {
		ID      string `json:"id"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	json.Unmarshal(reply, &answer)

	if err == nil {
		slog.Info("bug report sent", "server", id, "bytes", len(zip), "files", len(kept), "ref", answer.ID)
		return SendResult{ID: clip(answer.ID, 64), Message: clip(answer.Message, 500)}
	}

	slog.Error("bug report not sent", "server", id, "err", err)
	res := SendResult{Error: err.Error()}
	if answer.Error != "" {
		res.Error = clip(answer.Error, 500)
	}
	if path, saveErr := s.saveUnsent(id, zip); saveErr == nil {
		res.SavedTo = path
	} else {
		slog.Error("bug report not saved", "err", saveErr)
	}

	return res
}

// ShowSavedReport shows a report that SendReport saved instead of sending.
func (s *SupportService) ShowSavedReport(path string) error {
	if s.host.ShowFile == nil {
		return fmt.Errorf("showing files isn't available")
	}
	if !strings.EqualFold(filepath.Dir(path), s.reportsDir()) {
		return fmt.Errorf("not a saved report")
	}

	return s.host.ShowFile(path)
}

// keptReports is how many unsent reports the reports folder keeps.
const keptReports = 10

func (s *SupportService) reportsDir() string {
	return filepath.Join(s.st.Dir(), "reports")
}

// saveUnsent keeps a report that couldn't be sent, dropping the oldest beyond
// keptReports.
func (s *SupportService) saveUnsent(id string, zip []byte) (string, error) {
	dir := s.reportsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	path := filepath.Join(dir, id+"-report-"+time.Now().Format("2006-01-02-150405")+".zip")
	if err := writeFileAtomic(path, zip); err != nil {
		return "", err
	}

	if all, err := filepath.Glob(filepath.Join(dir, "*-report-*.zip")); err == nil && len(all) > keptReports {
		sort.Slice(all, func(i, j int) bool { return modTime(all[i]).Before(modTime(all[j])) })
		for _, f := range all[:len(all)-keptReports] {
			os.Remove(f)
		}
	}

	return path, nil
}

func modTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}

	return info.ModTime()
}

// reportForm is the upload SPEC section 3.4 describes.
func reportForm(zip []byte, id, message, contact string) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	for _, field := range [][2]string{{"server", id}, {"launcher", Version}, {"message", message}, {"contact", contact}} {
		if err := w.WriteField(field[0], field[1]); err != nil {
			return nil, "", err
		}
	}

	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="report"; filename="report.zip"`)
	h.Set("Content-Type", "application/zip")
	part, err := w.CreatePart(h)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(zip); err != nil {
		return nil, "", err
	}

	if err := w.Close(); err != nil {
		return nil, "", err
	}

	return buf.Bytes(), w.FormDataContentType(), nil
}

// scopedState is state.json with only server id's entry, so a report sent to
// one server doesn't tell it which others the player uses.
func scopedState(state *store.State, id string) []byte {
	cut := *state
	cut.Servers = map[string]*store.Server{}
	if srv := state.Servers[id]; srv != nil {
		cut.Servers[id] = srv
	}
	cut.Favourites = nil
	for _, f := range state.Favourites {
		if f == id {
			cut.Favourites = []string{id}
		}
	}

	data, err := json.MarshalIndent(&cut, "", "  ")
	if err != nil {
		return []byte(fmt.Sprintf("couldn't write the settings: %v\n", err))
	}

	return data
}

// clip trims s and cuts it to at most n runes.
func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		r = r[:n]
	}

	return string(r)
}
