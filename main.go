package main

import (
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/diablo2org/launcher/internal/app"
	"github.com/diablo2org/launcher/internal/core"
	"github.com/diablo2org/launcher/internal/fetch"
	"github.com/diablo2org/launcher/internal/launch"
	"github.com/diablo2org/launcher/internal/store"
)

// The built frontend is embedded, so the launcher ships as a single exe.
//
//go:embed all:frontend/dist
var assets embed.FS

//go:embed examples/slashdiablo/profile.json
var exampleProfile []byte

// listingURL is the published server listing: servers/index.json on the
// repository's default branch.
const listingURL = "https://raw.githubusercontent.com/diablo2org/launcher/main/servers/index.json"

// Development settings, read from the environment. None are set in a normal
// install.
const (
	// envListing is a folder of listing entries to use instead of the
	// published listing.
	envListing = "LAUNCHER_LISTING_DIR"
	// envDevCA is a certificate to trust, for cmd/devserve.
	envDevCA = "LAUNCHER_DEV_CA"
	// envData replaces the data folder, to keep test runs separate.
	envData = "LAUNCHER_DATA_DIR"
)

func init() {
	application.RegisterEvent[app.UpdateProgress]("update:progress")
}

func main() {
	dataDir := os.Getenv(envData)
	if dataDir == "" {
		dir, err := store.DefaultDir()
		if err != nil {
			log.Fatal(err)
		}
		dataDir = dir
	}

	st, err := store.Open(dataDir)
	if err != nil {
		log.Fatal(err)
	}

	var clientOpts []fetch.Option
	if ca := os.Getenv(envDevCA); ca != "" {
		trust, err := fetch.TrustCertificate(ca)
		if err != nil {
			log.Fatal(err)
		}
		clientOpts = append(clientOpts, trust)
	}

	var listing core.Listing = core.RemoteListing{
		URL:    listingURL,
		Client: fetch.New([]string{"raw.githubusercontent.com"}, clientOpts...),
		Store:  st,
	}
	if dir := os.Getenv(envListing); dir != "" {
		listing = core.DirListing(dir)
	}

	manager, err := core.New(st, listing, launch.New(launch.SystemRegistry{}, launch.StartProcess), clientOpts...)
	if err != nil {
		log.Fatal(err)
	}

	var a *application.App
	host := app.Host{
		Updates:    fetch.New([]string{"api.github.com"}),
		OpenFolder: func(path string) error { return a.Env.OpenFileManager(path, false) },
		Emit:       func(name string, data any) { a.Event.Emit(name, data) },
		PickFolder: func(title string) (string, error) {
			return a.Dialog.OpenFile().
				SetTitle(title).
				CanChooseDirectories(true).
				CanChooseFiles(false).
				PromptForSingleSelection()
		},
	}

	a = application.New(application.Options{
		Name:        "Launcher",
		Description: "Launcher for Diablo II private servers",
		Services: []application.Service{
			application.NewService(app.NewServerService(manager, host)),
			application.NewService(app.NewProfileService(exampleProfile)),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	a.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "Launcher",
		// The old launcher's 1024x600, plus the server rail.
		Width:     1100,
		Height:    640,
		MinWidth:  1000,
		MinHeight: 600,
		// The top bar draws its own controls and drags the window.
		Frameless:        true,
		BackgroundColour: application.NewRGB(10, 10, 13),
		// The top bar is a native title bar (CSS app-region), so Windows
		// gives it double-click to maximise, the window menu and snapping.
		Windows: application.WindowsWindow{NonClientRegionSupport: true},
		URL:     "/",
	})

	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}
