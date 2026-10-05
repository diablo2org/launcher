package app

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/diablo2org/launcher/internal/core"
	"github.com/diablo2org/launcher/internal/logs"
	"github.com/diablo2org/launcher/internal/report"
	"github.com/diablo2org/launcher/internal/store"
)

// SupportService is the frontend's way to logs and bug reports.
type SupportService struct {
	st   *store.Store
	m    *core.Manager
	host Host
}

// NewSupportService gives the frontend the logs in st's data folder.
func NewSupportService(st *store.Store, m *core.Manager, host Host) *SupportService {
	return &SupportService{st: st, m: m, host: host}
}

// ReportContents lists what a bug report would hold, for the player to see
// before making one.
func (s *SupportService) ReportContents() []report.Item {
	return report.Collect(s.input())
}

// SaveReport asks where to save a bug report, writes it there and shows it.
// It returns the file, or "" if the player cancelled.
func (s *SupportService) SaveReport() (string, error) {
	if s.host.SaveFile == nil {
		return "", fmt.Errorf("saving files isn't available")
	}

	name := "launcher-report-" + time.Now().Format("2006-01-02-1504") + ".zip"
	path, err := s.host.SaveFile("Save bug report", name)
	if err != nil || path == "" {
		return "", err
	}
	if !strings.EqualFold(filepath.Ext(path), ".zip") {
		path += ".zip"
	}

	in := s.input()
	var buf bytes.Buffer
	if err := report.Write(&buf, report.Collect(in), in.Home); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return "", err
	}
	slog.Info("bug report saved", "bytes", buf.Len())

	if s.host.ShowFile != nil {
		s.host.ShowFile(path)
	}

	return path, nil
}

// OpenLogs shows the logs folder.
func (s *SupportService) OpenLogs() error {
	if s.host.OpenFolder == nil {
		return fmt.Errorf("opening folders isn't available")
	}

	return s.host.OpenFolder(logs.Dir(s.st.Dir()))
}

// LogFrontendError records an error the page didn't handle.
func (s *SupportService) LogFrontendError(message string) {
	if len(message) > 4000 {
		message = message[:4000]
	}
	slog.Error("frontend", "err", message)
}

func (s *SupportService) input() report.Input {
	home, _ := os.UserHomeDir()

	in := report.Input{DataDir: s.st.Dir(), Base: s.m.Base(), Home: home}
	if state, err := s.st.Load(); err == nil {
		for id := range state.Servers {
			in.Servers = append(in.Servers, id)
		}
		sort.Strings(in.Servers)
	}

	var about strings.Builder
	fmt.Fprintf(&about, "Launcher %s\n", Version)
	fmt.Fprintf(&about, "System   %s %s, %s\n", runtime.GOOS, osVersion(), runtime.GOARCH)
	fmt.Fprintf(&about, "Built    %s\n", runtime.Version())
	fmt.Fprintf(&about, "Report   %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&about, "\nDiablo II folder: %s\n", in.Base)
	if in.Base != "" {
		r, warning, err := s.m.CheckBase()
		switch {
		case err != nil:
			fmt.Fprintf(&about, "  check failed: %v\n", err)
		default:
			names := make([]string, 0, len(r.Present))
			for n := range r.Present {
				names = append(names, n)
			}
			sort.Strings(names)
			fmt.Fprintf(&about, "  archives: %s\n", strings.Join(names, ", "))
			if len(r.Missing) > 0 {
				fmt.Fprintf(&about, "  missing: %s\n", strings.Join(r.Missing, ", "))
			}
			if len(r.MissingOptional) > 0 {
				fmt.Fprintf(&about, "  missing optional: %s\n", strings.Join(r.MissingOptional, ", "))
			}
			if warning != "" {
				fmt.Fprintf(&about, "  warning: %s\n", warning)
			}
		}
	}
	in.About = about.String()

	return in
}
