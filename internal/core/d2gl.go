package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/diablo2org/launcher/internal/settings"
	"github.com/diablo2org/launcher/internal/spec"
)

// D2GLResolutions are the window sizes d2gl offers itself, from its own list,
// for per-box profiles. "" means leave the profile's size alone.
var D2GLResolutions = []string{
	"800x600", "960x720", "1024x768", "1200x900", "1280x960", "1440x1080",
	"1600x1200", "1920x1440", "2560x1920", "2732x2048",
	"1068x600", "1280x720", "1600x900", "1920x1080", "2048x1152",
	"2560x1440", "3200x1800", "3840x2160",
}

func validResolution(r string) bool {
	if r == "" {
		return true
	}
	for _, v := range D2GLResolutions {
		if v == r {
			return true
		}
	}
	return false
}

// SetD2GLResolutions sets the window size for the main box and the loader
// boxes when d2gl profiles are split.
func (m *Manager) SetD2GLResolutions(id, main, loader string) error {
	if !validResolution(main) || !validResolution(loader) {
		return errors.New("unknown resolution")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	srv := m.state.Server(id)
	srv.D2GLMainResolution = main
	srv.D2GLLoaderResolution = loader
	return m.save()
}

func d2glOn(p *spec.Profile, components map[string]string) bool {
	for _, c := range p.Components {
		if c.Kind == "d2gl" && components[c.ID] != "" {
			return true
		}
	}
	return false
}

// applyD2GLProfiles writes the chosen window sizes into d2gl_main.ini and
// d2gl_loader.ini before launch. A profile that doesn't exist yet starts as
// a copy of d2gl.ini, so it inherits everything else the player has set.
// Only the size keys change; d2gl's own options menu keeps working for the
// rest.
func applyD2GLProfiles(dir, main, loader string) error {
	for profile, res := range map[string]string{"main": main, "loader": loader} {
		if res == "" {
			continue
		}

		w, h, ok := parseResolution(res)
		if !ok {
			continue
		}

		rel := "d2gl_" + profile + ".ini"
		path := filepath.Join(dir, rel)
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			seed, _ := os.ReadFile(filepath.Join(dir, "d2gl.ini"))
			if len(seed) == 0 {
				seed = []byte("[Screen]\r\n")
			}
			if err := os.WriteFile(path, seed, 0o644); err != nil {
				return err
			}
		}

		err := settings.SetINIKeys(dir, rel, "Screen", map[string]string{
			"window_width":  strconv.Itoa(w),
			"window_height": strconv.Itoa(h),
			// A fullscreen window ignores the size, so an explicit size only
			// means anything windowed.
			"fullscreen": "false",
		})
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
	}

	return nil
}

func parseResolution(r string) (int, int, bool) {
	parts := strings.Split(r, "x")
	if len(parts) != 2 {
		return 0, 0, false
	}

	w, err1 := strconv.Atoi(parts[0])
	h, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		return 0, 0, false
	}

	return w, h, true
}
