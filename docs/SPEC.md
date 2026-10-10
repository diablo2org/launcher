# Server profile specification

Status: **draft 0.1**. Written with Resurgence; expect breaking changes until 1.0.

This document defines how a Diablo II private server describes itself to the
launcher: who it is, how it looks, where its game files come from, which
optional components and settings it offers, and how the game is started.

The launcher runs **no code** from a server. Everything below is declarative
data. The only executable content a server delivers is the game files it
ships, which run inside the game exactly as they would with any other
launcher.

The key words MUST, SHOULD and MAY are used as in RFC 2119.

## 1. Overview

How a server gets into the launcher:

1. **The server keeps its files on its own web host**, over HTTPS.
2. **It publishes a file manifest** next to them: every file with its size
   and SHA-256, built with `d2pack manifest`. When it patches, it rebuilds
   and re-uploads the manifest. No pull request is needed for that.
3. **It writes a server profile**: name, branding, gateways, where its
   manifests are, and optionally components, settings, news and a ladder.
   `d2pack init` writes a starter one.
4. **It opens a pull request** adding the profile to `servers/` in the
   launcher repository.
5. **The maintainers review and merge it**, and the server appears in every
   launcher.

| Document | Where it lives | Changed by |
|---|---|---|
| **Server profile** | `servers/<id>.json` in the launcher repository | Pull request |
| **File manifest** (`manifest.json`) | The server's web host | The server, whenever it patches |
| **News feed**, **ladder** | The server's web host | The server |

Because the profile is reviewed, everything in it is approved: the gateway,
which hosts files may come from, and which components exist. A compromised
web host cannot redirect players elsewhere. What it could still do is serve
different game files under the approved manifest URLs, which is the same
trust every server's own launcher relies on today.

JSON Schemas for the profile and the manifest are in [`schema/`](../schema).

## 2. Install model

Every server gets its own folder under the player's Diablo II install:

```
<Diablo II>\                  base game: d2data.mpq, d2exp.mpq, ... (never modified)
<Diablo II>\<server id>\      everything the server ships, plus links to the base archives if needed
```

- The launcher **never writes to the base install**, apart from creating the
  server folders inside it.
- Each server ships a **complete executable set** for its game version
  (`Game.exe`, every `D2*.dll`, `Storm.dll`, `Fog.dll`, `patch_d2.mpq`, ...),
  so any retail version works as the base. The launcher does not detect or
  convert the base version.
- The base archives (`d2char`, `d2data`, `d2exp`, `d2music`, `d2sfx`,
  `d2speech`, `d2video`, `d2xmusic`, `d2xtalk`, `d2xvideo`) are **never**
  hosted by a server or the launcher. Their presence in the base folder is
  the ownership check.

### 2.1 Finding the base archives

The game only looks for archives in its working folder and its own folder,
never the parent. A profile declares how its game finds them with
`game.baseArchives`:

| Value | Meaning |
|---|---|
| `parent` | The server's own game code redirects archive loading to `..\` (as Project Diablo 2 and Path of Diablo do). The launcher does nothing extra. |
| `link` | The launcher places the base archives in the server folder as **hard links**. This needs no admin rights and no extra disk space, but only works when the server folder is on the same NTFS volume as the base install. Otherwise the launcher offers to copy them, stating the size first. |

Tested on 1.13c with SlashDiablo's files: started from a server folder
holding hard links to all ten base archives, the game reaches the main menu
in expansion mode with BH loaded.

The launcher never writes into an existing file in place. Updates are
written to a temporary file and renamed over the old one, so a hard link is
replaced rather than written through, and the retail archive it points to
can't be changed.

## 3. Server profile

