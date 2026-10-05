package app

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/diablo2org/launcher/internal/core"
	"github.com/diablo2org/launcher/internal/fetch"
	"github.com/diablo2org/launcher/internal/install"
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
	// UpdatesDir is the launcher's own folder for downloaded installers.
	UpdatesDir string
	// RunInstaller starts a downloaded launcher installer.
	RunInstaller func(path string) error
	// Quit closes the launcher.
	Quit func()
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
		o.Error = err.Error()
	}
	if servers != nil {
		o.Servers = servers
	}

	if o.Base != "" {
		r, warning, err := s.m.CheckBase()
		o.Report, o.Warning = r, warning
		if err != nil {
			o.BaseError = err.Error()
		}
	}

	return o
}

// AddServer adds a server that isn't listed, by the URL of its profile.
func (s *ServerService) AddServer(ctx context.Context, profileURL string) (string, error) {
	p, err := s.m.AddServer(ctx, profileURL)
	if err != nil {
		return "", err
	}

	return p.ID, nil
}

// RemoveServer forgets a server added by URL.
func (s *ServerService) RemoveServer(id string) error {
	return s.m.RemoveServer(id)
}

// ChooseBase lets the player pick their Diablo II folder.
func (s *ServerService) ChooseBase(ctx context.Context) (Overview, error) {
	dir, err := s.host.PickFolder("Choose your Diablo II folder")
	if err != nil || dir == "" {
		return s.Overview(ctx), err
	}

	if _, err := s.m.SetBase(dir); err != nil {
		return s.Overview(ctx), err
	}

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
	err := s.m.Update(ctx, id, allowCopy, func(p sync.Progress) {
		s.host.Emit("update:progress", UpdateProgress{Server: id, Progress: p})
	})

	var needsCopy core.ErrNeedsCopy
	switch {
	case err == nil:
		return UpdateResult{OK: true}
	case errors.As(err, &needsCopy):
		return UpdateResult{NeedsCopy: true, CopyBytes: needsCopy.Bytes}
	default:
		return UpdateResult{Error: err.Error()}
	}
}

// Play starts the server.
func (s *ServerService) Play(ctx context.Context, id string) error {
	return s.m.Play(ctx, id)
}

// Choices returns the player's picks for a server.
func (s *ServerService) Choices(ctx context.Context, id string) (core.Choices, error) {
	return s.m.Choices(ctx, id)
}

// SetChannel switches release channel.
func (s *ServerService) SetChannel(ctx context.Context, id, channel string) error {
	return s.m.SetChannel(ctx, id, channel)
}

// SetComponent turns a component on at a version, or off with "".
func (s *ServerService) SetComponent(ctx context.Context, id, component, version string) error {
	return s.m.SetComponent(ctx, id, component, version)
}

// SetInstances sets the number of boxes and d2gl profile splitting.
func (s *ServerService) SetInstances(ctx context.Context, id string, n int, splitD2GL bool) error {
	return s.m.SetInstances(ctx, id, n, splitD2GL)
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
	return s.m.SetSetting(ctx, id, setting, value)
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
