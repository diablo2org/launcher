package spec

import (
	"fmt"
	"strings"

	"github.com/diablo2org/launcher/internal/paths"
)

// Profile is a server profile. See docs/SPEC.md section 3.
type Profile struct {
	Schema      int         `json:"schema"`
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Summary     string      `json:"summary,omitempty"`
	Version     int         `json:"version"`
	MinLauncher string      `json:"minLauncher,omitempty"`
	Links       Links       `json:"links,omitempty"`
	Branding    Branding    `json:"branding,omitempty"`
	Hosts       []string    `json:"hosts"`
	Game        Game        `json:"game"`
	Gateways    []Gateway   `json:"gateways"`
	Channels    []Channel   `json:"channels"`
	Components  []Component `json:"components,omitempty"`
	Settings    []Setting   `json:"settings,omitempty"`
	Launch      Launch      `json:"launch,omitempty"`
	News        string      `json:"news,omitempty"`
	Ladder      string      `json:"ladder,omitempty"`
	Signing     *Signing    `json:"signing,omitempty"`
}

// Links are opened in the player's browser, as buttons on the server's launch
// page.
type Links struct {
	Website  string `json:"website,omitempty"`
	Discord  string `json:"discord,omitempty"`
	Forum    string `json:"forum,omitempty"`
	Trade    string `json:"trade,omitempty"`
	Wiki     string `json:"wiki,omitempty"`
	Register string `json:"register,omitempty"`
	Support  string `json:"support,omitempty"`
	Donate   string `json:"donate,omitempty"`
}

// Branding is how the server's tab looks.
type Branding struct {
	Logo       *Asset `json:"logo,omitempty"`
	Background *Asset `json:"background,omitempty"`
	Accent     string `json:"accent,omitempty"`
	// BackgroundHasLogo means the background art already shows the logo, so
	// the launch screen doesn't draw it again.
	BackgroundHasLogo bool `json:"backgroundHasLogo,omitempty"`
}

// Asset is an image the launcher downloads and caches.
type Asset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256,omitempty"`
}

// Game describes the game the server runs.
type Game struct {
	Version      string `json:"version"`
	Expansion    *bool  `json:"expansion,omitempty"`
	BaseArchives string `json:"baseArchives"`
	Saves        string `json:"saves,omitempty"`
}

// NeedsExpansion reports whether d2exp.mpq is required. It defaults to true.
func (g Game) NeedsExpansion() bool {
	return g.Expansion == nil || *g.Expansion
}

// Gateway is a Battle.net gateway written to the registry at launch.
type Gateway struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Timezone int    `json:"timezone,omitempty"`
	Realm    string `json:"realm,omitempty"`
}

// Channel is a release channel, such as live or a test realm.
type Channel struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Manifest string `json:"manifest"`
}

// Component is an optional layer the player turns on per server.
type Component struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Kind      string             `json:"kind,omitempty"`
	Default   string             `json:"default,omitempty"`
	Flags     []string           `json:"flags,omitempty"`
	Conflicts []string           `json:"conflicts,omitempty"`
	Versions  []ComponentVersion `json:"versions"`
}

// ComponentVersion is one installable version of a component.
type ComponentVersion struct {
	ID       string `json:"id"`
	Manifest string `json:"manifest"`
}

// Setting is a player-facing option declared by the server.
type Setting struct {
	ID          string        `json:"id"`
	Label       string        `json:"label"`
	Description string        `json:"description,omitempty"`
	Group       string        `json:"group,omitempty"`
	Component   string        `json:"component,omitempty"`
	Type        string        `json:"type"`
	Default     interface{}   `json:"default,omitempty"`
	Options     []Option      `json:"options,omitempty"`
	Min         *int          `json:"min,omitempty"`
	Max         *int          `json:"max,omitempty"`
	MaxLength   int           `json:"maxLength,omitempty"`
	Target      SettingTarget `json:"target"`
}

// Option is one choice of a choice setting.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// SettingTarget is where a setting's value lives: a key in a file, or a
// launch flag.
type SettingTarget struct {
	File    string `json:"file,omitempty"`
	Format  string `json:"format,omitempty"`
	Section string `json:"section,omitempty"`
	Key     string `json:"key,omitempty"`
	Flag    string `json:"flag,omitempty"`
}

// Launch controls how the game is started.
type Launch struct {
	Exe          string   `json:"exe,omitempty"`
	DefaultFlags []string `json:"defaultFlags,omitempty"`
	AllowedFlags []string `json:"allowedFlags,omitempty"`
	MaxInstances int      `json:"maxInstances,omitempty"`
	// SetGateways is false for a server whose own game code sets the
	// gateway, so the launcher leaves the Battle.net registry values alone.
	SetGateways *bool `json:"setGateways,omitempty"`
}

// SetsGateways reports whether the launcher writes the gateway registry
// values at launch. It defaults to true.
func (l Launch) SetsGateways() bool {
	return l.SetGateways == nil || *l.SetGateways
}

