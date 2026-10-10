package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"

	"github.com/diablo2org/launcher/internal/core"
	"github.com/diablo2org/launcher/internal/logs"
)

// LinkScheme is the URL scheme the installer registers, so a web page can
// link to diablo2org://add?profile=<https URL of a server profile>.
const LinkScheme = "diablo2org"

// maxLink is the longest link accepted, the same as a profile's URLs.
const maxLink = 2048

// LinkArg returns the first command line argument that is one of the
// launcher's links, or "". Windows passes a clicked link as an argument.
func LinkArg(args []string) string {
	for _, a := range args {
		if strings.HasPrefix(strings.ToLower(a), LinkScheme+":") {
			return a
		}
	}

	return ""
}

// ParseAddLink returns the profile URL an add link names. Only the URL is
// checked here; nothing is fetched until the player agrees.
func ParseAddLink(raw string) (string, error) {
	if len(raw) > maxLink {
		return "", errors.New("the link is too long")
	}

	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, LinkScheme) {
		return "", errors.New("this isn't a launcher link")
	}

	// diablo2org://add?... has the action as its host; diablo2org:add?...
	// as its opaque part.
	action := u.Host
	if action == "" {
		action = u.Opaque
	}
	if !strings.EqualFold(strings.Trim(action, "/"), "add") {
		return "", fmt.Errorf("the launcher doesn't know the link action %q", action)
	}

	profile := u.Query().Get("profile")
	p, err := url.Parse(profile)
	if profile == "" || err != nil || p.Scheme != "https" || p.Hostname() == "" || p.User != nil {
		return "", errors.New("the link's profile must be an https URL")
	}

	return p.String(), nil
}

// pendingLink is the link the frontend hasn't taken yet.
type pendingLink struct {
	mu  sync.Mutex
	req *LinkRequest
}

// LinkRequest is a link waiting for the player to agree to it.
type LinkRequest struct {
	// URL is the profile the link asks to add.
	URL string `json:"url"`
	// Host is where that profile comes from, for the player to see.
	Host string `json:"host"`
	// Error says why the link can't be used; URL is then empty.
	Error string `json:"error"`
}

// HandleLink takes a link the launcher was started with, or sent while
// running, and holds it until the frontend asks for it. running says the
// window is up, to be told and brought to the front. An empty link is
// ignored. It isn't a ServerService method so the frontend can't call it.
func HandleLink(s *ServerService, raw string, running bool) {
	if raw == "" {
		return
	}

	req := LinkRequest{}
	if profile, err := ParseAddLink(raw); err != nil {
		slog.Warn("link refused", "link", logs.RedactURLs(raw), "err", err)
		req.Error = err.Error()
	} else {
		u, _ := url.Parse(profile)
		req.URL, req.Host = profile, u.Hostname()
		slog.Info("link received", "profile", logs.RedactURLs(profile))
	}

	s.links.mu.Lock()
	s.links.req = &req
	s.links.mu.Unlock()

	if !running {
		return
	}
	if s.host.Emit != nil {
		s.host.Emit("link", nil)
	}
	if s.host.Focus != nil {
		s.host.Focus()
	}
}

// PendingLink returns the link waiting for the player, once, or nil.
func (s *ServerService) PendingLink() *LinkRequest {
	s.links.mu.Lock()
	defer s.links.mu.Unlock()

	req := s.links.req
	s.links.req = nil
	return req
}

// AddFromLink adds the server a link named, once the player has agreed, pins
// it if there is room, and returns its id. A server that is already listed is
// just opened.
func (s *ServerService) AddFromLink(ctx context.Context, profileURL string) (string, error) {
	p, err := s.m.AddServer(ctx, profileURL)
	switch {
	case errors.Is(err, core.ErrAlreadyListed) && p != nil:
		slog.Info("link opened a listed server", "server", p.ID)
	case err != nil:
		slog.Error("add server from link", "url", logs.RedactURLs(profileURL), "err", logs.RedactURLs(err.Error()))
		return "", err
	default:
		slog.Info("server added from link", "server", p.ID, "url", logs.RedactURLs(profileURL))
	}

	if !s.isFavourite(p.ID) {
		if err := s.m.SetFavourite(p.ID, true); err != nil && !errors.Is(err, core.ErrTooManyFavourites) {
			return "", err
		}
	}

	return p.ID, nil
}

func (s *ServerService) isFavourite(id string) bool {
	for _, f := range s.m.Favourites() {
		if f == id {
			return true
		}
	}

	return false
}
