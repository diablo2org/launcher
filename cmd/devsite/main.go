// Command devsite builds a local test environment: a sandbox Diablo II folder,
// three test servers built from a real install, their profiles in a listing,
// and a launcher data folder with them pinned. testenv/dev.ps1 runs it, then
// serves the site with devserve and starts the launcher against it.
//
// Nothing is copied that can be hard linked, so the environment costs almost
// no disk space, and the player's own Diablo II folder is only read.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/d2org/launcher/internal/install"
	"github.com/d2org/launcher/internal/pack"
	"github.com/d2org/launcher/internal/spec"
	"github.com/d2org/launcher/internal/store"
)

func main() {
	source := flag.String("source", "", "a working SlashDiablo install to build the test servers from")
	base := flag.String("base", "", "the Diablo II folder holding the base archives (defaults to -source)")
	out := flag.String("out", "testenv/.dev", "where to build the environment")
	art := flag.String("art", "", "optional folder with the old launcher's bg.png, logo-bg.png and logo-text.png")
	extraArt := flag.String("extra-art", "", "optional folder with other servers' art: resurgence-logo.png, resurgence-bg.png, diablo09-og.jpg")
	host := flag.String("url", "https://127.0.0.1:8667", "where devserve will serve the site")
	flag.Parse()

	if *source == "" {
		log.Fatal("-source is required")
	}
	if *base == "" {
		*base = *source
	}

	if err := build(*source, *base, *out, *art, *extraArt, *host); err != nil {
		log.Fatal(err)
	}
}

