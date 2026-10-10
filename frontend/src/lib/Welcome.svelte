<script lang="ts">
  import Icon from "./Icon.svelte";
  import { app } from "./state.svelte";

  // Two steps: the Diablo II folder, then the servers to pin.
  let step = $state<"folder" | "servers">("folder");
  let picked = $state<string[]>([]);
  let busy = $state(false);

  const o = $derived(app.overview);
  const missing = $derived(o?.report?.missing ?? []);
  const listed = $derived(app.servers.filter((s) => s.profile && s.verified));
  const full = $derived(picked.length >= 3);

  function toggle(id: string) {
    if (picked.includes(id)) picked = picked.filter((p) => p !== id);
    else if (!full) picked = [...picked, id];
  }

  async function finish() {
    busy = true;
    await app.finishWelcome(picked);
    busy = false;
  }

  // Logos for the listed servers, fetched once.
  $effect(() => {
    for (const s of listed) {
      if (s.profile?.branding?.logo) {
        const d = app.server(s.id);
        if (!d.branding.logo) d.loadBranding();
      }
    }
  });
</script>

<div class="h-full overflow-y-auto px-10 pt-8 pb-10">
  <div class="mx-auto max-w-3xl">
    <p class="text-[11px] text-muted">Step {step === "folder" ? 1 : 2} of 2</p>

    {#if step === "folder"}
      <h1 class="title mt-2 text-[26px]">Welcome</h1>
      <p class="mt-3 max-w-2xl text-[13px] leading-relaxed text-body">
        This launcher installs, updates and starts Diablo II private servers. Each server gets its own folder inside your Diablo II
        install, so your game is never changed and servers don't get in each other's way.
      </p>

      <section class="mt-8 rounded border border-line bg-panel/90 p-5">
        <h2 class="title text-[14px]">Your Diablo II folder</h2>

        {#if !o?.base}
          <p class="mt-2 text-[13px] text-body">
            Diablo II wasn't found. Choose the folder where Diablo II and Lord of Destruction are installed.
          </p>
        {:else}
          <p class="mt-2 truncate text-[13px] text-title" title={o.base}>{o.base}</p>
          {#if o.baseError}
            <p class="mt-2 text-[12px] text-bad">{o.baseError}</p>
          {:else if missing.length}
            <p class="mt-2 text-[12px] text-bad">
              Missing {missing.join(", ")}. Install Diablo II and Lord of Destruction there, or choose another folder.
            </p>
          {:else}
            <p class="mt-2 flex items-center gap-1.5 text-[12px] text-date"><Icon name="check" size={13} /> Diablo II and Lord of Destruction found.</p>
          {/if}
        {/if}
        {#if o?.warning}
          <p class="mt-2 text-[12px] text-date">{o.warning}</p>
        {/if}

        <button
          class="title mt-4 flex items-center gap-2 rounded-[3px] border border-edge px-4 py-2 text-[12px] hover:border-muted"
          onclick={() => app.chooseBase()}
        >
          <Icon name="folder" size={14} />
          {o?.base ? "Choose another folder" : "Choose folder"}
        </button>
      </section>

      <div class="mt-8 flex items-center gap-4">
        <button
          class="title rounded-[3px] bg-accent px-6 py-2 text-[13px] !text-white hover:brightness-125 disabled:opacity-50"
          disabled={!app.baseReady}
          onclick={() => (step = "servers")}
        >
          Next
        </button>
        {#if !app.baseReady}
          <button class="title text-[12px] !text-muted hover:!text-title" onclick={() => (step = "servers")}>Skip for now</button>
        {/if}
      </div>
    {:else}
      <h1 class="title mt-2 text-[26px]">Pick your servers</h1>
      <p class="mt-3 max-w-2xl text-[13px] leading-relaxed text-body">
        Choose up to three to keep in the bar on the left, and switch between them with Ctrl+1, 2 and 3. You can change them any time
        from All servers.
      </p>

      {#if o?.error}
        <p class="mt-6 text-[13px] text-bad" title={o.error}>
          Couldn't load the server list. Check your connection; you can pick servers later from All servers.
        </p>
      {:else if listed.length === 0}
        <p class="mt-6 text-[13px] text-body">No servers are listed yet. You can add one by URL from All servers.</p>
      {/if}

      <ul class="mt-6 grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-3">
        {#each listed as s (s.id)}
          {@const on = picked.includes(s.id)}
          {@const data = app.server(s.id)}
          <li>
            <button
              class={[
                "flex h-full w-full items-start gap-3 rounded border p-4 text-left transition-colors disabled:opacity-40",
                on ? "border-accent bg-accent/20" : "border-line bg-panel/90 hover:border-edge",
              ]}
              aria-pressed={on}
              disabled={!on && full}
              title={!on && full ? "Three servers can be pinned" : undefined}
              onclick={() => toggle(s.id)}
            >
              <div class="flex h-11 w-11 shrink-0 items-center justify-center rounded bg-raised">
                {#if data.branding.logo}
                  <img src={data.branding.logo} alt="" class="h-9 w-9 object-contain" />
                {:else}
                  <span class="title text-sm">{(s.profile?.name ?? s.id).slice(0, 2).toUpperCase()}</span>
                {/if}
              </div>
              <div class="min-w-0 flex-1">
                <p class="title truncate text-[14px]">{s.profile?.name ?? s.id}</p>
                <p class="mt-0.5 text-[11px] text-muted">Diablo II {s.profile?.game.version}</p>
                {#if s.profile?.summary}
                  <p class="mt-2 text-[12px] leading-snug text-body">{s.profile.summary}</p>
                {/if}
              </div>
              <span class="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-[3px] border" class:border-accent={on} class:bg-accent={on} class:border-edge={!on}>
                {#if on}<Icon name="check" size={12} />{/if}
              </span>
            </button>
          </li>
        {/each}
      </ul>

      <div class="mt-8 flex items-center gap-4">
        <button class="title rounded-[3px] bg-accent px-6 py-2 text-[13px] !text-white hover:brightness-125 disabled:opacity-50" disabled={busy} onclick={finish}>
          {picked.length ? "Done" : "Skip for now"}
        </button>
        <button class="title text-[12px] !text-muted hover:!text-title" onclick={() => (step = "folder")}>Back</button>
      </div>
    {/if}
  </div>
</div>
