<script lang="ts">
  import type { Snippet } from "svelte";
  import { fly } from "svelte/transition";

  // One notice: a badge, a title, a line or two, and a close button. A toast
  // that fades waits while the pointer is on it, so it can still be read.
  let {
    kind,
    title,
    fades = false,
    onclose,
    children,
  }: {
    kind: "ok" | "bad" | "info";
    title: string;
    fades?: boolean;
    onclose?: () => void;
    children?: Snippet;
  } = $props();

  let hovered = $state(false);

  $effect(() => {
    if (!fades || hovered || !onclose) return;
    const timer = setTimeout(onclose, 6000);
    return () => clearTimeout(timer);
  });
</script>

<div
  class={["toast pointer-events-auto flex w-[420px] items-start gap-3 py-3 pr-2.5 pl-3.5", kind === "bad" && "bad"]}
  role={kind === "bad" ? "alert" : "status"}
  onpointerenter={() => (hovered = true)}
  onpointerleave={() => (hovered = false)}
  transition:fly={{ y: -10, duration: 180 }}
>
  <span class="toast-badge mt-0.5 h-7 w-7 shrink-0" aria-hidden="true">
    <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
      {#if kind === "ok"}
        <path d="M3 7.5l2.5 2.5L11 4.5" />
      {:else if kind === "bad"}
        <path d="M7 3v5M7 10.8v.2" />
      {:else}
        <path d="M7 2.5v7M3.8 6.5L7 9.7l3.2-3.2M3 11.8h8" />
      {/if}
    </svg>
  </span>
  <div class="min-w-0 flex-1">
    <p class={["title text-[13.5px]", kind === "bad" ? "!text-bad" : "!text-parchment"]}>{title}</p>
    {#if children}
      <div class="mt-0.5 text-[12.5px] leading-[1.4] text-body">{@render children()}</div>
    {/if}
  </div>
  {#if onclose}
    <button class="shrink-0 p-1 text-muted hover:text-gold" aria-label="Close" onclick={onclose}>
      <svg width="12" height="12" viewBox="0 0 14 14" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><path d="M3 3l8 8M11 3l-8 8" /></svg>
    </button>
  {/if}
</div>
