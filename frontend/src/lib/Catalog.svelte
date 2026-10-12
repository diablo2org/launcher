<script lang="ts">
  import { app } from "./state.svelte";

  const full = $derived(app.favourites.length >= app.maxPinned);

  let addURL = $state("");
  let addError = $state("");
  let addBusy = $state(false);

  async function add(e: SubmitEvent) {
    e.preventDefault();
    addBusy = true;
    addError = await app.addServer(addURL.trim());
    addBusy = false;
    if (!addError) addURL = "";
  }

  // Logos for every listed server, fetched once.
  $effect(() => {
    for (const s of app.servers) {
      if (s.profile?.branding?.logo) {
        const d = app.server(s.id);
        if (!d.branding.logo) d.loadBranding();
      }
    }
  });
</script>

<div class="h-full overflow-y-auto px-9 pt-[30px] pb-10">
  <h1 class="title text-[20px] tracking-[0.06em]">All servers</h1>
  <p class="mt-1.5 max-w-2xl text-[13.5px] text-body">
    Pin up to {app.maxPinned} servers to keep them in the bar on the left. Switch between them with a click or Ctrl+1 to Ctrl+{Math.min(9, app.maxPinned)}.
  </p>

  {#if app.overview?.error}
    <p class="mt-6 text-bad" title={app.overview.error}>
      Couldn't load the server list. Check your connection; servers you've added by URL still work.
    </p>
  {:else if app.servers.length === 0}
    <p class="mt-6 text-body">No servers are listed yet.</p>
  {/if}

  <ul class="mt-[22px] grid grid-cols-[repeat(auto-fill,minmax(280px,1fr))] gap-4">
    {#each app.servers as s (s.id)}
      {@const pinned = app.favourites.includes(s.id)}
      {@const data = app.server(s.id)}
      <li class="card flex min-h-[168px] flex-col p-3.5">
        <div class="flex items-start gap-3">
          <span class="link-tile flex items-center justify-center" aria-hidden="true">
            {#if data.branding.logo}
              <img src={data.branding.logo} alt="" class="h-9 w-9 object-contain" />
            {:else}
              <span class="title text-[13px] !text-[#d9c49a]">{(s.profile?.name ?? s.id).slice(0, 2).toUpperCase()}</span>
            {/if}
          </span>
          <div class="min-w-0 flex-1">
            <h2 class="title mt-0.5 truncate text-[15px] !text-parchment">{s.profile?.name ?? s.id}</h2>
            <p class="mt-0.5 text-[11.5px] text-muted">
              {#if s.profile}Diablo II {s.profile.game.version} ·{/if}
              {#if s.verified}
                <span class="text-date">Verified</span>
              {:else}
                <span class="text-[#d0786c]" title="Added by URL. Not reviewed for the listing.">Not verified</span>
              {/if}
            </p>
          </div>
        </div>

        {#if s.error}
          <p class="mt-2.5 text-[12.5px] leading-[1.45] text-bad">{s.error}</p>
        {:else if s.profile?.summary}
          <p class="mt-2.5 text-[12.5px] leading-[1.45] text-body">{s.profile.summary}</p>
        {/if}

        <div class="mt-auto flex gap-2 pt-3.5">
          <button class="btn-stone primary h-8 flex-1 text-[11px] tracking-[0.12em]" disabled={!s.profile} onclick={() => app.select(s.id)}>OPEN</button>
          <button
            class="btn-stone flex h-8 items-center gap-1.5 px-3 text-[11px] tracking-[0.08em]"
            aria-pressed={pinned}
            disabled={!s.profile || (!pinned && full)}
            title={!pinned && full ? `Unpin a server first; ${app.maxPinned} can be pinned` : undefined}
            onclick={() => app.setFavourite(s.id, !pinned)}
          >
            <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width={pinned ? 1.6 : 1.4} aria-hidden="true">
              {#if pinned}<path d="M2.5 6.5l2.5 2.5 4.5-6" />{:else}<path d="M4.5 1.5h3l-.5 3 2 2h-6l2-2z M6 6.5V11" />{/if}
            </svg>
            {pinned ? "PINNED" : "PIN"}
          </button>
          {#if !s.verified}
            <button class="btn-stone danger h-8 px-3 text-[11px] tracking-[0.08em]" title="Forget this server. Its files are left in place." onclick={() => app.removeServer(s.id)}>
              REMOVE
            </button>
          {/if}
        </div>
      </li>
    {/each}

    <!-- Adding by URL is the grid's last card. -->
    <li class="card min-h-[168px] border-dashed p-3.5">
      <form onsubmit={add}>
        <h2 class="title text-[14px] !text-parchment">Add a server by URL</h2>
        <p class="mt-1 text-[12px] leading-[1.4] text-muted">For servers not in the list yet. Its team gives you the link; servers added this way haven't been reviewed.</p>
        <label for="catalog-url" class="title mt-2.5 mb-1 block text-[10.5px] tracking-[0.08em] !text-[#c9b48a]">PROFILE URL</label>
        <div class="flex gap-2">
          <input
            id="catalog-url"
            class="field h-8 min-w-0 flex-1 px-2.5 text-[12.5px]"
            placeholder="https://example.com/launcher/profile.json"
            required
            spellcheck="false"
            bind:value={addURL}
          />
          <button class="btn-stone primary h-8 px-3.5 text-[11px]" disabled={addBusy}>{addBusy ? "…" : "ADD"}</button>
        </div>
        {#if addError}
          <p class="mt-2 text-[12px] text-bad" role="alert">{addError}</p>
        {/if}
      </form>
    </li>
  </ul>
</div>
