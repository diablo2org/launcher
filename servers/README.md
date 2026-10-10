# Server listing

Every server the launcher shows as verified has its profile here, as
`servers/<id>.json`. See [the spec](../docs/SPEC.md) for everything a
profile can hold.

## Adding your server

You need `d2pack` from the [latest release](https://github.com/diablo2org/launcher/releases).

1. **Build a manifest from your game files.** Point `d2pack` at the folder
   of files players need (your patched `Game.exe`, DLLs, `patch_d2.mpq` and
   so on) and the URL you'll upload them to:

   ```
   d2pack manifest -server myserver -version 2026.10.01 -url https://files.myserver.net/live ./live
   ```

   Blizzard's base archives (`d2data.mpq` and the rest) are always left out;
   players' own copies are used. Mark files players customise with `-once`,
   for example `-once BH.cfg`, so an update never overwrites them.

2. **Upload** the folder, including the new `manifest.json`, to that URL.

3. **Write your profile:**

   ```
   d2pack init -id myserver -name "My Server" -gateway play.myserver.net -manifest https://files.myserver.net/live/manifest.json
   ```

   Then add your summary, links, logo and background, and anything else you
   want from the spec (components such as a maphack, settings, news, a
   ladder). Check it with:

   ```
   d2pack check -profile myserver.json ./live/manifest.json
   ```

   The launcher's Profile checker (Settings, General) does the same check.

4. **Open a pull request** adding `servers/myserver.json`, and run
   `go run ./cmd/listing` to update `servers/index.json` (or ask a
   maintainer to). A maintainer confirms it's really your server, then
   merges, and your server appears in every launcher.

## Building everything at once

Once you have components, such as several maphack versions and a renderer,
each needs its own manifest. `d2pack build` builds them all from a plan file
kept next to your profile:

```json
{
  "profile": "myserver.json",
  "manifests": [
    { "channel": "live", "source": ["patches/base", "patches/current"] },
    { "component": "maphack", "version": "1.9.9", "source": "patches/maphack", "once": ["BH_settings.cfg"] }
  ]
}
```

```
d2pack build build.json
```

The manifest URLs come from the profile, and each manifest's files are
published in the same folder as it. The output, `upload/<version>` by
default, mirrors those URLs: upload what's in its host folder to the root of
that host. Every manifest is checked against the profile, and a failed build
leaves nothing behind.

Each entry takes:

- `channel`, or `component` and `version`: which manifest to build.
- `source`: a folder, or a list of folders layered in order, where a later
  folder's file replaces an earlier one's. Paths are relative to the plan.
  A `source` at the top of the plan is used by entries without their own.
- `once`, `exclude`, `only`: as for `d2pack manifest`. An `exclude` at the
  top of the plan applies to every entry.
- `files`: the URL the files are published at, when that isn't the
  manifest's folder.

A folder in `source` can also be `{ "folder": ..., "files": ... }`, which
publishes that folder's files at their own URL rather than the entry's. Use
it for files several manifests share, such as the base game files under
every channel, so they're uploaded once:

```json
{
  "profile": "myserver.json",
  "manifests": [
    { "channel": "live", "source": [{ "folder": "patches/base", "files": "https://files.myserver.net/base/" }, "patches/live"] },
    { "channel": "beta", "source": [{ "folder": "patches/base", "files": "https://files.myserver.net/base/" }, "patches/beta"] }
  ]
}
```

Here both channels point at one copy of the base files under `base/`, and
each channel's own files sit beside its manifest. A file a later folder
replaces is published with that folder.

Channels and versions the plan leaves out are listed, and their published
manifests are left as they are. [`examples/slashdiablo/build.json`](../examples/slashdiablo/build.json)
builds SlashDiablo's from its current patch folders.

## Patching

Rebuild the manifest and upload it with the new files. No pull request is
needed; players get the update next time they open the launcher. A pull
request is only needed to change the profile itself, such as adding a new
maphack version or changing your gateway.