```jsonc
{
  "schema": 1,
  "id": "slashdiablo",
  "name": "SlashDiablo",
  "summary": "Classic 1.13c ladder with a light touch.",
  "version": 3,
  "minLauncher": "0.1.0",

  "links": { "website": "https://slashdiablo.net", "discord": "https://discord.gg/..." },

  "branding": {
    "logo":       { "url": "https://.../logo.webp",       "sha256": "..." },
    "background": { "url": "https://.../background.webp", "sha256": "..." },
    "accent": "#8f3131"
  },

  "hosts": ["files.slashdiablo.net"],

  "game": {
    "version": "1.13c",
    "expansion": true,
    "baseArchives": "link",
    "saves": "shared"
  },

  "gateways": [
    { "name": "SlashDiablo", "host": "play.slashdiablo.net", "timezone": 0, "realm": "Slash Diablo" }
  ],

  "channels": [
    { "id": "live", "name": "Live", "manifest": "https://files.slashdiablo.net/live/manifest.json" },
    { "id": "beta", "name": "PTR",  "manifest": "https://files.slashdiablo.net/ptr/manifest.json" }
  ],

  "components": [ ... ],   // section 5
  "settings":   [ ... ],   // section 6
  "launch":     { ... },   // section 7
  "news":   "https://slashdiablo.net/feed.json",
  "ladder": "https://slashdiablo.net/ladder.json"
}
```

### 3.1 Fields

- **`schema`** (required): profile format version. This document is `1`.
- **`id`** (required): lowercase letters, digits and `-`, 2 to 32 characters.
  It is also the server's folder name, so it MUST NOT change once published.
- **`name`**, **`summary`**: shown in the catalog and on the server tab.
- **`version`** (required): an integer the server increments on every
  change. For a server added by URL (9.1), the launcher refuses a profile
  older than one it has already seen, so an old profile can't be replayed.
- **`minLauncher`**: the oldest launcher version that understands this
  profile. Older launchers show "update the launcher to play".
- **`links`**: any of `website`, `discord`, `forum`, `wiki`, `trade`,
  `register`, `support`, `donate`, each an HTTPS URL. Every link given is a
  button on the server's launch page, in that order, and opens in the
  player's browser; hovering one shows where it goes. The launcher's
  Community tab opens `discord`, else `forum`, else `website`.
- **`branding`**:
  - `logo`: PNG or WebP, at most 512 KB, square or wide.
  - `background`: JPEG or WebP, at most 3 MB, 16:9, at least 1280x720.
  - `accent`: a `#rrggbb` colour used for buttons and highlights. The launcher
    MAY adjust it for contrast.
  - `backgroundHasLogo`: set when the background art already shows the logo,
    so the launch screen doesn't draw it a second time. The logo is still
    used elsewhere, such as the server rail.
  - Images are cached, and verified when `sha256` is given. Servers cannot
    supply HTML, CSS, fonts or scripts; the layout is the same for every
    server.
- **`hosts`** (required): the only hosts the launcher will download files
  from for this server. Every URL in the profile and its manifests MUST be
  HTTPS on one of these hosts, except `links`, which open in the browser.
  Redirects are checked against the same list. Files published as GitHub
  release assets redirect from `github.com` to
  `release-assets.githubusercontent.com` (formerly
  `objects.githubusercontent.com`), so list those too; `d2pack init` does
  when the manifest URL is on `github.com`.
- **`game.version`**: `1.07`, `1.08`, `1.09b`, `1.09d`, `1.10f`, `1.11b`,
  `1.12a`, `1.13c`, `1.13d` or `1.14d`. Informational; used for display and
  for launcher features that only work on some versions. The subfolder
  install has been tested on 1.13c only.
