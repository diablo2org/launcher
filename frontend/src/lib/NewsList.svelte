<script lang="ts">
  import { Browser } from "@wailsio/runtime";
  import type { ServerData } from "./state.svelte";

  let { data }: { data: ServerData } = $props();

  const format = new Intl.DateTimeFormat("en-GB", { day: "2-digit", month: "short", year: "numeric" });

  function date(value: unknown): string {
    const d = new Date(String(value));
    return isNaN(d.getTime()) || d.getFullYear() < 2000 ? "" : format.format(d).toUpperCase();
  }

  // Collapsed to its heading until opened; the choice is kept for next time.
  const key = "news-open";
  function remembered(): boolean {
    try {
      return localStorage.getItem(key) === "1";
    } catch {
      return false;
    }
  }
  let open = $state(remembered());
  function toggle() {
    open = !open;
    try {
      localStorage.setItem(key, open ? "1" : "0");
    } catch {
      // Not remembered; it still opens and closes.
    }
  }

  // One post at a time, newest first.
  const posts = $derived(data.news ?? []);
  let index = $state(0);
  const post = $derived(posts[Math.min(index, posts.length - 1)]);
  $effect(() => {
    if (index >= posts.length) index = 0;
  });
</script>

<!-- The heading stays where it is whether the card is open or not: the card
     grows leftwards and downwards from it. -->
<section class="glass flex max-h-full min-h-0 flex-col" class:w-[318px]={open} aria-label="News">
  <button
    class="flex shrink-0 items-center justify-end gap-2.5 px-4 py-[11px] text-gold hover:text-[#fff0c4]"
    aria-expanded={open}
    aria-controls="news-body"
    onclick={toggle}
  >
    <span class="title text-[13px] tracking-[0.14em] !text-current">NEWS</span>
    <svg class="transition-transform" class:rotate-180={open} width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
      <path d="M2.5 4.5L6 8l3.5-3.5" />
    </svg>
  </button>

  {#if open}
    <div id="news-body" class="flex min-h-0 flex-col px-4 pb-3">
      {#if data.news === null && !data.newsError}
        <p class="text-[13px] text-body">Loading news…</p>
      {:else if data.newsError && !posts.length}
        <p class="text-[13px] text-body">News is unavailable right now.</p>
      {:else if !post}
        <p class="text-[13px] text-body">No news yet.</p>
      {:else}
        <article class="min-h-0 overflow-y-auto pr-1">
          {#if date(post.date)}
            <p class="text-[11px] tracking-[0.08em] text-date">{date(post.date)}</p>
          {/if}
          {#if post.url}
            <button class="title mt-0.5 text-left text-[14px] !text-parchment hover:!text-[#fff0c4]" onclick={() => Browser.OpenURL(post.url)}>{post.title}</button>
          {:else}
            <h3 class="title mt-0.5 text-[14px] !text-parchment">{post.title}</h3>
          {/if}
          {#if post.summary}
            <p class="mt-1.5 text-[13px] leading-[1.45] whitespace-pre-line text-body">{post.summary}</p>
          {/if}
        </article>

        {#if post.url || posts.length > 1}
          <div class="mt-2.5 flex shrink-0 items-center justify-between border-t border-gold-line/30 pt-2.5">
            {#if post.url}
              <button class="text-[13px] text-gold-mid hover:text-[#f3e2b4]" title="Opens the post in your browser" onclick={() => Browser.OpenURL(post.url)}>
                Read more ↗
              </button>
            {:else}
              <span></span>
            {/if}
            {#if posts.length > 1}
              <div class="flex items-center gap-1.5 text-[12px] text-muted">
                <button class="pager" aria-label="Newer post" disabled={index === 0} onclick={() => index--}>‹</button>
                <span aria-live="polite">{index + 1} / {posts.length}</span>
                <button class="pager" aria-label="Older post" disabled={index >= posts.length - 1} onclick={() => index++}>›</button>
              </div>
            {/if}
          </div>
        {/if}
      {/if}
    </div>
  {/if}
</section>

<style>
  .pager {
    width: 22px;
    height: 22px;
    border: 1px solid rgba(201, 164, 96, 0.28);
    color: #8c7d62;
    font-size: 13px;
    line-height: 1;
  }
  .pager:hover:not(:disabled) {
    border-color: var(--color-gold-line);
    color: var(--color-parchment);
  }
  .pager:disabled {
    opacity: 0.35;
  }
</style>
