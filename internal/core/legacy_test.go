package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportLegacy(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// A fresh launcher: no folder, nothing pinned.
	st, _ := h.store.Load()
	st.BasePath = ""
	st.Favourites = nil
	h.store.Save(st)
	m, _ := New(h.store, h.m.listing, h.m.launcher, h.m.clientOpts...)

	// The harness server is "slash"; import targets LegacyServer.
	h.profile["id"] = LegacyServer
	h.list(t)
	if _, err := m.Servers(ctx); err != nil {
		t.Fatal(err)
	}

	oldBase := strings.ReplaceAll(h.base, `\`, "/")
	config := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(config, []byte(`{
		"launch_delay": 3000,
		"games": [
			{"location": "/`+oldBase+`", "instances": 1, "flags": ["-3dfx", "-w", "-skiptobnet"],
			 "hd_version": "none", "maphack_version": "2", "d2gl_version": "1.3.3", "d2gl_split_profiles": true},
			{"location": "/C:/elsewhere", "instances": 3, "flags": []}
		]
	}`), 0o644)

	imported, err := m.ImportLegacy(ctx, config)
	if err != nil || !imported {
		t.Fatalf("ImportLegacy = %v, %v", imported, err)
	}

	if got := m.Base(); filepath.Clean(got) != filepath.Clean(h.base) {
		t.Errorf("base = %q, want %q", got, h.base)
	}
	if m.LaunchDelay() != 3000 {
		t.Errorf("launch delay = %d", m.LaunchDelay())
	}
	if favs := m.Favourites(); len(favs) != 1 || favs[0] != LegacyServer {
		t.Errorf("favourites = %v", favs)
	}

	c, err := m.Choices(ctx, LegacyServer)
	if err != nil {
		t.Fatal(err)
	}
	// 1 + 3 boxes, capped at the profile's maximum of 2.
	if c.Instances != 2 || !c.SplitD2GL || c.Components["maphack"] != "2" || c.Components["d2gl"] != "1.3.3" {
		t.Errorf("choices = %+v", c)
	}

	m.mu.Lock()
	flags := strings.Join(m.state.Server(LegacyServer).Flags, " ")
	m.mu.Unlock()
	if flags != "-w" {
		t.Errorf("flags = %q; only allowed flags should come across", flags)
	}

	// Only once.
	if again, _ := m.ImportLegacy(ctx, config); again {
		t.Error("imported twice")
	}
}

func TestLegacyPath(t *testing.T) {
	if got := legacyPath("/C:/Users/Me/Diablo II"); got != filepath.Clean(`C:\Users\Me\Diablo II`) && got != filepath.Clean("C:/Users/Me/Diablo II") {
		t.Errorf("legacyPath = %q", got)
	}
}
