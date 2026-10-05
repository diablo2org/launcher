package app

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"

	"github.com/diablo2org/launcher/internal/core"
	"github.com/diablo2org/launcher/internal/fetch"
	"github.com/diablo2org/launcher/internal/install"
	"github.com/diablo2org/launcher/internal/logs"
	"github.com/diablo2org/launcher/internal/sync"
	"github.com/diablo2org/launcher/internal/updates"
)

// Host is what the service needs from the app window.
type Host struct {
	// Emit sends an event to the frontend.
	Emit func(name string, data any)
	// PickFolder asks the player for a folder; "" means cancelled.
	PickFolder func(title string) (string, error)
	// Updates fetches release information; nil turns update checks off.
	Updates *fetch.Client
	// OpenFolder shows a folder in the file manager.
	OpenFolder func(path string) error
	// SaveFile asks where to save a file; "" means cancelled.
	SaveFile func(title, name string) (string, error)
	// ShowFile shows a file, selected, in the file manager.
	ShowFile func(path string) error
}

// ServerService is how the frontend drives the launcher.
type ServerService struct {
	m    *core.Manager
	host Host
}

// NewServerService wraps a manager for the frontend.
func NewServerService(m *core.Manager, host Host) *ServerService {
	return &ServerService{m: m, host: host}
}

// Overview is the launcher's starting state.
type Overview struct {
	Base       string            `json:"base"`
	Report     install.Report    `json:"report"`
	Warning    string            `json:"warning"`
	BaseError  string            `json:"baseError"`
	Servers    []core.ServerInfo `json:"servers"`
	Favourites []string          `json:"favourites"`
	Error      string            `json:"error"`
}

// Overview loads the base install check and every server. On the first run it
// also brings across the old SlashDiablo launcher's settings, then finds the
// Diablo II folder from the registry if it still isn't known.
func (s *ServerService) Overview(ctx context.Context) Overview {
	servers, err := s.m.Servers(ctx)

	s.m.ImportLegacy(ctx, core.LegacyConfigPath())
	s.m.DetectBase()

	o := Overview{Base: s.m.Base(), Favourites: s.m.Favourites(), Servers: []core.ServerInfo{}}
	if err != nil {
		slog.Warn("servers", "err", err)
		o.Error = err.Error()
	}
	if servers != nil {
		o.Servers = servers
	}

	if o.Base != "" {
		r, warning, err := s.m.CheckBase()
		o.Report, o.Warning = r, warning
		if err != nil {
			slog.Warn("diablo ii folder", "err", err)
			o.BaseError = err.Error()
		} else if !r.OK() {
			slog.Warn("diablo ii folder", "missing", r.Missing)
		}
	}

	return o
}

// AddServer adds a server that isn't listed, by the URL of its profile.
func (s *ServerService) AddServer(ctx context.Context, profileURL string) (string, error) {
	p, err := s.m.AddServer(ctx, profileURL)
	if err != nil {
		slog.Error("add server", "url", logs.RedactURLs(profileURL), "err", logs.RedactURLs(err.Error()))
		return "", err
	}
	slog.Info("server added", "server", p.ID, "url", logs.RedactURLs(profileURL))

	return p.ID, nil
}

// RemoveServer forgets a server added by URL.
func (s *ServerService) RemoveServer(id string) error {
	return logged("remove server", id, s.m.RemoveServer(id))
}

// ChooseBase lets the player pick their Diablo II folder.
func (s *ServerService) ChooseBase(ctx context.Context) (Overview, error) {
	dir, err := s.host.PickFolder("Choose your Diablo II folder")
	if err != nil || dir == "" {
		return s.Overview(ctx), err
	}

	if _, err := s.m.SetBase(dir); err != nil {
		return s.Overview(ctx), logged("choose diablo ii folder", "", err)
	}
	slog.Info("diablo ii folder chosen", "dir", dir)

	return s.Overview(ctx), nil
}

// SetFavourite pins or unpins a server.
func (s *ServerService) SetFavourite(id string, on bool) error {
	return s.m.SetFavourite(id, on)
}

// SetFavouriteOrder saves the pinned servers' order after the player drags
// them in the rail.
func (s *ServerService) SetFavouriteOrder(ids []string) error {
	return s.m.SetFavouriteOrder(ids)
}

// Status reports whether a server needs installing or updating.
func (s *ServerService) Status(ctx context.Context, id string) core.Status {
	return s.m.Status(ctx, id)
}

// UpdateProgress is sent as the "update:progress" event while updating.
type UpdateProgress struct {
	Server   string        `json:"server"`
	Progress sync.Progress `json:"progress"`
}

// UpdateResult tells the frontend how an update went.
type UpdateResult struct {
	OK bool `json:"ok"`
	// NeedsCopy asks the player whether to copy the game archives, using
	// CopyBytes of disk space.
	NeedsCopy bool   `json:"needsCopy"`
	CopyBytes int64  `json:"copyBytes"`
	Error     string `json:"error"`
}

