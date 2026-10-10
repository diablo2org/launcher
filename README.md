# launcher

An open-source launcher for Diablo II: Lord of Destruction private servers.

Each server is described by a JSON profile: name, branding, gateways, where
its game files come from, the optional components and settings it offers,
and how the game starts. Servers get listed by a reviewed pull request adding
their profile to [`servers/`](servers/). Players pin up to three
favourite servers and switch between them with a click or Ctrl+1, 2 and 3.
Each server is installed into its own folder inside the player's Diablo II
install, which is otherwise never modified.

- [Server profile specification](docs/SPEC.md)
- [JSON Schemas](schema/)
- [Example profile and manifest](examples/)
- [Server listing](servers/), and how to get a server listed

## Status

Working end to end against a local test server: install, update, components
(maphack, D2GL), declarative settings, news, ladder, multibox with D2GL box
profiles, pinned servers, adding servers by URL, and bringing across the old
SlashDiablo launcher's settings. Not yet tried against a real server, and
Play has only been tested in unit tests so far.

| Phase | |
|---|---|
| 1. Spec and skeleton | Done: spec draft 0.1, schemas, CI |
| 2. SlashDiablo on the new launcher | Done locally; needs SlashDiablo to publish manifests and add its profile |
| 3. Multi-server | Done: listing, catalog, pinning, per-server branding, add by URL |
| 4. Hardening | Update check, tagged releases, code signing and bug reports done |
| 5. Reach | Other servers, Linux |

## For server teams

Your files stay on your own web host. You publish a manifest next to them,
write a profile, and open a pull request. [`servers/README.md`](servers/README.md)
walks through it; in short:

```
d2pack manifest -server myserver -version 2026.10.01 -url https://files.example.com/live ./live
d2pack init -id myserver -name "My Server" -gateway play.example.com -manifest https://files.example.com/live/manifest.json
d2pack check -profile myserver.json ./live/manifest.json
```

Blizzard's base archives are always left out of a manifest. `-once` marks
files the player owns after the first install, such as maphack settings, and
`-only` builds a component's manifest from a full install. When you patch,
rebuild and re-upload the manifest; no pull request is needed.

With components, `d2pack build <plan.json>` builds every channel and
component version's manifest in one go, ready to upload. See
[`examples/slashdiablo/build.json`](examples/slashdiablo/build.json).

## Development

You need Go 1.25 or newer, Node 24, and the Wails CLI at the version CI uses:

```
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.26
cd frontend && npm install && cd ..
```

| Command | What it does |
|---|---|
| `wails3 dev` | Run with live reload |
| `wails3 build` | Build `bin/launcher.exe` |
| `wails3 package INSTALL_SCOPE=user` | Build a per-user installer, no admin needed |
| `go test ./internal/...` | Run the tests, including every example and listed profile |
| `go run ./cmd/listing` | Regenerate `servers/index.json` after changing `servers/` |

### Releases and code signing

Pushing a `v1.2.3` tag builds a release: the installer, `d2pack.exe` and
`SHA256SUMS.txt` go on a GitHub release. Tagged builds are code signed
through SSL.com's eSigner, so Windows names the publisher rather than
warning about an unknown one. NSIS signs the launcher, its uninstaller and
the installer as it builds them, and CI signs `d2pack.exe` and checks every
signature before anything is published. Other builds aren't signed, since
each signing counts against the eSigner plan (about four per release).

Signing needs four repository secrets: `ES_USERNAME` and `ES_PASSWORD` (the
SSL.com account), `ES_CREDENTIAL_ID` (the certificate's eSigner credential)
and `ES_TOTP_SECRET` (the eSigner authenticator-app secret).
[`build/windows/sign.ps1`](build/windows/sign.ps1) does the signing, and can
be run by hand on Windows with the same four set as environment variables.

### Local test environment

`testenv/dev.ps1` builds three test servers from a working
SlashDiablo install, serves them over HTTPS on 127.0.0.1:8667, and starts
the launcher against them with its own data folder and a sandbox Diablo II
folder made of hard links to your archives:

```
.\testenv\dev.ps1 -Source "C:\Games\Diablo II" -Art ..\slashdiablo-launcher\qml\assets
```

`-Art` is optional: the old launcher's `qml\assets` folder, for SlashDiablo's
key art. Your Diablo II folder is only read. Pressing Play does write the
Battle.net registry values, as a real launch would.

Setting `LAUNCHER_DEVTOOLS_PORT` opens WebView2's DevTools protocol on that
port, so a script can drive and screenshot the page.

### Logs and bug reports

The launcher logs to `logs\launcher.log` in its data folder
(`%LOCALAPPDATA%\diablo2org\launcher`), keeping three older files of up to
1 MB each, and writes any crash to `logs\crash.log`. Settings, General,
Bug report shows players what a report holds, then saves it as a zip:
those logs, `state.json`, and the newest Diablo II crash logs (`D2*.txt`)
from each server folder, with the user folder replaced by `%USERPROFILE%`.
Nothing is uploaded.

### Layout

- `internal/spec`: parses and checks profiles, manifests and listing
  entries against the schemas in `schema/`, plus the rules a schema can't
  express.
- `internal/fetch`: the only code that touches the network. HTTPS to allowed
  hosts only, redirects included; status, size and hashes checked.
- `internal/sync`: brings a server folder in line with a manifest, never
  writing a file in place.
- `internal/install`: the base install, server folders and hard-linked
  archives.
- `internal/settings`: surgical edits to ini and BH settings files.
- `internal/launch`: Battle.net registry values and starting boxes.
- `internal/logs`, `internal/report`: the log file, crash capture and bug
  reports.
- `internal/core`: everything the launcher does, independent of the UI.
- `internal/app`: the thin services the frontend calls.
- `frontend/`: Svelte 5, TypeScript and Tailwind.
- `cmd/d2pack`, `cmd/listing`: tools for server teams and maintainers.
- `cmd/devsite`, `cmd/devserve`: the local test environment.

## Licence

[MIT](LICENSE)
