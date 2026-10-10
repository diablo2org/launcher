// A bug report endpoint for the launcher (docs/SPEC.md section 3.4), as a
// Cloudflare Worker. It checks each upload and posts it to a Discord channel
// through a webhook that stays here, out of the public profile.
//
// Settings (wrangler.toml [vars], and a secret for the webhook):
//   SERVER_ID        the profile id reports must name
//   DISCORD_WEBHOOK  secret: wrangler secret put DISCORD_WEBHOOK
//   MAX_BYTES        the profile's report.maxBytes (default 8 MB; Discord
//                    takes attachments up to 10 MB on a server without boosts)
//   PER_HOUR         reports one address may send in an hour (default 3)
// and, optionally, a KV namespace bound as LIMITS for the per-hour count.
// Without it there is no rate limit.

const json = (status, body) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const refuse = (status, error) => json(status, { error });

// A short reference the player can quote, such as R-K4F2Q9.
function reference() {
  const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789";
  const bytes = crypto.getRandomValues(new Uint8Array(6));
  return "R-" + Array.from(bytes, (b) => alphabet[b % alphabet.length]).join("");
}

// One address's reports this hour. KV is eventually consistent, so a burst
// can slip a report or two past the limit; that is fine for this.
async function overLimit(env, request) {
  if (!env.LIMITS) return false;

  const ip = request.headers.get("cf-connecting-ip") || "unknown";
  const key = `ip:${ip}:${Math.floor(Date.now() / 3600000)}`;
  const count = parseInt((await env.LIMITS.get(key)) || "0", 10);
  if (count >= parseInt(env.PER_HOUR || "3", 10)) return true;

  await env.LIMITS.put(key, String(count + 1), { expirationTtl: 3700 });
  return false;
}

// Keeps a player's text from pinging anyone or breaking out of its quote.
function quoted(text) {
  return text
    .split(/\r?\n/)
    .map((line) => "> " + line.replace(/[`*_~|<>@]/g, (c) => "\\" + c))
    .join("\n");
}

export default {
  async fetch(request, env) {
    if (request.method !== "POST") return refuse(405, "Bug reports are sent with POST.");

    const max = parseInt(env.MAX_BYTES || String(8 << 20), 10);
    const length = parseInt(request.headers.get("content-length") || "0", 10);
    if (length > max) return refuse(413, "The report is too large.");

    if (await overLimit(env, request)) return refuse(429, "Too many reports from you in the last hour. Please try again later.");

    let form;
    try {
      form = await request.formData();
    } catch {
      return refuse(400, "The report couldn't be read.");
    }

    const server = String(form.get("server") || "");
    const launcher = String(form.get("launcher") || "").slice(0, 32);
    const message = String(form.get("message") || "").slice(0, 2000);
    const contact = String(form.get("contact") || "").slice(0, 100);
    const report = form.get("report");

    if (server !== env.SERVER_ID) return refuse(400, "This endpoint takes reports for another server.");
    if (!(report instanceof File) || report.size === 0) return refuse(400, "The report is missing.");
    if (report.size > max) return refuse(413, "The report is too large.");

    const zip = await report.arrayBuffer();
    const magic = new Uint8Array(zip, 0, 4);
    if (magic[0] !== 0x50 || magic[1] !== 0x4b || magic[2] !== 0x03 || magic[3] !== 0x04) {
      return refuse(400, "The report isn't a zip.");
    }

    const id = reference();
    const lines = [`**Bug report ${id}** · launcher ${launcher || "unknown"}`];
    if (contact) lines.push(`From: ${quoted(contact).slice(2)}`);
    if (message) lines.push(quoted(message));

    const post = new FormData();
    post.append(
      "payload_json",
      JSON.stringify({ content: lines.join("\n").slice(0, 2000), allowed_mentions: { parse: [] } }),
    );
    post.append("files[0]", new Blob([zip], { type: "application/zip" }), `${id}.zip`);

    const sent = await fetch(env.DISCORD_WEBHOOK, { method: "POST", body: post });
    if (!sent.ok) {
      console.log(`webhook answered ${sent.status} for ${id}`);
      return refuse(502, "The report couldn't be delivered. Please try again later.");
    }

    return json(200, { id, message: "Thanks, the team has your report." });
  },
};