- **`game.expansion`**: when true, `d2exp.mpq` is required in the base.
- **`game.saves`**: `shared` (default) keeps the retail `Save\` folder;
  `isolated` gives the server `<server folder>\Save\`, set through the
  registry `Save Path` at launch. Realm characters live on the server either
  way; this only matters for single player and open Battle.net.
- **`gateways`**: at least one. `realm` is the realm name written to
  `Preferred Realm`, which the game uses to pick the realm on login.
- **`channels`**: at least one. The first is the default. Players can switch
  channel per server.
- **`news`**: a [JSON Feed 1.1](https://www.jsonfeed.org/version/1.1/). See
  3.2.
- **`ladder`**: a ladder document. See 3.3.

### 3.2 News

The launcher shows the newest 20 items of the feed. From each item it uses
`title`, `summary` (or `content_text` when there is no summary),
`date_published` and `url`. HTML content is never rendered, and `url` is
only offered as a link when it is `https`; it opens in the player's browser.
Items without a title are skipped.

### 3.3 Ladder

```jsonc
{
  "schema": 1,
  "boards": [
    {
      "id": "sc-ladder",
      "name": "Softcore",
      "entries": [
        { "rank": 1, "name": "Meanski", "class": "Sorceress", "level": 99, "title": "Queen", "status": "alive" }
      ]
    }
  ]
}
```

- **`boards`**: one per ranking, such as softcore, hardcore or expansion
  only. The launcher shows the first 200 entries of each.
- **`title`** and **`status`** are optional. `status` is free text such as
  `alive` or `dead`, shown as given.

Both are fetched only from the profile's `hosts`, and hold nothing the
launcher acts on beyond showing them.

## 4. File manifest

```jsonc
{
  "schema": 1,
  "server": "slashdiablo",
  "version": "2026.09.29",
  "files": [
    {
      "path": "Game.exe",
      "size": 69632,
      "sha256": "…",
      "urls": ["https://files.slashdiablo.net/live/Game.exe"]
    },
    {
      "path": "BH_settings.cfg",
      "size": 5120,
      "sha256": "…",
      "urls": ["https://files.slashdiablo.net/live/BH_settings.cfg"],
      "mode": "once"
    },
    { "path": "old.dll", "mode": "delete" }
  ]
}
```

- **`path`**: relative to the server folder, forward slashes. It MUST NOT be
  absolute, contain `..`, a drive letter, `:` (alternate data streams) or a
  Windows reserved name (`CON`, `NUL`, `COM1`, ...). Paths are compared
  case-insensitively and MUST be unique.
- **`size`**, **`sha256`**: required unless `mode` is `delete`. The launcher
  checks both before a file is moved into place.
- **`urls`**: one or more HTTPS mirrors, tried in order, each on a host from
  the profile's `hosts`.
- **`mode`**:
  - `replace` (default): the file must match; it is downloaded whenever it is
    missing or differs.
  - `once`: downloaded only when missing. For files the player owns
    afterwards, such as maphack or renderer settings.
  - `delete`: removed if present.

### 4.1 How the launcher applies a manifest

1. Download the manifest from one of the profile's hosts. The last copy is
   kept, so an installed server still plays offline.
2. For each file, compare size and SHA-256 with what is on disk.
3. Download each needed file to a temporary name in the same folder. Check
   the HTTP status before writing, then check size and hash.
4. Rename into place. A file in use (the game is running) stops the update
   with a clear message rather than failing half way.

Each file's hash is remembered with its size and modification time, so
unchanged files aren't rehashed on every start. A file modified in the last
two seconds is always rehashed, since file times are too coarse to trust
that soon. The player can also verify a server's files, which hashes every
file again regardless, and then repairs any that differ the same way an
update would. `once` files and custom components are never verified.

Files in the server folder that no manifest lists are left alone, apart from
component switching (section 5).

## 5. Components

Components are optional layers the player turns on per server: a maphack, a
renderer, an HD mod. Each version of a component has its own manifest.

```jsonc
"components": [
  {
    "id": "maphack",
    "name": "Maphack",
    "kind": "bh",
    "default": "1.9.9",
    "versions": [
      { "id": "1.9.9", "manifest": "https://files.slashdiablo.net/maphack/1.9.9/manifest.json" },
      { "id": "1.9.8", "manifest": "https://files.slashdiablo.net/maphack/1.9.8/manifest.json" }
    ]
  },
  {
    "id": "d2gl",
    "name": "D2GL renderer",
    "kind": "d2gl",
    "versions": [ { "id": "1.3.3", "manifest": "…" } ],
    "flags": ["-3dfx"],
    "conflicts": ["hd"]
  }
]
```

- A component with no `default` is off until the player picks a version.
- **Switching or turning off** a component deletes the files listed by the
  version being removed, except `once` files and files another layer still
  lists, then applies the new version. When the launcher has no record of
  installing the component (for example files from an older layout), it
  only deletes files that match one of the component's versions exactly,
  by size and SHA-256, so an edited file is never removed on a guess.
- **Custom**: every component also offers "Custom", for players running
  their own files, such as a maphack build they're testing. With Custom
  chosen, the launcher doesn't install, update, repair or remove any file a
  version of that component lists, even where another layer lists the same
  path, while the component's settings and `flags` still apply. The launcher
  warns that custom files aren't supported by the server and are used at the
  player's own risk. Choosing a version again puts the server's files back.
  `custom` is reserved and can't be used as a version id.
- **Layering**: the channel's manifest and each component's are combined in
  order: the channel first, then components in the order the profile lists
  them. When two list the same path, the later one wins. A renderer that
  ships its own `glide3x.dll` replaces the channel's while it is on, and the
  channel's comes back when it is turned off.
- **`flags`**: launch flags the component needs, added automatically while it
  is on.
- **`conflicts`**: component ids that cannot be on at the same time.
- **`kind`** lets the launcher offer extra built-in behaviour for well-known
  components:
  - `d2gl`: per-box renderer profiles for multiboxing. With them on, the
    first box starts with `-config main` and the rest with `-config loader`,
    and before launch the launcher writes the player's chosen window size
    into `d2gl_main.ini` and `d2gl_loader.ini` (`[Screen]` `window_width`,
    `window_height`, `fullscreen=false`). A profile that doesn't exist yet
    starts as a copy of `d2gl.ini`.
  - `bh`: nothing extra yet; its settings are declared like any other.
  - Any other value, or none, means no extra behaviour.

## 6. Settings

Settings are declared by the server and shown in the launcher's settings
page for that server. They write to files inside the server folder only.

```jsonc
"settings": [
  {
    "id": "reveal-map",
    "label": "Reveal map",
    "group": "Maphack",
    "component": "maphack",
    "type": "bool",
    "target": { "file": "BH_settings.cfg", "format": "bh", "key": "Reveal Map" }
  },
  {
    "id": "unlock-cursor",
    "label": "Unlock cursor",
    "description": "Let the mouse leave the game window.",
    "group": "D2GL",
    "component": "d2gl",
    "type": "bool",
    "default": false,
    "target": { "file": "d2gl.ini", "format": "ini", "section": "Screen", "key": "unlock_cursor" }
  },
  {
    "id": "windowed",
    "label": "Windowed",
    "group": "Game",
    "type": "bool",
    "target": { "flag": "-w" }
  }
]
```

- **`group`**: the tab the setting appears on, such as `Maphack` or `D2GL`.
  Settings in a group called `Game` (or `General`) appear on the launcher's
  own Game tab, under the component and box options.
- **`type`**: `bool`, `choice` (with `options`), `int` (with `min`/`max`) or
  `string` (with `maxLength`).
- **`component`**: the setting is only shown, and only written, while that
  component is on.
- **`target`**, one of:
  - a file key: `file` (a path under the same rules as manifest paths),
    `format` and `key`, plus `section` for `ini`.
  - a launch flag: `flag`, for `bool` only. The flag MUST be one of
    `launch.allowedFlags`.
- **`format`**:
  - `ini`: `key=value` lines under `[section]` headers; `;` comments.
  - `bh`: `Key: Value` lines with `//` comments. For `bool`, only the leading
    `True`/`False` is replaced, so a hotkey after it survives.