func build(source, base, out, art, extraArt, siteURL string) error {
	site := filepath.Join(out, "site")
	sandbox := filepath.Join(out, "base")

	for _, dir := range []string{site, filepath.Join(out, "listing")} {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	// A sandbox Diablo II folder: links to the real base archives, so the
	// launcher can create server folders without touching the real install.
	if err := os.MkdirAll(sandbox, 0o755); err != nil {
		return err
	}
	for _, a := range install.Archives {
		src := filepath.Join(base, a.Name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := linkOrCopy(src, filepath.Join(sandbox, a.Name)); err != nil {
			return err
		}
	}
	if r, err := install.Check(sandbox, true); err != nil || !r.OK() {
		return fmt.Errorf("sandbox Diablo II folder is incomplete (missing %v): %v", r.Missing, err)
	}

	// One live channel, linked from the source install, shared by every test
	// server through its own manifest.
	live := filepath.Join(site, "files", "live")
	if err := linkTree(source, live); err != nil {
		return err
	}

	// The test servers. SlashDiablo's details are real. Resurgence and
	// Diablo 09 use their real names, links and art, but everything else is
	// a dummy: their gateways are made-up .test hosts, and all three install
	// SlashDiablo's game files.
	type server struct {
		id, name, summary, accent string
		game, gateway, realm      string
		links                     map[string]any
		// components splits the maphack and d2gl out as optional layers.
		components bool
		art        func(dir string) (map[string]asset, bool, error)
	}

	servers := []server{
		{
			id: "slashdiablo", name: "SlashDiablo", accent: "#5c0202",
			summary: "Local test build of SlashDiablo, served from your own install.",
			game:    "1.13c", gateway: "play.slashdiablo.net", realm: "Slash Diablo",
			links:      map[string]any{"website": "https://slashdiablo.net", "discord": "https://discord.gg/slashdiablo"},
			components: true,
			art: func(dir string) (map[string]asset, bool, error) {
				if art == "" {
					return nil, false, nil
				}
				a, err := slashArt(art, dir)
				return a, false, err
			},
		},
		{
			id: "resurgence", name: "Resurgence", accent: "#b4531b",
			summary: "Dummy profile for testing. A Lord of Destruction mod since 2013, with its own realm.",
			game:    "1.13c", gateway: "gateway.resurgence.test", realm: "Resurgence",
			links: map[string]any{
				"website": "https://d2resurgence.github.io/",
				"discord": "https://discord.gg/SrbNuxyTEb",
				"wiki":    "https://diablo2resurgence.fandom.com/wiki/",
			},
			components: true,
			art: func(dir string) (map[string]asset, bool, error) {
				return thirdPartyArt(extraArt, dir, "resurgence-logo.png", "resurgence-bg.png", nil)
			},
		},
		{
			id: "diablo09", name: "Diablo 09", accent: "#b8862b",
			summary: "Dummy profile for testing. Diablo II 1.09d: the classic 2001 game with a few quality-of-life changes.",
			game:    "1.09d", gateway: "gateway.diablo09.test", realm: "Diablo 09",
			links: map[string]any{
				"website": "https://diablo09.com/",
				"discord": "https://diablo09.com/discord/",
			},
			art: func(dir string) (map[string]asset, bool, error) {
				// Their only art is a social-media card; the wordmark is cut
				// out of it for a logo.
				return thirdPartyArt(extraArt, dir, "", "", func() (image.Image, error) {
					og, err := readImage(filepath.Join(extraArt, "diablo09-og.jpg"))
					if err != nil {
						return nil, err
					}
					return crop(og, image.Rect(95, 195, 1085, 385)), nil
				})
			},
		},
	}

	version := int(time.Now().Unix())
	junk := []string{"*.txt", "*.log", "*.htm", "*.html", "*.md", "*.lnk", "*.url", "*.dmp", "Crashdump", "support", "*.bat", "bncache.dat", "BNUpdate.exe"}
	maphackFiles := []string{"BH.dll", "BH.cfg", "BH_settings.cfg"}
	d2glFiles := []string{"d2gl.mpq", "d2gl*.ini", "glide3x.dll"}

	for _, s := range servers {
		dir := filepath.Join(site, s.id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}

		// The same files split as a real server would publish them: the base
		// channel, and the maphack and d2gl as components.
		layers := []struct {
			file string
			opts pack.Options
		}{
			{"live.json", pack.Options{
				Once:    []string{"UI.ini", "SGD2FreeResolution.json", "default.key"},
				Exclude: append(append(append([]string{}, junk...), maphackFiles...), "d2gl.mpq", "d2gl*.ini"),
			}},
		}
		if s.components {
			layers = append(layers,
				struct {
					file string
					opts pack.Options
				}{"maphack.json", pack.Options{Only: maphackFiles, Once: []string{"BH.cfg", "BH_settings.cfg"}}},
				struct {
					file string
					opts pack.Options
				}{"d2gl.json", pack.Options{Only: d2glFiles, Once: []string{"d2gl*.ini"}}},
			)
		}

		for _, l := range layers {
			l.opts.Server, l.opts.Version, l.opts.BaseURL = s.id, "dev", siteURL+"/files/live"
			m, err := pack.BuildManifest(live, l.opts)
			if err != nil {
				return err
			}
			if err := writeJSON(filepath.Join(dir, l.file), m); err != nil {
				return err
			}
		}

		settings := []any{
			setting("windowed", "Windowed", "Game", "Run the game in a window.", map[string]any{"flag": "-w"}),
			setting("no-sound", "No sound", "Game", "", map[string]any{"flag": "-ns"}),
		}

		profile := map[string]any{
			"schema":  1,
			"id":      s.id,
			"name":    s.name,
			"summary": s.summary,
			"version": version,
			"links":   s.links,
			"hosts":   []string{"127.0.0.1"},
			"game":    map[string]any{"version": s.game, "expansion": true, "baseArchives": "link", "saves": "shared"},
			"gateways": []any{
				map[string]any{"name": s.name, "host": s.gateway, "timezone": 0, "realm": s.realm},
			},
			"channels": []any{
				map[string]any{"id": "live", "name": "Live", "manifest": siteURL + "/" + s.id + "/live.json"},
			},
			"launch": map[string]any{
				"defaultFlags": []string{},
				"allowedFlags": []string{"-w", "-ns", "-skiptobnet", "-direct", "-txt"},
				"maxInstances": 4,
			},
			"news":   siteURL + "/" + s.id + "/feed.json",
			"ladder": siteURL + "/" + s.id + "/ladder.json",
		}

		if s.components {
			profile["components"] = []any{
				map[string]any{"id": "maphack", "name": "Maphack", "kind": "bh", "default": "1.9.9",
					"versions": []any{map[string]any{"id": "1.9.9", "manifest": siteURL + "/" + s.id + "/maphack.json"}}},
				map[string]any{"id": "d2gl", "name": "D2GL renderer", "kind": "d2gl", "default": "1.3.3", "flags": []string{"-3dfx"},
					"versions": []any{map[string]any{"id": "1.3.3", "manifest": siteURL + "/" + s.id + "/d2gl.json"}}},
			}
			settings = append(settings,
				component(setting("reveal-map", "Reveal map", "Maphack", "", bh("Reveal Map")), "maphack"),
				component(setting("show-monsters", "Show monsters", "Maphack", "", bh("Show Monsters")), "maphack"),
				component(setting("show-missiles", "Show missiles", "Maphack", "", bh("Show Missiles")), "maphack"),
				component(setting("show-sockets", "Show sockets", "Maphack", "", bh("Show Sockets")), "maphack"),
				component(setting("show-ilvl", "Show item level", "Maphack", "", bh("Show iLvl")), "maphack"),
				component(setting("experience-meter", "Experience meter", "Maphack", "", bh("Experience Meter")), "maphack"),
				component(setting("unlock-cursor", "Unlock cursor", "D2GL", "Let the mouse leave the game window.",
					map[string]any{"file": "d2gl.ini", "format": "ini", "section": "Screen", "key": "unlock_cursor"}), "d2gl"),
				component(setting("vsync", "Vertical sync", "D2GL", "",
					map[string]any{"file": "d2gl.ini", "format": "ini", "section": "Screen", "key": "vsync"}), "d2gl"),
			)
		}
		profile["settings"] = settings

		branding := map[string]any{"accent": s.accent}
		assets, bgHasLogo, err := s.art(dir)
		if err != nil {
			return fmt.Errorf("%s art: %w", s.id, err)
		}
		for k, v := range assets {
			branding[k] = map[string]any{"url": siteURL + "/" + s.id + "/" + v.name, "sha256": v.sha}
		}
		if bgHasLogo {
			branding["backgroundHasLogo"] = true
		}
		profile["branding"] = branding

		if err := writeJSON(filepath.Join(dir, "feed.json"), feed(s.name)); err != nil {
			return err
		}
		if err := writeJSON(filepath.Join(dir, "ladder.json"), ladder()); err != nil {
			return err
		}

		data, err := json.MarshalIndent(profile, "", "  ")
		if err != nil {
			return err
		}
		if _, err := spec.ParseProfile(data); err != nil {
			return fmt.Errorf("%s profile: %w", s.id, err)
		}
		// The profile goes in the listing, as it would after its pull
		// request is merged.
		if err := os.WriteFile(filepath.Join(out, "listing", s.id+".json"), data, 0o644); err != nil {
			return err
		}
	}

	// Launcher state: the sandbox folder, with all three pinned. Kept across
	// runs so installs and choices survive, but the base path and pins are
	// always reset to the test setup.
	st, err := store.Open(filepath.Join(out, "data"))
	if err != nil {
		return err
	}
	state, err := st.Load()
	if err != nil {
		return err
	}
	abs, _ := filepath.Abs(sandbox)
	state.BasePath = abs
	state.Favourites = []string{"slashdiablo", "resurgence", "diablo09"}
	// Keep the old SlashDiablo launcher's real settings out of the test setup.
	state.LegacyImported = true
	if err := st.Save(state); err != nil {
		return err
	}

	fmt.Printf("Built %s: 3 test servers, sandbox Diablo II folder %s\n", out, abs)
	return nil
}

func setting(id, label, group, description string, target map[string]any) map[string]any {
	s := map[string]any{"id": id, "label": label, "group": group, "type": "bool", "target": target}
	if description != "" {
		s["description"] = description
	}
	return s
}

func component(s map[string]any, id string) map[string]any {
	s["component"] = id
	return s
}

func bh(key string) map[string]any {
	return map[string]any{"file": "BH_settings.cfg", "format": "bh", "key": key}
}

func feed(name string) map[string]any {
	now := time.Now().UTC()
	return map[string]any{
		"version": "https://jsonfeed.org/version/1.1",
		"title":   name,
		"items": []any{
			map[string]any{"id": "3", "title": name + " on the new launcher", "url": "https://github.com/d2org/launcher",
				"date_published": now.Format(time.RFC3339),
				"summary": "Test news served by testenv/dev.ps1, not from " + name +
					". Real servers publish a JSON Feed from their own site."},
			map[string]any{"id": "2", "title": "Ladder reset", "date_published": now.AddDate(0, 0, -9).Format(time.RFC3339),
				"summary": "A sample post without a link. Dates, titles and summaries come straight from the feed."},
			map[string]any{"id": "1", "title": "Maphack 1.9.9 released", "url": "https://github.com/planqi/slashdiablo-maphack",
				"date_published": now.AddDate(0, -1, 0).Format(time.RFC3339),
				"summary":        "The latest sanctioned maphack. Its toggles are in Settings, Maphack."},
		},
	}
}

func ladder() map[string]any {
	classes := []string{"Sorceress", "Paladin", "Barbarian", "Necromancer", "Amazon", "Druid", "Assassin"}
	names := []string{"Meanski", "Tyrael", "Deckard", "Akara", "Kashya", "Charsi", "Gheed", "Warriv", "Flavie", "Fara", "Lysander", "Elzix"}

	board := func(offset int) []any {
		var entries []any
		for i := 0; i < 40; i++ {
			e := map[string]any{
				"rank": i + 1, "name": fmt.Sprintf("%s%d", names[(i+offset)%len(names)], i+offset),
				"class": classes[(i+offset)%len(classes)], "level": 99 - i/3,
			}
			if i%9 == 4 {
				e["status"] = "dead"
			} else {
				e["status"] = "alive"
			}
			entries = append(entries, e)
		}
		return entries
	}

	return map[string]any{
		"schema": 1,
		"boards": []any{
			map[string]any{"id": "sc", "name": "Softcore", "entries": board(0)},
			map[string]any{"id": "hc", "name": "Hardcore", "entries": board(5)},
		},
	}
}

type asset struct{ name, sha string }

// slashArt turns the old launcher's art into branding images within the
// spec's limits: the background as JPEG, the two logo layers as one PNG.
func slashArt(dir, out string) (map[string]asset, error) {
	bg, err := readPNG(filepath.Join(dir, "bg.png"))
	if err != nil {
		return nil, err
	}
	logoBG, err := readPNG(filepath.Join(dir, "logo-bg.png"))
	if err != nil {
		return nil, err
	}
	logoText, err := readPNG(filepath.Join(dir, "logo-text.png"))
	if err != nil {
		return nil, err
	}

	// The old launcher drew the text 90px below the top of the emblem.
	logo := image.NewRGBA(image.Rect(0, 0, 260, 267))
	draw.Draw(logo, logoBG.Bounds().Add(image.Pt(13, 0)), logoBG, logoBG.Bounds().Min, draw.Over)
	draw.Draw(logo, logoText.Bounds().Add(image.Pt(10, 100)), logoText, logoText.Bounds().Min, draw.Over)

	result := map[string]asset{}

	bgFile := filepath.Join(out, "background.jpg")
	f, err := os.Create(bgFile)
	if err != nil {
		return nil, err
	}
	if err := jpeg.Encode(f, bg, &jpeg.Options{Quality: 82}); err != nil {
		f.Close()
		return nil, err
	}
	f.Close()

	logoFile := filepath.Join(out, "logo.png")
	f, err = os.Create(logoFile)
	if err != nil {
		return nil, err
	}
	if err := png.Encode(f, logo); err != nil {
		f.Close()
		return nil, err
	}
	f.Close()

	for key, file := range map[string]string{"background": bgFile, "logo": logoFile} {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		result[key] = asset{name: filepath.Base(file), sha: hex.EncodeToString(sum[:])}
	}

	return result, nil
}

func readPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return png.Decode(f)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// thirdPartyArt turns another server's published art into branding within
// the spec's limits. logo and background name files in dir (either may be
// empty); makeLogo, when set, builds the logo instead. It reports whether the
// background already shows the logo. Missing files mean no art, not an error,
// so the environment still builds offline.
func thirdPartyArt(dir, out, logo, background string, makeLogo func() (image.Image, error)) (map[string]asset, bool, error) {
	result := map[string]asset{}
	if dir == "" {
		return result, false, nil
	}

	var logoImg image.Image
	switch {
	case makeLogo != nil:
		img, err := makeLogo()
		if err == nil {
			logoImg = img
		}
	case logo != "":
		img, err := readImage(filepath.Join(dir, logo))
		if err == nil {
			logoImg = img
		}
	}
	if logoImg != nil {
		a, err := writeImage(filepath.Join(out, "logo.png"), logoImg, false)
		if err != nil {
			return nil, false, err
		}
		result["logo"] = a
	}

	hasBackground := false
	if background != "" {
		if img, err := readImage(filepath.Join(dir, background)); err == nil {
			a, err := writeImage(filepath.Join(out, "background.jpg"), img, true)
			if err != nil {
				return nil, false, err
			}
			result["background"] = a
			hasBackground = true
		}
	}

	// Resurgence's backgrounds all carry its logo.
	return result, hasBackground && logo != "", nil
}

// readImage decodes a PNG or JPEG.
func readImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	return img, err
}

// writeImage encodes as JPEG or PNG and returns the file's name and hash.
func writeImage(path string, img image.Image, asJPEG bool) (asset, error) {
	f, err := os.Create(path)
	if err != nil {
		return asset{}, err
	}

	if asJPEG {
		err = jpeg.Encode(f, img, &jpeg.Options{Quality: 82})
	} else {
		err = png.Encode(f, img)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return asset{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return asset{}, err
	}
	sum := sha256.Sum256(data)

	return asset{name: filepath.Base(path), sha: hex.EncodeToString(sum[:])}, nil
}

// crop cuts r out of img and fades its dark backdrop to transparent, so a
// wordmark on black sits on any background.
func crop(img image.Image, r image.Rectangle) image.Image {
	r = r.Intersect(img.Bounds())
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Bounds(), img, r.Min, draw.Src)

	for i := 0; i < len(out.Pix); i += 4 {
		px := out.Pix[i : i+4]
		bright := max(px[0], px[1], px[2])
		// Near-black goes fully clear; anything bright stays solid.
		a := (int(bright) - 40) * 4
		px[3] = uint8(min(max(a, 0), 255))
	}

	return out
}

// linkTree mirrors a folder with hard links, leaving out the base archives
// and saves.
func linkTree(src, dst string) error {
	skip := map[string]bool{}
	for _, a := range install.Archives {
		skip[a.Name] = true
	}

	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(src, path)
		lower := strings.ToLower(filepath.ToSlash(rel))
		if lower == "save" || strings.HasPrefix(lower, "save/") || skip[lower] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}

		return linkOrCopy(path, target)
	})
}

func linkOrCopy(src, dst string) error {
	if s, err := os.Stat(src); err == nil {
		if d, err := os.Stat(dst); err == nil && os.SameFile(s, d) {
			return nil
		}
	}
	os.Remove(dst)

	if err := os.Link(src, dst); err == nil {
		return nil
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}

	return out.Close()
}
