<script lang="ts">
  import Icon from "./Icon.svelte";
  import { app } from "./state.svelte";

  const full = $derived(app.favourites.length >= 3);

  let adding = $state(false);
  let addURL = $state("");
  let addError = $state("");
  let addBusy = $state(false);

  async function add(e: SubmitEvent) {
    e.preventDefault();
    addBusy = true;
    addError = await app.addServer(addURL.trim());
    addBusy = false;
    if (!addError) {
      adding = false;
      addURL = "";
    }
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

<div class="h-full overflow-y-auto px-10 pt-6 pb-10">
  <p class="max-w-2xl text-[13px] text-body">
    Pin up to three servers to keep them in the bar on the left. Switch between them with a click or Ctrl+1, 2 and 3.
  </p>

  {#if app.overview?.error}
    <p class="mt-6 text-bad" title={app.overview.error}>
      Couldn't load the server list. Check your connection; servers you've added by URL still work.
    </p>
  {:else if app.servers.length === 0}
    <p class="mt-6 text-body">No servers are listed yet.</p>
  {/if}

  <ul class="mt-6 grid grid-cols-[repeat(auto-fill,minmax(300px,1fr))] gap-4">
    {#each app.servers as s (s.id)}
      {@const pinned = app.favourites.includes(s.id)}
      {@const data = app.server(s.id)}
      <li class="flex flex-col rounded border border-line bg-panel/90 p-4">
        <div class="flex items-start gap-3">
          <div class="flex h-12 w-12 shrink-0 items-center justify-center rounded bg-raised">
            {#if data.branding.logo}
              <img src={data.branding.logo} alt="" class="h-10 w-10 object-contain" />
            {:else}
              <span class="title text-sm">{(s.profile?.name ?? s.id).slice(0, 2).toUpperCase()}</span>
            {/if}
          </div>
          <div class="min-w-0 flex-1">
            <h2 class="title truncate text-[15px]">{s.profile?.name ?? s.id}</h2>
            <p class="mt-0.5 text-[11px] text-muted">
              {#if s.profile}Diablo II {s.profile.game.version}{/if}
              {#if s.verified}
                · <span class="text-date">Verified</span>
              {:else}
                · <span class="text-bad/80" title="Added by URL. Not reviewed for the listing.">Not verified</span>
              {/if}
            </p>
          </div>
        </div>

        {#if s.error}
          <p class="mt-3 text-[12px] text-bad">{s.error}</p>
        {:else if s.profile?.summary}
          <p class="mt-3 text-[12px] leading-snug text-body">{s.profile.summary}</p>
        {/if}

        <div class="mt-auto flex gap-2 pt-4">
          <button
            class="title flex-1 rounded-[3px] bg-accent py-2 text-[12px] !text-white hover:brightness-125 disabled:opacity-50"
            disabled={!s.profile}
            onclick={() => app.select(s.id)}
          >
            Open
          </button>
          <button
            class="title flex items-center gap-1.5 rounded-[3px] border border-edge px-3 py-2 text-[12px] hover:border-muted disabled:opacity-40"
            aria-pressed={pinned}
            disabled={!s.profile || (!pinned && full)}
            title={!pinned && full ? "Unpin a server first; three can be pinned" : undefined}
            onclick={() => app.setFavourite(s.id, !pinned)}
          >
            <Icon name={pinned ? "check" : "pin"} size={13} />
            {pinned ? "Pinned" : "Pin"}
          </button>
          {#if !s.verified}
            <button
              class="title rounded-[3px] border border-edge px-3 py-2 text-[12px] hover:border-bad hover:!text-bad"
              title="Forget this server. Its files are left in place."
              onclick={() => app.removeServer(s.id)}
            >
              Remove
            </button>
          {/if}
        </div>
      </li>
    {/each}
  </ul>

  <section class="mt-8 max-w-2xl">
    {#if !adding}
      <button class="title text-[12px] !text-muted hover:!text-title" onclick={() => (adding = true)}>+ Add a server by URL</button>
    {:else}
      <form class="rounded border border-line bg-panel/90 p-4" onsubmit={add}>
        <h2 class="title text-[13px]">Add a server by URL</h2>
        <p class="mt-1 text-[12px] text-body">
          For servers not in the list yet, such as a new or test server. Its team gives you the link. Servers added this way
          haven't been reviewed.
        </p>
        <label class="mt-4 block text-[12px] text-muted">
          Profile URL
          <input
            class="mt-1 block h-9 w-full rounded-[3px] border border-edge bg-ink px-3 text-[13px] text-title"
            placeholder="https://example.com/launcher/profile.json"
            required
            spellcheck="false"
            bind:value={addURL}
          />
        </label>
        {#if addError}
          <p class="mt-3 text-[12px] text-bad" role="alert">{addError}</p>
        {/if}
        <div class="mt-4 flex gap-2">
          <button class="title rounded-[3px] bg-accent px-4 py-2 text-[12px] !text-white hover:brightness-125 disabled:opacity-50" disabled={addBusy}>
            {addBusy ? "Checking…" : "Add server"}
          </button>
          <button type="button" class="title rounded-[3px] border border-edge px-4 py-2 text-[12px]" onclick={() => (adding = false)}>Cancel</button>
        </div>
      </form>
    {/if}
  </section>
</div>
