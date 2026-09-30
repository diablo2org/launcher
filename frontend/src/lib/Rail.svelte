<script lang="ts">
  import Icon from "./Icon.svelte";
  import { app } from "./state.svelte";

  // The pinned servers, one click (or Ctrl+1..3) away. Empty slots lead to
  // the catalog so it's clear there's room for three.
  const slots = $derived([0, 1, 2].map((i) => app.favourites[i] ?? null));
  const pinned = $derived(app.favourites.length);

  // A server opened from the catalog without pinning it gets a tile below.
  const transient = $derived(
    app.selected && !app.favourites.includes(app.selected) && app.page !== "catalog" ? app.selected : null,
  );

  function initials(name: string): string {
    return name
      .split(/\s+/)
      .map((w) => w[0])
      .join("")
      .slice(0, 2)
      .toUpperCase();
  }

  // Drag to reorder, like Discord's server list. dropAt is the gap the tile
  // would land in: 0 is above the first tile, pinned is below the last.
  let dragFrom = $state<number | null>(null);
  let dropAt = $state<number | null>(null);

  function onDragStart(e: DragEvent, i: number) {
    dragFrom = i;
    e.dataTransfer?.setData("text/plain", app.favourites[i]);
    if (e.dataTransfer) e.dataTransfer.effectAllowed = "move";
  }

  function onDragOver(e: DragEvent, i: number) {
    if (dragFrom === null) return;
    e.preventDefault();
    if (e.dataTransfer) e.dataTransfer.dropEffect = "move";

    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    dropAt = e.clientY < r.top + r.height / 2 ? i : i + 1;
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    if (dragFrom !== null && dropAt !== null) {
      // Removing the tile first shifts every gap below it up by one.
      const to = dropAt > dragFrom ? dropAt - 1 : dropAt;
      app.moveFavourite(dragFrom, to);
    }
    endDrag();
  }

  function endDrag() {
    dragFrom = null;
    dropAt = null;
  }

  // The same from the keyboard: Alt+Up and Alt+Down on a focused tile.
  function onKeydown(e: KeyboardEvent, i: number) {
    if (!e.altKey) return;
    const to = e.key === "ArrowUp" ? i - 1 : e.key === "ArrowDown" ? i + 1 : null;
    if (to === null || to < 0 || to >= pinned) return;

    e.preventDefault();
    app.moveFavourite(i, to);
    // Keep focus on the moved tile once it has re-rendered.
    requestAnimationFrame(() => document.querySelector<HTMLElement>(`[data-slot="${to}"] button`)?.focus());
  }

  // A gap is only worth showing where the tile would actually move.
  function showGap(gap: number): boolean {
    return dragFrom !== null && dropAt === gap && gap !== dragFrom && gap !== dragFrom + 1;
  }
</script>

{#snippet tile(id: string, index: number | null)}
  {@const info = app.info(id)}
  {@const data = app.server(id)}
  {@const active = app.selected === id && app.page !== "catalog" && app.page !== "checker"}
  {@const name = info?.profile?.name ?? id}
  <button
    class="group relative flex h-14 w-14 items-center justify-center rounded-lg border transition-colors hover:border-edge"
    class:border-edge={active}
    class:border-line={!active}
    class:bg-raised={active}
    class:bg-[#0d0d10]={!active}
    title={index !== null ? `${name} (Ctrl+${index + 1}, drag to reorder)` : name}
    aria-label={index !== null ? `${name}, pinned ${index + 1} of ${pinned}. Alt+Up or Alt+Down to move.` : name}
    aria-current={active ? "page" : undefined}
    onclick={() => app.select(id)}
    onkeydown={(e) => index !== null && onKeydown(e, index)}
  >
    <span
      class="absolute top-1/2 -left-2.5 h-8 w-1 -translate-y-1/2 rounded-r bg-title transition-opacity"
      class:opacity-0={!active}
      aria-hidden="true"
    ></span>
    {#if data.branding.logo}
      <img
        src={data.branding.logo}
        alt=""
        draggable="false"
        class="h-10 w-10 object-contain opacity-80 transition-opacity group-hover:opacity-100"
        class:opacity-100={active}
      />
    {:else}
      <span class="title text-sm" class:text-muted={!active}>{initials(name)}</span>
    {/if}
    {#if data.busy === "updating"}
      <span class="absolute right-1 bottom-1 h-2 w-2 animate-pulse rounded-full bg-title" aria-label="Updating"></span>
    {:else if data.status && !data.status.error && data.status.installed && !data.status.upToDate}
      <span class="absolute right-1 bottom-1 h-2 w-2 rounded-full bg-accent ring-1 ring-title/50" aria-label="Update available"></span>
    {/if}
  </button>
{/snippet}

<nav class="drag relative z-10 flex h-full w-[76px] shrink-0 flex-col items-center gap-2 border-r border-line bg-[#070709] pt-4 pb-3" aria-label="Pinned servers">
  {#each slots as id, i (id ?? `empty-${i}`)}
    {#if id}
      <div
        class="no-drag relative transition-opacity"
        class:opacity-40={dragFrom === i}
        data-slot={i}
        draggable="true"
        role="listitem"
        ondragstart={(e) => onDragStart(e, i)}
        ondragover={(e) => onDragOver(e, i)}
        ondrop={onDrop}
        ondragend={endDrag}
      >
        {#if showGap(i)}
          <span class="pointer-events-none absolute -top-[5px] right-0 left-0 h-[3px] rounded-full bg-title" aria-hidden="true"></span>
        {/if}
        {@render tile(id, i)}
        {#if i === pinned - 1 && showGap(pinned)}
          <span class="pointer-events-none absolute right-0 -bottom-[5px] left-0 h-[3px] rounded-full bg-title" aria-hidden="true"></span>
        {/if}
      </div>
    {:else}
      <div class="no-drag">
        <button
          class="flex h-14 w-14 items-center justify-center rounded-lg border border-dashed border-edge text-xl text-dim hover:border-muted hover:text-muted"
          title="Pin a server"
          aria-label="Pin a server"
          onclick={() => (app.page = "catalog")}
        >+</button>
      </div>
    {/if}
  {/each}

  {#if transient}
    <div class="my-1 h-px w-8 bg-line" aria-hidden="true"></div>
    <div class="no-drag">{@render tile(transient, null)}</div>
  {/if}

  <div class="flex-1"></div>

  <button
    class="no-drag flex h-12 w-12 items-center justify-center rounded-lg hover:bg-raised"
    class:text-title={app.page === "catalog"}
    class:text-muted={app.page !== "catalog"}
    title="All servers"
    aria-label="All servers"
    aria-current={app.page === "catalog" ? "page" : undefined}
    onclick={() => (app.page = "catalog")}
  >
    <Icon name="grid" size={20} />
  </button>
</nav>