// ExePath is the executable to start, defaulting to Game.exe.
func (l Launch) ExePath() string {
	if l.Exe == "" {
		return "Game.exe"
	}

	return l.Exe
}

// ParseProfile checks a server profile against the schema and the rules in
// docs/SPEC.md, returning every problem found.
func ParseProfile(data []byte) (*Profile, error) {
	var p Profile
	if err := decode(profileSchemaID, data, &p); err != nil {
		return nil, err
	}

	if err := p.check(); err != nil {
		return nil, err
	}

	return &p, nil
}

func (p *Profile) check() error {
	var problems Problems

	// Every download must come from a host the profile declares.
	urls := map[string]string{"news": p.News, "ladder": p.Ladder}
	if p.Branding.Logo != nil {
		urls["branding.logo"] = p.Branding.Logo.URL
	}
	if p.Branding.Background != nil {
		urls["branding.background"] = p.Branding.Background.URL
	}
	for _, c := range p.Channels {
		urls["channel "+c.ID] = c.Manifest
	}
	for _, c := range p.Components {
		for _, v := range c.Versions {
			urls[fmt.Sprintf("component %s %s", c.ID, v.ID)] = v.Manifest
		}
	}
	for where, u := range urls {
		if u != "" && !allowedHost(u, p.Hosts) {
			problems.add("%s: %s is not on one of the profile's hosts", where, u)
		}
	}

	channels := make(map[string]bool, len(p.Channels))
	for _, c := range p.Channels {
		if channels[c.ID] {
			problems.add("channel %q is declared twice", c.ID)
		}
		channels[c.ID] = true
	}

	components := make(map[string]bool, len(p.Components))
	for _, c := range p.Components {
		if components[c.ID] {
			problems.add("component %q is declared twice", c.ID)
		}
		components[c.ID] = true
	}

	for _, c := range p.Components {
		versions := make(map[string]bool, len(c.Versions))
		for _, v := range c.Versions {
			if versions[v.ID] {
				problems.add("component %q: version %q is declared twice", c.ID, v.ID)
			}
			if strings.EqualFold(v.ID, "custom") {
				problems.add("component %q: \"custom\" is reserved for players' own files", c.ID)
			}
			versions[v.ID] = true
		}

		if c.Default != "" && !versions[c.Default] {
			problems.add("component %q: default %q is not one of its versions", c.ID, c.Default)
		}

		for _, other := range c.Conflicts {
			switch {
			case other == c.ID:
				problems.add("component %q conflicts with itself", c.ID)
			case !components[other]:
				problems.add("component %q conflicts with unknown component %q", c.ID, other)
			}
		}
	}

	allowed := make(map[string]bool, len(p.Launch.AllowedFlags))
	for _, f := range p.Launch.AllowedFlags {
		allowed[f] = true
	}

	if err := paths.Check(p.Launch.ExePath()); err != nil {
		problems.add("launch.exe: %v", err)
	}

	settings := make(map[string]bool, len(p.Settings))
	for _, s := range p.Settings {
		if settings[s.ID] {
			problems.add("setting %q is declared twice", s.ID)
		}
		settings[s.ID] = true

		if s.Component != "" && !components[s.Component] {
			problems.add("setting %q: unknown component %q", s.ID, s.Component)
		}

		if s.Target.Flag != "" && !allowed[s.Target.Flag] {
			problems.add("setting %q: flag %s is not in launch.allowedFlags", s.ID, s.Target.Flag)
		}

		if s.Target.File != "" {
			if err := paths.Check(s.Target.File); err != nil {
				problems.add("setting %q: %v", s.ID, err)
			}
			if s.Target.Format == "ini" && s.Target.Section == "" {
				problems.add("setting %q: ini settings need a section", s.ID)
			}
		}

		if msg := s.checkDefault(); msg != "" {
			problems.add("setting %q: %s", s.ID, msg)
		}
	}

	if _, err := p.ManifestKeys(); err != nil {
		problems.add("signing: %v", err)
	}

	return problems.err()
}

// checkDefault makes sure a default matches the setting's type.
func (s Setting) checkDefault() string {
	if s.Default == nil {
		return ""
	}

	switch s.Type {
	case "bool":
		if _, ok := s.Default.(bool); !ok {
			return "default must be true or false"
		}
	case "int":
		n, ok := s.Default.(float64)
		if !ok || n != float64(int(n)) {
			return "default must be a whole number"
		}
		if (s.Min != nil && int(n) < *s.Min) || (s.Max != nil && int(n) > *s.Max) {
			return "default is outside min/max"
		}
	case "string":
		if _, ok := s.Default.(string); !ok {
			return "default must be a string"
		}
	case "choice":
		v, ok := s.Default.(string)
		if !ok {
			return "default must be one of the option values"
		}
		for _, o := range s.Options {
			if o.Value == v {
				return ""
			}
		}
		return fmt.Sprintf("default %q is not one of the options", v)
	}

	return ""
}
