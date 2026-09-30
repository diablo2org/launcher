package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestD2GLProfilesOnPlay(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.m.Servers(ctx)

	h.m.SetComponent(ctx, "slash", "d2gl", "1.3.3")
	if err := h.m.Update(ctx, "slash", false, nil); err != nil {
		t.Fatal(err)
	}
	h.m.SetInstances(ctx, "slash", 2, true)

	if err := h.m.SetD2GLResolutions("slash", "3840x2160", "bogus"); err == nil {
		t.Error("unknown resolution accepted")
	}
	if err := h.m.SetD2GLResolutions("slash", "3840x2160", "1600x900"); err != nil {
		t.Fatal(err)
	}

	if err := h.m.Play(ctx, "slash"); err != nil {
		t.Fatal(err)
	}

	main := h.read(t, "d2gl_main.ini")
	for _, want := range []string{"[Screen]", "window_width=3840", "window_height=2160", "fullscreen=false", "unlock_cursor=false"} {
		if !strings.Contains(main, want) {
			t.Errorf("d2gl_main.ini missing %q:\n%s", want, main)
		}
	}
	if loader := h.read(t, "d2gl_loader.ini"); !strings.Contains(loader, "window_width=1600") {
		t.Errorf("d2gl_loader.ini:\n%s", loader)
	}

	// The boxes are told which profile to use.
	if len(h.started) != 2 || !strings.Contains(strings.Join(h.started[1].Args, " "), "-config loader") {
		t.Errorf("started %+v", h.started)
	}
}

func TestD2GLProfilesLeftAloneWhenNotSplit(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.m.Servers(ctx)

	h.m.SetComponent(ctx, "slash", "d2gl", "1.3.3")
	h.m.Update(ctx, "slash", false, nil)
	h.m.SetD2GLResolutions("slash", "3840x2160", "")

	if err := h.m.Play(ctx, "slash"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(h.base, "slash", "d2gl_main.ini")); !os.IsNotExist(err) {
		t.Error("wrote a box profile without split profiles")
	}
}
