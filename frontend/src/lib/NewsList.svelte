<script lang="ts">
  import { Browser } from "@wailsio/runtime";
  import type { ServerData } from "./state.svelte";
  import Icon from "./Icon.svelte";

  let { data }: { data: ServerData } = $props();

  const format = new Intl.DateTimeFormat("en-GB", { day: "2-digit", month: "short", year: "numeric" });

  function date(value: unknown): string {
    const d = new Date(String(value));
    return isNaN(d.getTime()) || d.getFullYear() < 2000 ? "" : format.format(d).toUpperCase();
  }
</script>

<div class="h-full overflow-y-auto px-4 pt-6 pb-10" aria-label="News">
  {#if data.news === null && !data.newsError}
    <p class="text-body">Loading news…</p>
  {:else if data.newsError && !data.news?.length}
    <p class="text-body">News is unavailable right now.</p>
  {:else if !data.news?.length}
    <p class="text-body">No news yet.</p>
  {:else}
    <ul class="space-y-7">
      {#each data.news as item, i (i)}
        <li>
          {#if item.url}
            <button class="title flex items-start gap-2 text-left text-[16px] hover:text-white" onclick={() => Browser.OpenURL(item.url)}>
              <span>{item.title}</span>
              <span class="mt-1 shrink-0 opacity-30"><Icon name="out" size={13} /></span>
            </button>
          {:else}
            <h3 class="title text-[16px]">{item.title}</h3>
          {/if}
          {#if date(item.date)}
            <p class="title mt-0.5 text-[12px] !text-date">{date(item.date)}</p>
          {/if}
          {#if item.summary}
            <p class="mt-2.5 text-[12px] leading-[1.45] text-body">{item.summary}</p>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>
