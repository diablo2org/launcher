<script lang="ts">
  import type { ServerData } from "./state.svelte";

  let { data }: { data: ServerData } = $props();

  let board = $state(0);

  $effect(() => {
    data.id;
    board = 0;
    if (!data.ladder) data.loadLadder();
  });

  const boards = $derived(data.ladder?.boards ?? []);
  const entries = $derived(boards[board]?.entries ?? []);
</script>

<div class="flex h-full flex-col px-10 pt-6 pb-6">
  {#if boards.length > 1}
    <div class="mb-4 flex gap-6" role="tablist">
      {#each boards as b, i (b.id)}
        <button
          role="tab"
          aria-selected={board === i}
          class="title text-[13px]"
          class:!text-dim={board !== i}
          onclick={() => (board = i)}
        >
          {b.name}
        </button>
      {/each}
    </div>
  {/if}

  {#if data.ladder === null && !data.ladderError}
    <p class="text-body">Loading ladder…</p>
  {:else if data.ladderError}
    <p class="text-body">The ladder is unavailable right now.</p>
  {:else if entries.length === 0}
    <p class="text-body">Nobody on this ladder yet.</p>
  {:else}
    <div class="min-h-0 flex-1 overflow-y-auto rounded border border-line bg-ink/70">
      <table class="w-full text-left text-[13px]">
        <thead class="sticky top-0 bg-panel">
          <tr class="title text-[12px]">
            <th class="w-16 px-4 py-3">Rank</th>
            <th class="px-4 py-3">Name</th>
            <th class="px-4 py-3">Class</th>
            <th class="w-20 px-4 py-3">Level</th>
            <th class="px-4 py-3">Status</th>
          </tr>
        </thead>
        <tbody>
          {#each entries as e (e.rank + e.name)}
            <tr class="border-t border-line text-body hover:bg-raised/70">
              <td class="title px-4 py-2 !text-date">{e.rank}</td>
              <td class="px-4 py-2 text-title">
                {#if e.title}<span class="text-muted">{e.title} </span>{/if}{e.name}
              </td>
              <td class="px-4 py-2">{e.class}</td>
              <td class="px-4 py-2">{e.level}</td>
              <td class="px-4 py-2 capitalize" class:!text-bad={e.status === "dead"}>{e.status}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
