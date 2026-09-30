package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (h *harness) write(t *testing.T, rel, body string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(h.base, "slash", filepath.FromSlash(rel)), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A custom maphack is the player's: the launcher never overwrites, repairs or
// removes it, and its settings keep working.
func TestCustomComponentLeftAlone(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.m.Servers(ctx)

	if err := h.m.Update(ctx, "slash", false, nil); err != nil {
		t.Fatal(err)
	}

	if err := h.m.SetComponent(ctx, "slash", "maphack", CustomVersion); err != nil {
		t.Fatal(err)
	}
	h.write(t, "BH.dll", "my own build")

	if st := h.m.Status(ctx, "slash"); !st.UpToDate || st.Error != "" {
		t.Errorf("custom maphack shows as needing an update: %+v", st)
	}
	if err := h.m.Update(ctx, "slash", false, nil); err != nil {
		t.Fatal(err)
	}
	if got := h.read(t, "BH.dll"); got != "my own build" {
		t.Errorf("BH.dll = %q; the custom build was replaced", got)
	}

	// Its settings still apply.
	if err := h.m.SetSetting(ctx, "slash", "reveal", true); err != nil {
		t.Errorf("maphack setting with a custom maphack: %v", err)
	}

	c, _ := h.m.Choices(ctx, "slash")
	if c.Components["maphack"] != CustomVersion {
		t.Errorf("choices = %+v", c.Components)
	}

	// Going back to a real version puts the server's files back.
	h.m.SetComponent(ctx, "slash", "maphack", "1")
	if st := h.m.Status(ctx, "slash"); st.UpToDate {
		t.Error("switching back to version 1 shows nothing to do")
	}
	if err := h.m.Update(ctx, "slash", false, nil); err != nil {
		t.Fatal(err)
	}
	if got := h.read(t, "BH.dll"); got != "bh one" {
		t.Errorf("BH.dll = %q after switching back", got)
	}
}

// A custom renderer's files are protected even where the base channel lists
// the same path, and its launch flags still apply.
func TestCustomComponentSharedPath(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.m.Servers(ctx)

	h.m.SetComponent(ctx, "slash", "d2gl", CustomVersion)
	if err := h.m.Update(ctx, "slash", false, nil); err != nil {
		t.Fatal(err)
	}
	h.write(t, "glide3x.dll", "my glide build")

	if st := h.m.Status(ctx, "slash"); !st.UpToDate {
		t.Errorf("custom glide3x.dll shows as needing an update: %+v", st)
	}
	if err := h.m.Update(ctx, "slash", false, nil); err != nil {
		t.Fatal(err)
	}
	if got := h.read(t, "glide3x.dll"); got != "my glide build" {
		t.Errorf("glide3x.dll = %q; the base layer overwrote the custom build", got)
	}

	if err := h.m.Play(ctx, "slash"); err != nil {
		t.Fatal(err)
	}
	if len(h.started) != 1 || !strings.Contains(strings.Join(h.started[0].Args, " "), "-3dfx") {
		t.Errorf("custom d2gl lost its launch flag: %+v", h.started)
	}
}

func TestCustomRejectsUnknownComponent(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.m.Servers(ctx)

	if err := h.m.SetComponent(ctx, "slash", "nope", CustomVersion); err == nil {
		t.Error("custom accepted for an unknown component")
	}
}

func TestServerFolder(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.m.Servers(ctx)

	dir, err := h.m.ServerFolder("slash")
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() || filepath.Base(dir) != "slash" {
		t.Errorf("ServerFolder = %q, %v", dir, err)
	}
}