// Update installs or updates a server.
func (s *ServerService) Update(ctx context.Context, id string, allowCopy bool) UpdateResult {
	slog.Info("update", "server", id, "allowCopy", allowCopy)
	err := s.m.Update(ctx, id, allowCopy, func(p sync.Progress) {
		s.host.Emit("update:progress", UpdateProgress{Server: id, Progress: p})
	})

	var needsCopy core.ErrNeedsCopy
	switch {
	case err == nil:
		slog.Info("update done", "server", id)
		return UpdateResult{OK: true}
	case errors.As(err, &needsCopy):
		slog.Info("update needs archives copied", "server", id, "bytes", needsCopy.Bytes)
		return UpdateResult{NeedsCopy: true, CopyBytes: needsCopy.Bytes}
	default:
		logged("update", id, err)
		return UpdateResult{Error: err.Error()}
	}
}

// Play starts the server.
func (s *ServerService) Play(ctx context.Context, id string) error {
	slog.Info("play", "server", id)
	return logged("play", id, s.m.Play(ctx, id))
}

// Choices returns the player's picks for a server.
func (s *ServerService) Choices(ctx context.Context, id string) (core.Choices, error) {
	return s.m.Choices(ctx, id)
}

// SetChannel switches release channel.
func (s *ServerService) SetChannel(ctx context.Context, id, channel string) error {
	return logged("set channel "+channel, id, s.m.SetChannel(ctx, id, channel))
}

// SetComponent turns a component on at a version, or off with "".
func (s *ServerService) SetComponent(ctx context.Context, id, component, version string) error {
	return logged("set component "+component+" "+version, id, s.m.SetComponent(ctx, id, component, version))
}

// SetInstances sets the number of boxes and d2gl profile splitting.
func (s *ServerService) SetInstances(ctx context.Context, id string, n int, splitD2GL bool) error {
	return logged("set instances", id, s.m.SetInstances(ctx, id, n, splitD2GL))
}

// OpenServerFolder shows a server's folder, for dropping in custom files.
func (s *ServerService) OpenServerFolder(id string) error {
	dir, err := s.m.ServerFolder(id)
	if err != nil {
		return err
	}
	if s.host.OpenFolder == nil {
		return errors.New("opening folders isn't available")
	}

	return s.host.OpenFolder(dir)
}

// SetD2GLResolutions sets the main and loader box window sizes.
func (s *ServerService) SetD2GLResolutions(id, main, loader string) error {
	return s.m.SetD2GLResolutions(id, main, loader)
}

// Settings returns a server's declared settings with their values.
func (s *ServerService) Settings(ctx context.Context, id string) ([]core.SettingValue, error) {
	return s.m.Settings(ctx, id)
}

// SetSetting changes one setting.
func (s *ServerService) SetSetting(ctx context.Context, id, setting string, value any) error {
	return logged("set setting "+setting, id, s.m.SetSetting(ctx, id, setting, value))
}

// News returns a server's latest posts.
func (s *ServerService) News(ctx context.Context, id string) ([]core.NewsItem, error) {
	return s.m.News(ctx, id)
}

// Ladder returns a server's ladder boards.
func (s *ServerService) Ladder(ctx context.Context, id string) (*core.Ladder, error) {
	return s.m.Ladder(ctx, id)
}

// LaunchDelay is the wait between boxes, in milliseconds.
func (s *ServerService) LaunchDelay() int {
	return s.m.LaunchDelay()
}

// SetLaunchDelay changes the wait between boxes.
func (s *ServerService) SetLaunchDelay(ms int) error {
	return s.m.SetLaunchDelay(ms)
}

// Version is the launcher's version, shown in the corner.
func (s *ServerService) Version() string {
	return Version
}

// CheckForUpdate reports a newer launcher release, if there is one. Errors
// are swallowed: being offline shouldn't show as a problem.
func (s *ServerService) CheckForUpdate(ctx context.Context) *updates.Release {
	if s.host.Updates == nil {
		return nil
	}

	r, err := updates.Check(ctx, s.host.Updates, updates.LatestURL, Version)
	if err != nil {
		return nil
	}

	return r
}

// logged records a failed action in the log, so it's there for a bug report,
// and returns the error unchanged. URLs in the error lose their queries,
// which can hold tokens.
func logged(action, server string, err error) error {
	if err != nil {
		slog.Error(action, "server", server, "err", logs.RedactURLs(err.Error()))
	}

	return err
}

// Branding is a server's images as data URLs, ready for the page.
type Branding struct {
	Logo       string `json:"logo"`
	Background string `json:"background"`
}

// Branding fetches a server's images. Missing or bad images are left empty;
// the tab falls back to its plain style.
func (s *ServerService) Branding(ctx context.Context, id string) Branding {
	var b Branding

	if data, err := s.m.Asset(ctx, id, "logo"); err == nil {
		b.Logo = dataURL(data)
	}
	if data, err := s.m.Asset(ctx, id, "background"); err == nil {
		b.Background = dataURL(data)
	}

	return b
}

// dataURL only passes through the image types the spec allows, going by the
// content rather than anything the server claims.
func dataURL(data []byte) string {
	switch t := http.DetectContentType(data); t {
	case "image/png", "image/jpeg", "image/webp":
		return "data:" + t + ";base64," + base64.StdEncoding.EncodeToString(data)
	}

	return ""
}
