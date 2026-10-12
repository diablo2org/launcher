<script lang="ts">
  import { Browser } from "@wailsio/runtime";
  import type { Links } from "../../bindings/github.com/diablo2org/launcher/internal/spec";
  import LinkTile from "./LinkTile.svelte";
  import { app } from "./state.svelte";

  // ladder: the profile hides the tab bar, so the ladder is a button here.
  let { links, ladder = false }: { links: Links | undefined; ladder?: boolean } = $props();

  // Every link the profile gives, in the spec's order.
  const labels: [keyof Links, string][] = [
    ["website", "Website"],
    ["discord", "Discord"],
    ["forum", "Forum"],
    ["wiki", "Wiki"],
    ["trade", "Trade"],
    ["register", "Register"],
    ["support", "Support"],
    ["donate", "Donate"],
  ];

  const shown = $derived(labels.filter(([key]) => links?.[key]).map(([key, label]) => ({ key, label, url: links![key]! })));

  // Up to four to a column, bottom-aligned; more start a second column. Report
  // always comes last.
  // Nine or ten take five rows, so they still fit two columns beside PLAY at
  // the narrowest window.
  const total = $derived(shown.length + (ladder ? 2 : 1));
  const rows = $derived(total > 8 ? 5 : Math.min(4, total));

  // The hover text says where a link goes, since the server chose it.
  function host(url: string): string {
    try {
      return new URL(url).host;
    } catch {
      return url;
    }
  }
</script>

<nav class="grid grid-flow-col content-end gap-x-8 gap-y-2" style:grid-template-rows={`repeat(${rows}, auto)`} aria-label="Server links">
  {#each shown as l (l.key)}
    <LinkTile kind={l.key} label={l.label} title={`Opens ${host(l.url)}`} onclick={() => Browser.OpenURL(l.url)} />
  {/each}
  {#if ladder}
    <LinkTile kind="ladder" label="Ladder" glyph="rank" title="This server's ladder" onclick={() => (app.page = "ladder")} />
  {/if}
  <LinkTile kind="report" label="Report" title="Report a problem with this server" onclick={() => (app.report = "server")} />
</nav>
