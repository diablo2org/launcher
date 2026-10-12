<script lang="ts">
  import { app } from "./state.svelte";

  // The pinned servers, one click (or Ctrl+1, 2, ...) away, then one "+" while
  // there's room for another. Settings and All servers sit at the foot.
  const pinned = $derived(app.favourites.length);
  const room = $derived(pinned < app.maxPinned);

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

  // Each tile shows a different patch of the stone, so no two look identical.
  const patches = ["0px 0px", "-72px -24px", "-24px -120px", "-132px -84px", "-48px -60px", "-108px -150px", "-160px -10px", "-10px -170px"];
  function patch(i: number): string {
    const [fx, fy] = patches[i % patches.length].split(" ");
    return `--fx: ${fx}; --fy: ${fy}`;
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

<!-- The gold bar beside whatever page is open. -->
{#snippet bar(on: boolean)}
  <span
    class="absolute top-1/2 -left-[14px] h-[26px] w-1 -translate-y-1/2 rounded-r-[2px] bg-[#d9b56e] shadow-[0_0_8px_rgba(217,181,110,0.7)] transition-opacity"
    class:opacity-0={!on}
    aria-hidden="true"
  ></span>
{/snippet}

{#snippet tile(id: string, index: number | null)}
  {@const info = app.info(id)}
  {@const data = app.server(id)}
  {@const active = app.selected === id && app.page !== "catalog" && app.page !== "checker"}
  {@const name = info?.profile?.name ?? id}
  <button
    class="rail-btn group transition-opacity"
    class:opacity-70={!active}
    class:hover:opacity-100={!active}
    style={patch(index ?? pinned)}
    title={index !== null && index < 9 ? `${name} (Ctrl+${index + 1}, drag to reorder)` : name}
    aria-label={index !== null ? `${name}, pinned ${index + 1} of ${pinned}. Alt+Up or Alt+Down to move.` : name}
    aria-current={active ? "page" : undefined}
    onclick={() => app.select(id)}
    onkeydown={(e) => index !== null && onKeydown(e, index)}
  >
    {@render bar(active)}
    {#if data.branding.logo}
      <img src={data.branding.logo} alt="" draggable="false" class="relative h-[30px] w-[30px] object-contain" />
    {:else}
      <span
        class={[
          "title relative text-[14px]",
          active
            ? "!text-[#f3dba0] [text-shadow:0_0_10px_rgba(240,190,90,0.55),0_1px_2px_#000]"
            : "!text-[#9a8b70] [text-shadow:0_1px_2px_#000] group-hover:!text-parchment",
        ]}
      >{initials(name)}</span>
    {/if}
    {#if data.busy === "updating"}
      <span class="absolute -right-1 -bottom-1 z-10 h-2.5 w-2.5 animate-pulse rounded-full border-2 border-[#0d0c0b] bg-[#d9b56e] shadow-[0_0_8px_rgba(217,181,110,0.9)]" aria-label="Updating"></span>
    {:else if data.status && !data.status.error && data.status.installed && !data.status.upToDate}
      <span class="absolute -right-1 -bottom-1 z-10 h-2.5 w-2.5 rounded-full border-2 border-[#0d0c0b] bg-blood" aria-label="Update available"></span>
    {/if}
  </button>
{/snippet}

<nav class="drag tex-slate relative z-10 flex h-full w-[76px] shrink-0 flex-col items-center gap-3 border-r border-[#3b2f1c] pt-4 pb-3.5" aria-label="Pinned servers">
  {#each app.favourites as id, i (id)}
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
        <span class="pointer-events-none absolute -top-[8px] right-0 left-0 h-[3px] rounded-full bg-gold" aria-hidden="true"></span>
      {/if}
      {@render tile(id, i)}
      {#if i === pinned - 1 && showGap(pinned)}
        <span class="pointer-events-none absolute right-0 -bottom-[8px] left-0 h-[3px] rounded-full bg-gold" aria-hidden="true"></span>
      {/if}
    </div>
  {/each}

  {#if room}
    <div class="no-drag">
      <button class="rail-btn tool" style={patch(pinned + 1)} title="Pin a server" aria-label="Pin a server" onclick={() => (app.page = "catalog")}>
        <span class="rail-icon plus" aria-hidden="true"></span>
      </button>
    </div>
  {/if}

  {#if transient}
    <div class="my-0.5 h-px w-8 bg-gold-line/50" aria-hidden="true"></div>
    <div class="no-drag">{@render tile(transient, null)}</div>
  {/if}

  <div class="flex-1"></div>

  <div class="no-drag flex flex-col gap-3">
    <button
      class="rail-btn tool relative"
      style={patch(pinned + 2)}
      title="All servers"
      aria-label="All servers"
      aria-current={app.page === "catalog" ? "page" : undefined}
      onclick={() => (app.page = "catalog")}
    >
      {@render bar(app.page === "catalog")}
      <span class="rail-icon map" aria-hidden="true"></span>
    </button>
    <button class="rail-btn tool" style={patch(pinned + 3)} title="Settings" aria-label="Settings" onclick={() => (app.settingsOpen = true)}>
      <span class="rail-icon cog" aria-hidden="true"></span>
    </button>
  </div>
</nav>
