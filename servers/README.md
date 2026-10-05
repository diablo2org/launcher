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

## Patching

Rebuild the manifest and upload it with the new files. No pull request is
needed; players get the update next time they open the launcher. A pull
request is only needed to change the profile itself, such as adding a new
maphack version or changing your gateway.
