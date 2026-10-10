package launch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/diablo2org/launcher/internal/spec"
)

// Choices are the player's per-server selections that affect how the game
// starts.
type Choices struct {
	// Components maps each turned-on component to its version.
	Components map[string]string
	// Flags are the flags the player turned on, through flag settings or the
	// advanced page. Only flags in the profile's allowedFlags are used.
	Flags []string
	// Instances is how many boxes to start.
	Instances int
	// SplitD2GL launches the first box with the d2gl "main" profile and the
	// rest with "loader", so each can keep its own resolution.
	SplitD2GL bool
}

// Box is one game process to start.
type Box struct {
	Exe  string
	Args []string
	Dir  string
}

// d2glProfile is the d2gl config a box uses when profiles are split. d2gl
// reads "-config <name>" and then uses d2gl_<name>.ini.
func d2glProfile(box int) string {
	if box == 0 {
		return "main"
	}

	return "loader"
}

// Boxes works out exactly what to start. Flags come from the profile's
// defaults, the turned-on components and the player's allowed choices, in that
// order, without duplicates.
func Boxes(serverDir string, p *spec.Profile, c Choices) ([]Box, error) {
	instances := c.Instances
	if instances < 1 {
		instances = 1
	}

	max := p.Launch.MaxInstances
	if max < 1 {
		max = 1
	}
	if instances > max {
		return nil, fmt.Errorf("%s allows at most %d boxes", p.Name, max)
	}

	allowed := make(map[string]bool, len(p.Launch.AllowedFlags))
	for _, f := range p.Launch.AllowedFlags {
		allowed[f] = true
	}

	var flags []string
	seen := map[string]bool{}
	add := func(f string) {
		if !seen[f] {
			seen[f] = true
			flags = append(flags, f)
		}
	}

	for _, f := range p.Launch.DefaultFlags {
		add(f)
	}

	d2gl := false
	for _, comp := range p.Components {
		if _, on := c.Components[comp.ID]; !on {
			continue
		}
		if comp.Kind == "d2gl" {
			d2gl = true
		}
		for _, f := range comp.Flags {
			add(f)
		}
	}

	for _, f := range c.Flags {
		if !allowed[f] {
			return nil, fmt.Errorf("flag %s is not allowed by %s", f, p.Name)
		}
		add(f)
	}

	exe := filepath.Join(serverDir, filepath.FromSlash(p.Launch.ExePath()))

	boxes := make([]Box, instances)
	for i := range boxes {
		args := append([]string{}, flags...)
		if d2gl && c.SplitD2GL {
			args = append(args, "-config", d2glProfile(i))
		}
		boxes[i] = Box{Exe: exe, Args: args, Dir: serverDir}
	}

	return boxes, nil
}

// Registry is the game's Battle.net settings in the registry. It's an
// interface so launches can be tested without touching the real registry.
type Registry interface {
	GatewayList() ([]string, error)
	SetGatewayList([]string) error
	// String reads a value under the game's own key; one not set is "".
	String(name string) (string, error)
	SetString(name, value string) error
}

// Starter starts a game process.
type Starter func(Box) error

// Launcher starts servers one at a time. Battle.net settings are global, so
// two servers launching together could each pick up the other's gateway.
type Launcher struct {
	mu       sync.Mutex
	registry Registry
	start    Starter
	sleep    func(context.Context, time.Duration) error
}

// New returns a launcher using the given registry and process starter.
func New(r Registry, start Starter) *Launcher {
	return &Launcher{registry: r, start: start, sleep: sleepCtx}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Launch points Battle.net at the server and starts its boxes, waiting delay
// between each so the game's own startup reads of the registry don't overlap.
func (l *Launcher) Launch(ctx context.Context, base, serverDir string, p *spec.Profile, c Choices, delay time.Duration) error {
	boxes, err := Boxes(serverDir, p, c)
	if err != nil {
		return err
	}

	if _, err := os.Stat(boxes[0].Exe); err != nil {
		return fmt.Errorf("%s is not installed: %w", p.Name, err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.configure(base, serverDir, p); err != nil {
		return fmt.Errorf("setting up Battle.net: %w", err)
	}

	for i, b := range boxes {
		if i > 0 {
			if err := l.sleep(ctx, delay); err != nil {
				return err
			}
		}

		if err := l.start(b); err != nil {
			return fmt.Errorf("starting box %d: %w", i+1, err)
		}
	}

	return nil
}

// configure writes the registry values listed in docs/SPEC.md section 8.
func (l *Launcher) configure(base, serverDir string, p *spec.Profile) error {
	if p.Launch.SetsGateways() {
		if err := l.setGateways(p.Gateways); err != nil {
			return err
		}
	}

	return l.setSavePath(base, serverDir, p)
}

func (l *Launcher) setGateways(gateways []spec.Gateway) error {
	existing, err := l.registry.GatewayList()
	if err != nil {
		return err
	}

	list := Merge(ParseGatewayList(existing), gateways)
	if err := l.registry.SetGatewayList(list.Values()); err != nil {
		return err
	}

	first := gateways[0]
	if err := l.registry.SetString("BNETIP", first.Host); err != nil {
		return err
	}

	if first.Realm != "" {
		return l.registry.SetString("Preferred Realm", first.Realm)
	}

	return nil
}

// setSavePath points the game at the server's saves. An isolated server
// always writes its own folder. A shared server writes the base's Save
// folder only when Save Path is unset or still holds an isolated server's
// folder, so an isolated path never carries over to the next server and a
// Save Path the player chose is kept.
func (l *Launcher) setSavePath(base, serverDir string, p *spec.Profile) error {
	save := filepath.Join(base, "Save")

	if p.Game.Saves == "isolated" {
		save = filepath.Join(serverDir, "Save")
	} else {
		current, err := l.registry.String("Save Path")
		if err != nil {
			return err
		}
		if current != "" && !isolatedSavePath(base, current) {
			return nil
		}
	}

	if err := os.MkdirAll(save, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}

	return l.registry.SetString("Save Path", save+string(filepath.Separator))
}

// serverID is the profile spec's id pattern.
var serverID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,31}$`)

// isolatedSavePath reports whether path is <base>\<server id>\Save, the
// folder the launcher writes for an isolated server.
func isolatedSavePath(base, path string) bool {
	path = filepath.Clean(path)
	server := filepath.Dir(path)

	return strings.EqualFold(filepath.Base(path), "Save") &&
		strings.EqualFold(filepath.Dir(server), filepath.Clean(base)) &&
		serverID.MatchString(strings.ToLower(filepath.Base(server)))
}
