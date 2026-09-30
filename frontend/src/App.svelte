<script lang="ts">
  import Catalog from "./lib/Catalog.svelte";
  import LadderView from "./lib/LadderView.svelte";
  import LaunchView from "./lib/LaunchView.svelte";
  import ProfileChecker from "./lib/ProfileChecker.svelte";
  import Rail from "./lib/Rail.svelte";
  import SettingsModal from "./lib/SettingsModal.svelte";
  import TopBar from "./lib/TopBar.svelte";
  import { app } from "./lib/state.svelte";

  app.load();

  // Every server that has been shown keeps its background layer, so switching
  // crossfades rather than flashing to black while an image decodes.
  const layers = $derived([...new Set([...app.favourites, app.selected].filter(Boolean))]);
  const showBackground = $derived(app.page === "launch" || app.page === "ladder");

  function onKeydown(e: KeyboardEvent) {
    if ((e.ctrlKey || e.metaKey) && ["1", "2", "3"].includes(e.key)) {
      e.preventDefault();
      app.selectFavourite(Number(e.key) - 1);
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="relative flex h-full overflow-hidden bg-ink">
  {#each layers as id (id)}
    {@const data = app.server(id)}
    {@const accent = app.info(id)?.profile?.branding?.accent}
    <div
      class="pointer-events-none absolute inset-0 transition-opacity duration-500"
      class:opacity-0={!(showBackground && app.selected === id)}
      aria-hidden="true"
    >
      {#if data.branding.background}
        <div class="absolute inset-0 bg-cover bg-center" style:background-image={`url(${data.branding.background})`}></div>
      {:else}
        <div
          class="absolute inset-0"
          style:background={`radial-gradient(ellipse at 35% 30%, ${accent || "#5c0202"}33 0%, transparent 60%)`}
        ></div>
      {/if}
      <div class="absolute inset-0 bg-gradient-to-t from-ink/80 via-transparent to-ink/40"></div>
    </div>
  {/each}

  <Rail />

  <!-- A server's accent only applies on its own pages; the catalog and tools keep the launcher's. -->
  <div
    class="relative flex min-w-0 flex-1 flex-col"
    style:--color-accent={(showBackground && app.current?.profile?.branding?.accent) || undefined}
  >
    <TopBar />

    <main class="min-h-0 flex-1">
      {#if app.error}
        <p class="mx-8 mt-4 rounded border border-bad/50 bg-panel/90 p-3 text-[12px] text-bad" role="alert">{app.error}</p>
      {/if}

      {#if app.page === "catalog"}
        <Catalog />
      {:else if app.page === "checker"}
        <ProfileChecker />
      {:else if app.current?.profile}
        {#key app.selected}
          {#if app.page === "ladder"}
            <LadderView data={app.server(app.selected)} />
          {:else}
            <LaunchView info={app.current} data={app.server(app.selected)} />
          {/if}
        {/key}
      {:else if app.current?.error}
        <p class="mx-8 mt-6 rounded border border-bad/50 bg-panel/90 p-4 text-[13px] text-bad">{app.current.error}</p>
      {:else if !app.overview}
        <p class="px-8 pt-6 text-body">Loading…</p>
      {/if}
    </main>
  </div>

  {#if app.settingsOpen}
    <SettingsModal />
  {/if}
</div>
