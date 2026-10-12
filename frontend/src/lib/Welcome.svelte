<script lang="ts">
  import GetDiablo from "./GetDiablo.svelte";
  import Icon from "./Icon.svelte";
  import { app } from "./state.svelte";

  // The servers to pin come first, so the next step can show what they
  // need; then getting Diablo II, by installing it or finding it.
  let step = $state<"servers" | "game">("servers");
  let picked = $state<string[]>([]);

  const o = $derived(app.overview);
  const listed = $derived(app.servers.filter((s) => s.profile && s.verified));
  const full = $derived(picked.length >= app.maxPinned);

  function toggle(id: string) {
    if (picked.includes(id)) picked = picked.filter((p) => p !== id);
    else if (!full) picked = [...picked, id];
  }

  const pickedInfo = $derived(picked.map((id) => listed.find((s) => s.id === id)).filter((s) => s !== undefined));

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
    {#if step === "game"}
      <GetDiablo picked={pickedInfo} onBack={() => (step = "servers")} onFinish={() => app.finishWelcome(picked)} />
    {:else}
      <p class="title text-[11px] tracking-[0.12em] !text-date">STEP 1</p>
      <h1 class="title mt-2 text-[26px]">Welcome</h1>
      <p class="mt-3 max-w-2xl text-[13px] leading-relaxed text-body">
        This launcher installs, updates and starts Diablo II private servers. Each server gets its own folder inside your Diablo II
        install, so your game is never changed and servers don't get in each other's way.
      </p>

      <h2 class="title mt-8 text-[18px]">Pick your servers</h2>
      <p class="mt-3 max-w-2xl text-[13px] leading-relaxed text-body">
        Choose up to {app.maxPinned} to keep in the bar on the left, and switch between them with Ctrl+1 to Ctrl+{Math.min(9, app.maxPinned)}. You can change them any time
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
              class="card pick flex h-full w-full items-start gap-3 p-4 text-left"
              aria-pressed={on}
              disabled={!on && full}
              title={!on && full ? `${app.maxPinned} servers can be pinned` : undefined}
              onclick={() => toggle(s.id)}
            >
              <div class="link-tile flex items-center justify-center">
                {#if data.branding.logo}
                  <img src={data.branding.logo} alt="" class="h-9 w-9 object-contain" />
                {:else}
                  <span class="title text-[13px] !text-[#d9c49a]">{(s.profile?.name ?? s.id).slice(0, 2).toUpperCase()}</span>
                {/if}
              </div>
              <div class="min-w-0 flex-1">
                <p class="title truncate text-[14px] !text-parchment">{s.profile?.name ?? s.id}</p>
                <p class="mt-0.5 text-[11px] text-muted">Diablo II {s.profile?.game.version}</p>
                {#if s.profile?.summary}
                  <p class="mt-2 text-[12px] leading-snug text-body">{s.profile.summary}</p>
                {/if}
              </div>
              <span class={["mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full border", on ? "mark-on" : "border-edge"]}>
                {#if on}<Icon name="check" size={12} />{/if}
              </span>
            </button>
          </li>
        {/each}
      </ul>

      <div class="mt-8 flex items-center gap-4">
        <button class="btn-stone primary h-[38px] px-6 text-[12px]" onclick={() => (step = "game")}>
          {picked.length ? "NEXT" : "SKIP FOR NOW"}
        </button>
      </div>
    {/if}
  </div>
</div>