- **Editing rules**: the launcher changes only the value of the declared key,
  keeping every other line, comment and alignment. It never creates the file:
  if it is missing, the setting is shown as unavailable until the file is
  installed (a missing file usually means the component has not been
  installed yet, and creating one could stop a `once` file ever arriving).
- **Where values live**: file settings are read from and written to the file
  itself, so changes made in game or by hand show up in the launcher. Only
  flag settings are stored by the launcher.

## 7. Launch

```jsonc
"launch": {
  "exe": "Game.exe",
  "defaultFlags": ["-skiptobnet"],
  "allowedFlags": ["-w", "-3dfx", "-skip", "-skiptobnet", "-ns", "-nofixaspect", "-direct", "-txt"],
  "maxInstances": 8
}
```

- **`exe`**: path of the executable inside the server folder. Default
  `Game.exe`.
- **`defaultFlags`**: always passed.
- **`allowedFlags`**: flags a player may toggle, as settings or on the
  advanced page. Anything else is refused, so a profile cannot smuggle in
  arguments the player did not see.
- **`maxInstances`**: upper limit for multiboxing. Default 1.
- **`setGateways`**: `false` for a server whose own game code sets the
  gateway; the launcher then writes no Battle.net values (section 8).
  Default `true`.

The launcher starts the game like this:

1. It takes a global launch lock, so two servers never launch at the same time.
2. Unless `setGateways` is `false`, it writes the server's gateways to the
   registry (section 8).
3. It sets `Save Path` when the server needs it (section 8).
4. It starts `exe` with the working folder set to the server folder.
5. It waits the player's launch delay before the next box, then releases the lock.

The launcher does not need, and does not request, administrator rights.

