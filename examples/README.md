# Examples

Illustrative documents for the [server profile spec](../docs/SPEC.md). They
validate against the schemas in [`schema/`](../schema) but are **not live**:

- URLs under `slashdiablo.net/files/launcher/` don't exist yet. They show
  where SlashDiablo would publish once it moves to the new format.
- `build.json` is a `d2pack build` plan for that profile, reading
  SlashDiablo's current patch folders (`slashdiablo-patches/1.13c`,
  `current`, `maphack_1.9.9` and so on) from beside it.
- File sizes and hashes in `manifest.json` are placeholders, and the file
  list is cut down to one of each kind: a game file, a mod archive, a
  player-owned `once` file and a `delete`.
- `report-relay/` is a bug report endpoint (SPEC 3.4) as a Cloudflare
  Worker that posts reports to a Discord channel. Unlike the rest, it is
  meant to be deployed as it is.
