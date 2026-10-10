package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"log"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/diablo2org/launcher/internal/app"
	"github.com/diablo2org/launcher/internal/core"
	"github.com/diablo2org/launcher/internal/fetch"
	"github.com/diablo2org/launcher/internal/launch"
	"github.com/diablo2org/launcher/internal/logs"
	"github.com/diablo2org/launcher/internal/store"
	"github.com/diablo2org/launcher/internal/updates"
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
	// envUpdates is a server to check for launcher updates instead of
	// GitHub; see updates.UseSource.
	envUpdates = "LAUNCHER_UPDATES_URL"
	// envData replaces the data folder, to keep test runs separate.
	envData = "LAUNCHER_DATA_DIR"
	// envDevTools opens WebView2's DevTools protocol on this port, so tests
	// can drive the page.
	envDevTools = "LAUNCHER_DEVTOOLS_PORT"
)

func init() {
	application.RegisterEvent[app.UpdateProgress]("update:progress")
	application.RegisterEvent[app.LauncherUpdateProgress]("launcher:update")
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

	// Without a log file the launcher still runs; it just can't help with a
	// bug report.
	if closeLog, err := logs.Setup(dataDir, app.Version); err == nil {
		defer closeLog()
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
	var window *application.WebviewWindow
	host := app.Host{
		Updates: updatesClient(clientOpts),
		// The installer runs on its own, shown to the player, and replaces
		// this launcher once it has closed.
		UpdatesDir:   filepath.Join(dataDir, "updates"),
		RunInstaller: func(path string) error { return exec.Command(path).Start() },
		// Quit from a frontend call returns first, so the call completes.
		Quit:       func() { go func() { time.Sleep(300 * time.Millisecond); a.Quit() }() },
		OpenFolder: func(path string) error { return a.Env.OpenFileManager(path, false) },
		ShowFile:   func(path string) error { return a.Env.OpenFileManager(path, true) },
		SaveFile: func(title, name string) (string, error) {
			return a.Dialog.SaveFile().
				SetMessage(title).
				SetFilename(name).
				AddFilter("Zip files", "*.zip").
				PromptForSingleSelection()
		},
		Emit: func(name string, data any) { a.Event.Emit(name, data) },
		Focus: func() {
			if window != nil {
				window.UnMinimise()
				window.Show()
				window.Focus()
			}
		},
		PickFolder: func(title string) (string, error) {
			return a.Dialog.OpenFile().
				SetTitle(title).
				CanChooseDirectories(true).
				CanChooseFiles(false).
				PromptForSingleSelection()
		},
	}

	servers := app.NewServerService(manager, host)

	a = application.New(application.Options{
		Name:        "Launcher",
		Description: "Launcher for Diablo II private servers",
		Services: []application.Service{
			application.NewService(servers),
			application.NewService(app.NewProfileService(exampleProfile)),
			application.NewService(app.NewSupportService(st, manager, host)),
		},
		// Wails' own problems go to the log file too; its routine messages,
		// such as every asset served, don't.
		Logger:  slog.New(logs.AtLeast(slog.Default().Handler(), slog.LevelWarn)),
		Windows: windowsOptions(),
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		// A second launch, such as a clicked diablo2org:// link while the
		// launcher is open, hands its arguments to this one and exits.
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: instanceID(),
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				if link := app.LinkArg(data.Args); link != "" {
					app.HandleLink(servers, link, true)
				} else {
					host.Focus()
				}
			},
		},
	})

	window = a.Window.NewWithOptions(application.WebviewWindowOptions{
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

	// A link the launcher was started with waits for the frontend to load.
	// macOS hands links over as an event instead; on Windows that event
	// repeats the argument, so it's only used on macOS.
	app.HandleLink(servers, app.LinkArg(os.Args[1:]), false)
	if runtime.GOOS == "darwin" {
		a.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(e *application.ApplicationEvent) {
			app.HandleLink(servers, e.Context().URL(), true)
		})
	}

	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}

// instanceID names the running launcher for single-instance. A test run with
// its own data folder gets its own, so it can run beside the player's.
func instanceID() string {
	const id = "org.diablo2.launcher"

	dir := os.Getenv(envData)
	if dir == "" {
		return id
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	// Windows and macOS paths are case-insensitive; elsewhere /tmp/Game and
	// /tmp/game are different folders.
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		dir = strings.ToLower(dir)
	}
	sum := sha256.Sum256([]byte(dir))

	return id + "." + hex.EncodeToString(sum[:4])
}

// updatesClient fetches launcher releases from GitHub, or from the server in
// LAUNCHER_UPDATES_URL.
func updatesClient(opts []fetch.Option) *fetch.Client {
	base := os.Getenv(envUpdates)
	if base == "" {
		return fetch.New(updates.Hosts)
	}

	u, err := url.Parse(base)
	if err != nil || u.Hostname() == "" {
		log.Fatalf("%s: %q is not a URL", envUpdates, base)
	}
	updates.UseSource(base)

	return fetch.New([]string{u.Hostname()}, opts...)
}

func windowsOptions() application.WindowsOptions {
	var o application.WindowsOptions
	if v := os.Getenv(envDevTools); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port >= 1 && port <= 65535 {
			o.AdditionalBrowserArgs = []string{"--remote-debugging-port=" + strconv.Itoa(port)}
		} else {
			slog.Warn("ignoring "+envDevTools+": not a port number", "value", v)
		}
	}

	return o
}
