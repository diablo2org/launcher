# Bug report relay

A `report.url` endpoint ([SPEC](../../docs/SPEC.md) section 3.4) as a
Cloudflare Worker. It takes the launcher's upload, checks it, and posts the
player's message with the report zip to a Discord channel. The webhook URL
stays in the Worker as a secret: anything in a profile is public, and a
webhook URL there would let anyone post to the channel.

What it checks: the request is a `POST` within `MAX_BYTES`, names this
server, and carries a zip, counting the bytes as they arrive so a chunked
upload can't get past `MAX_BYTES`; one address sends about `PER_HOUR`
reports an hour. The count is approximate (KV is eventually consistent),
and a KV failure lets a report through rather than refusing it. The player's text is quoted with mentions turned off, so it can't ping
anyone.

## Set up

1. In Discord, Channel settings, Integrations, Webhooks: create a webhook
   for the channel reports go to and copy its URL.
2. Edit `wrangler.toml`: your server id in `SERVER_ID`, and a name for the
   Worker.
3. Create the rate-limit store and put its id in `wrangler.toml`:

   ```
   npx wrangler kv namespace create LIMITS
   ```

4. Store the webhook and deploy:

   ```
   npx wrangler secret put DISCORD_WEBHOOK
   npx wrangler deploy
   ```

5. Add the Worker's URL to your profile as `report.url`, and its host to
   `hosts`. `d2pack init -report <url>` does both for a new profile.

The free Workers plan covers a server's reports many times over. Discord
takes attachments up to 10 MB in a server without boosts, so keep
`MAX_BYTES` and the profile's `report.maxBytes` at 8 MB or less unless the
server is boosted.
