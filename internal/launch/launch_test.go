package launch

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/d2org/launcher/internal/spec"
)

func TestParseGatewayList(t *testing.T) {
	// As found on a real 1.13c install.
	l := ParseGatewayList([]string{"9999", "01", "192.168.0.31", "-12", "Build"})

	if l.Header != "9999" || l.Selected != 1 || len(l.Gateways) != 1 {
		t.Fatalf("got %+v", l)
	}
	if g := l.Gateways[0]; g.Host != "192.168.0.31" || g.Timezone != -12 || g.Name != "Build" {
		t.Errorf("gateway = %+v", g)
	}

	// Round trip.
	if got := l.Values(); !reflect.DeepEqual(got, []string{"9999", "01", "192.168.0.31", "-12", "Build"}) {
		t.Errorf("Values = %q", got)
	}

	// Junk and a trailing partial entry don't break it.
	if l := ParseGatewayList([]string{"1001", "02", "a.net", "0", "A", "b.net"}); len(l.Gateways) != 1 {
		t.Errorf("partial entry parsed: %+v", l)
	}
	if l := ParseGatewayList(nil); l.Header != DefaultGatewayHeader || len(l.Gateways) != 0 {
		t.Errorf("empty = %+v", l)
	}
}

func TestMerge(t *testing.T) {
	existing := ParseGatewayList([]string{
		"9999", "01",
		"192.168.0.31", "-12", "Build",
		"PLAY.slashdiablo.net", "0", "Old Slash entry",
	})

	merged := Merge(existing, []spec.Gateway{{Name: "SlashDiablo", Host: "play.slashdiablo.net"}})

	want := []string{
		"9999", "02",
		"192.168.0.31", "-12", "Build",
		"play.slashdiablo.net", "0", "SlashDiablo",
	}
	if got := merged.Values(); !reflect.DeepEqual(got, want) {
		t.Errorf("merged = %q\nwant %q", got, want)
	}
}

func profile() *spec.Profile {
	return &spec.Profile{
		ID:       "slashdiablo",
		Name:     "SlashDiablo",
		Game:     spec.Game{Version: "1.13c", BaseArchives: "link"},
		Gateways: []spec.Gateway{{Name: "SlashDiablo", Host: "play.slashdiablo.net", Realm: "Slash Diablo"}},
		Components: []spec.Component{
			{ID: "maphack", Kind: "bh"},
			{ID: "d2gl", Kind: "d2gl", Flags: []string{"-3dfx"}},
		},
		Launch: spec.Launch{
			DefaultFlags: []string{"-skiptobnet"},
			AllowedFlags: []string{"-w", "-3dfx", "-ns"},
			MaxInstances: 4,
		},
	}
}

func TestBoxes(t *testing.T) {
	c := Choices{
		Components: map[string]string{"d2gl": "1.3.3"},
		Flags:      []string{"-w", "-3dfx"},
		Instances:  3,
		SplitD2GL:  true,
	}

	boxes, err := Boxes(`C:\D2\slashdiablo`, profile(), c)
	if err != nil {
		t.Fatal(err)
	}

	if len(boxes) != 3 {
		t.Fatalf("got %d boxes", len(boxes))
	}
	if boxes[0].Exe != filepath.Join(`C:\D2\slashdiablo`, "Game.exe") || boxes[0].Dir != `C:\D2\slashdiablo` {
		t.Errorf("box = %+v", boxes[0])
	}

	// Defaults, then component flags, then the player's, without the
	// duplicate -3dfx; then the d2gl profile per box.
	if want := []string{"-skiptobnet", "-3dfx", "-w", "-config", "main"}; !reflect.DeepEqual(boxes[0].Args, want) {
		t.Errorf("box 1 args = %q, want %q", boxes[0].Args, want)
	}
	if want := []string{"-skiptobnet", "-3dfx", "-w", "-config", "loader"}; !reflect.DeepEqual(boxes[2].Args, want) {
		t.Errorf("box 3 args = %q, want %q", boxes[2].Args, want)
	}
}

func TestBoxesRules(t *testing.T) {
	p := profile()

	if _, err := Boxes("x", p, Choices{Flags: []string{"-direct"}}); err == nil {
		t.Error("a flag outside allowedFlags was accepted")
	}

	if _, err := Boxes("x", p, Choices{Instances: 5}); err == nil {
		t.Error("more boxes than maxInstances were accepted")
	}

	// Split profiles only apply while d2gl is on.
	boxes, _ := Boxes("x", p, Choices{Instances: 2, SplitD2GL: true})
	for _, b := range boxes {
		if strings.Contains(strings.Join(b.Args, " "), "-config") {
			t.Errorf("-config passed without d2gl: %q", b.Args)
		}
	}
}

type fakeRegistry struct {
	gateways []string
	strings  map[string]string
}

func (f *fakeRegistry) GatewayList() ([]string, error)     { return f.gateways, nil }
func (f *fakeRegistry) SetGatewayList(v []string) error    { f.gateways = v; return nil }
func (f *fakeRegistry) SetString(name, value string) error { f.strings[name] = value; return nil }

func TestLaunch(t *testing.T) {
	base := t.TempDir()
	server := filepath.Join(base, "slashdiablo")
	os.MkdirAll(server, 0o755)
	os.WriteFile(filepath.Join(server, "Game.exe"), []byte("exe"), 0o644)

	reg := &fakeRegistry{strings: map[string]string{}}
	var started []Box
	var slept []time.Duration

	l := New(reg, func(b Box) error { started = append(started, b); return nil })
	l.sleep = func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }

	if err := l.Launch(context.Background(), base, server, profile(), Choices{Instances: 2}, 3*time.Second); err != nil {
		t.Fatal(err)
	}

	if len(started) != 2 || len(slept) != 1 || slept[0] != 3*time.Second {
		t.Errorf("started %d boxes with waits %v", len(started), slept)
	}

	if !reflect.DeepEqual(reg.gateways, []string{"1001", "01", "play.slashdiablo.net", "0", "SlashDiablo"}) {
		t.Errorf("gateways = %q", reg.gateways)
	}
	if reg.strings["BNETIP"] != "play.slashdiablo.net" || reg.strings["Preferred Realm"] != "Slash Diablo" {
		t.Errorf("strings = %v", reg.strings)
	}
	if want := filepath.Join(base, "Save") + string(filepath.Separator); reg.strings["Save Path"] != want {
		t.Errorf("Save Path = %q, want the shared %q", reg.strings["Save Path"], want)
	}

	// Isolated saves go in the server folder.
	p := profile()
	p.Game.Saves = "isolated"
	l.Launch(context.Background(), base, server, p, Choices{}, 0)
	if want := filepath.Join(server, "Save") + string(filepath.Separator); reg.strings["Save Path"] != want {
		t.Errorf("Save Path = %q, want %q", reg.strings["Save Path"], want)
	}
}

func TestLaunchNotInstalled(t *testing.T) {
	reg := &fakeRegistry{strings: map[string]string{}}
	l := New(reg, func(Box) error { t.Error("started a box"); return nil })

	if err := l.Launch(context.Background(), t.TempDir(), t.TempDir(), profile(), Choices{}, 0); err == nil {
		t.Error("launched with no Game.exe")
	}

	if reg.gateways != nil {
		t.Error("registry written for a launch that never happened")
	}
}
