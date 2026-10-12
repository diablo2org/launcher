<script lang="ts">
  import Icon from "./Icon.svelte";
  import type { Row } from "./wine.svelte";

  // A setup's steps, each done, under way, waiting or failed.
  let { rows }: { rows: Row[] } = $props();
</script>

<ol class="mt-6 grid max-w-xl gap-2">
  {#each rows as row (row.label)}
    <li class="card flex items-center gap-3 px-4 py-3">
      <span
        class={[
          "flex h-5 w-5 shrink-0 items-center justify-center rounded-full border text-[11px]",
          row.state === "done" && "mark-on",
          row.state === "active" && "animate-pulse border-gold-mid",
          row.state === "failed" && "border-bad text-bad",
          row.state === "pending" && "border-edge",
        ]}
        aria-label={row.state}
      >
        {#if row.state === "done"}<Icon name="check" size={12} />{:else if row.state === "failed"}!{/if}
      </span>
      <span class="title text-[13px] !text-parchment">{row.label}</span>
      {#if row.detail}
        <span class="ml-auto truncate text-[12px] text-muted" title={row.detail}>{row.detail}</span>
      {/if}
    </li>
  {/each}
</ol>
