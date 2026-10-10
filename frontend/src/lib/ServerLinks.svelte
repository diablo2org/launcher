<script lang="ts">
  import { Browser } from "@wailsio/runtime";
  import type { Links } from "../../bindings/github.com/diablo2org/launcher/internal/spec";
  import Icon from "./Icon.svelte";

  let { links }: { links: Links | undefined } = $props();

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

  const shown = $derived(labels.filter(([key]) => links?.[key]).map(([key, label]) => ({ label, url: links![key]! })));

  // The hover text says where a link goes, since the server chose it.
  function host(url: string): string {
    try {
      return new URL(url).host;
    } catch {
      return url;
    }
  }
</script>

{#if shown.length}
  <nav class="flex flex-wrap gap-2" aria-label="Server links">
    {#each shown as l (l.label)}
      <button
        class="title flex h-7 items-center gap-1.5 rounded-[3px] border border-edge bg-panel/80 px-3 text-[11px] transition-colors hover:border-muted hover:!text-title"
        title={`Opens ${host(l.url)}`}
        onclick={() => Browser.OpenURL(l.url)}
      >
        {l.label}
        <span class="opacity-40"><Icon name="out" size={11} /></span>
      </button>
    {/each}
  </nav>
{/if}