## 8. Registry

Diablo II reads its Battle.net settings from the current user's registry.
These are the only values the launcher writes:

| Key | Value | Type | Written |
|---|---|---|---|
| `HKCU\Software\Battle.net\Configuration` | `Diablo II Battle.net Gateways` | `REG_MULTI_SZ` | The server's gateways, merged into the player's list. Not with `launch.setGateways: false` |
| `HKCU\Software\Blizzard Entertainment\Diablo II` | `BNETIP` | `REG_SZ` | The first gateway's host. Not with `launch.setGateways: false` |
| `HKCU\Software\Blizzard Entertainment\Diablo II` | `Preferred Realm` | `REG_SZ` | The first gateway's `realm`, when set. Not with `launch.setGateways: false` |
| `HKCU\Software\Blizzard Entertainment\Diablo II` | `Save Path` | `REG_SZ` | `<server folder>\Save\` with `game.saves: isolated`; otherwise `<base>\Save\`, only when needed (below) |

An isolated server writes `Save Path` on every launch. A shared server writes
`<base>\Save\` only when `Save Path` is unset or still holds an isolated
server's folder (`<base>\<server id>\Save\`). So an isolated server's folder
never stays set for the next server launched, and a `Save Path` the player
chose themselves is kept.

The gateway list is a multi-string: a header entry, the selected gateway as
a two-digit index starting at `01`, then a `host`, `timezone`, `name` triple
per gateway. The launcher keeps the gateways already in the list, replaces
any entry with the same host as one of the server's, appends the server's
gateways and selects the first of them. The existing header is kept; `1001`
is written when there is none. Tested on 1.13c: a list written this way with
header `1001` is read and the selected gateway shown in the main menu.

A server's own game code may set the gateway itself. SlashDiablo's
`SlashDiablo.dll` does, so for SlashDiablo the registry list has no visible
effect. The launcher writes it unless the profile sets
`launch.setGateways: false`, so servers without such code need nothing extra,
and servers with it can leave the player's Battle.net settings untouched.

## 9. Server listing

The listing is the set of servers shown in the launcher's catalog. It lives
in this repository under [`servers/`](../servers): each server's full
profile, as `servers/<id>.json`, added and changed by pull request.

- The file name MUST match the profile's `id`.
- A maintainer reviews and merges. Listed servers show as verified.
- `servers/index.json` holds every profile in one file, so the launcher
  needs one request for the whole listing. It is generated with
  `go run ./cmd/listing`, and CI fails when it is out of date or when any
  profile is invalid.
- The launcher fetches `servers/index.json` from this repository's default
  branch on `raw.githubusercontent.com`, so a server can be listed, changed
  or delisted without a launcher release. The last copy is kept for offline
  use, and a profile that fails its schema is shown with its error rather
  than hiding the rest.
- Routine patches need no pull request: the server rebuilds and re-uploads
  its manifests. A pull request is only needed to change the profile itself,
  such as adding a component version or changing the gateway.

### 9.1 Servers added by URL

A player MAY add a server that is not listed, for example a new server, a
test server, or a server's own development build, by pasting the URL of its
profile. The profile is then hosted by the server itself, over HTTPS.

- Such servers show as not verified: nobody has reviewed them.
- The launcher refuses a later profile with a lower `version` than one it
  has already seen.
- A server added by URL with the same `id` as a listed server is refused;
  the listed one is used.
- Removing a server added by URL forgets it; its folder is left alone.

## 10. Launcher-side storage

Not part of the server contract, recorded here so server staff know what the
launcher keeps:

- `%LOCALAPPDATA%\<launcher>\` holds launcher settings, the listing and
  manifest caches, cached images and logs.
- Per server it stores: the chosen channel, component versions, flag
  settings, instance count and d2gl box profiles.
- It stores nothing inside the base install, and nothing inside the server
  folder except what manifests and file settings write there.

## 11. Open questions

1. Loot filters: PD2 offers filters from several authors. Worth a `filters`
   section so any server can offer the same?
2. Self-update and code signing for the launcher itself: out of scope for
   this document.
3. Linux: a Flatpak build that runs the game through Wine needs the registry
   writes to go into the Wine prefix instead.
4. Optional signing: a server could add a public key to its profile and sign
   its manifests, so a compromised web host couldn't swap game files. Left
   out until a server asks for it.
