package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/diablo2org/launcher/internal/install"
	"github.com/diablo2org/launcher/internal/settings"
	"github.com/diablo2org/launcher/internal/spec"
)

// CustomVersion is the component "version" for the player's own files: a
// maphack build they're testing, say. The launcher doesn't install, update,
// repair or remove anything that component could hold. It isn't supported by
// the server; the launcher says so when it's chosen.
const CustomVersion = "custom"

// Choices is what the player has picked for a server.
type Choices struct {
	Channel    string            `json:"channel"`
	Components map[string]string `json:"components"`
	Instances  int               `json:"instances"`
	SplitD2GL  bool              `json:"splitD2gl"`

	D2GLMainResolution   string   `json:"d2glMainResolution"`
	D2GLLoaderResolution string   `json:"d2glLoaderResolution"`
	D2GLResolutions      []string `json:"d2glResolutions"`
}

// Choices returns the player's current picks, with defaults filled in.
func (m *Manager) Choices(ctx context.Context, id string) (Choices, error) {
	p, err := m.Profile(ctx, id)
	if err != nil {
		return Choices{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	srv := m.state.Server(id)
	channel := srv.Channel
	if channel == "" {
		channel = p.Channels[0].ID
	}

	return Choices{
		Channel:    channel,
		Components: enabled(p, srv),
		Instances:  srv.Instances,
		SplitD2GL:  srv.SplitD2GL,

		D2GLMainResolution:   srv.D2GLMainResolution,
		D2GLLoaderResolution: srv.D2GLLoaderResolution,
		D2GLResolutions:      D2GLResolutions,
	}, nil
}

// SetChannel switches release channel.
func (m *Manager) SetChannel(ctx context.Context, id, channel string) error {
	p, err := m.Profile(ctx, id)
	if err != nil {
		return err
	}

	for _, c := range p.Channels {
		if c.ID == channel {
			m.mu.Lock()
			defer m.mu.Unlock()
			m.state.Server(id).Channel = channel
			return m.save()
		}
	}

	return fmt.Errorf("unknown channel %q", channel)
}

// SetComponent turns a component on at a version, on with the player's own
// files (CustomVersion), or off with "". Takes effect on the next update.
func (m *Manager) SetComponent(ctx context.Context, id, comp, version string) error {
	p, err := m.Profile(ctx, id)
	if err != nil {
		return err
	}

	if !hasComponent(p, comp) {
		return fmt.Errorf("unknown component %q", comp)
	}
	if version != "" && version != CustomVersion {
		if _, err := componentManifestURL(p, comp, version); err != nil {
			return err
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	srv := m.state.Server(id)
	srv.Components[comp] = version

	// Turning a component on turns off anything it conflicts with, whichever
	// side declared the conflict.
	if version != "" {
		for _, c := range p.Components {
			for _, other := range c.Conflicts {
				switch {
				case c.ID == comp:
					srv.Components[other] = ""
				case other == comp:
					srv.Components[c.ID] = ""
				}
			}
		}
	}

	return m.save()
}

func hasComponent(p *spec.Profile, comp string) bool {
	for _, c := range p.Components {
		if c.ID == comp {
			return true
		}
	}

	return false
}

// SetInstances sets how many boxes to start.
func (m *Manager) SetInstances(ctx context.Context, id string, n int, splitD2GL bool) error {
	p, err := m.Profile(ctx, id)
	if err != nil {
		return err
	}

	max := p.Launch.MaxInstances
	if max < 1 {
		max = 1
	}
	if n < 1 || n > max {
		return fmt.Errorf("%s allows 1 to %d boxes", p.Name, max)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	srv := m.state.Server(id)
	srv.Instances = n
	srv.SplitD2GL = splitD2GL
	return m.save()
}

// SettingValue is one declared setting with its current value.
type SettingValue struct {
	Setting   spec.Setting `json:"setting"`
	Value     interface{}  `json:"value"`
	Available bool         `json:"available"`
	Reason    string       `json:"reason"`
}

// Settings returns every setting the server declares, with its value. File
// settings are read from the file itself, so changes made in game show up.
func (m *Manager) Settings(ctx context.Context, id string) ([]SettingValue, error) {
	p, err := m.Profile(ctx, id)
	if err != nil {
		return nil, err
	}

	dir, err := m.serverDir(id)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	srv := m.state.Server(id)
	on := enabled(p, srv)
	flags := map[string]bool{}
	for _, f := range srv.Flags {
		flags[f] = true
	}
	m.mu.Unlock()

	out := make([]SettingValue, 0, len(p.Settings))
	for _, s := range p.Settings {
		v := SettingValue{Setting: s, Available: true}

		switch {
		case s.Component != "" && on[s.Component] == "":
			v.Available = false
			v.Reason = "Turn on " + componentName(p, s.Component) + " to change this."
		case s.Target.Flag != "":
			v.Value = flags[s.Target.Flag]
		default:
			value, err := settings.Read(dir, s)
			switch {
			case errors.Is(err, settings.ErrFileMissing):
				v.Available = false
				v.Reason = "Install or update " + p.Name + " to change this."
			case err != nil:
				v.Available = false
				v.Reason = err.Error()
			default:
				v.Value = value
			}
		}

		out = append(out, v)
	}

	return out, nil
}

func componentName(p *spec.Profile, id string) string {
	for _, c := range p.Components {
		if c.ID == id {
			return c.Name
		}
	}

	return id
}

// SetSetting changes one setting. File settings are written straight to the
// file; flag settings are stored and passed at launch.
func (m *Manager) SetSetting(ctx context.Context, id, settingID string, value interface{}) error {
	p, err := m.Profile(ctx, id)
	if err != nil {
		return err
	}

	var s *spec.Setting
	for i := range p.Settings {
		if p.Settings[i].ID == settingID {
			s = &p.Settings[i]
		}
	}
	if s == nil {
		return fmt.Errorf("unknown setting %q", settingID)
	}

	if s.Target.Flag == "" {
		return settings.Write(install.ServerDir(m.Base(), id), *s, value)
	}

	on, ok := value.(bool)
	if !ok {
		return fmt.Errorf("setting %q needs true or false", s.ID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	srv := m.state.Server(id)
	flags := []string{}
	for _, f := range srv.Flags {
		if f != s.Target.Flag {
			flags = append(flags, f)
		}
	}
	if on {
		flags = append(flags, s.Target.Flag)
	}
	srv.Flags = flags

	return m.save()
}
