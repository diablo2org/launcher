<script lang="ts">
  import { Browser } from "@wailsio/runtime";
  import { ServerService } from "../../bindings/github.com/diablo2org/launcher/internal/app";
  import type { ServerInfo } from "../../bindings/github.com/diablo2org/launcher/internal/core";
  import { bytes, errorText } from "./format";
  import NewsList from "./NewsList.svelte";
  import ServerLinks from "./ServerLinks.svelte";
  import { app, type ServerData } from "./state.svelte";

  let { info, data }: { info: ServerInfo; data: ServerData } = $props();

  const profile = $derived(info.profile);
  const status = $derived(data.status);
  const percent = $derived(data.progress && data.progress.total > 0 ? Math.min(100, (data.progress.done / data.progress.total) * 100) : 0);
  const channels = $derived(profile?.channels ?? []);

  // Components running the player's own files, named so it's never a surprise.
  const customNames = $derived(
    (profile?.components ?? []).filter((c) => (data.choices?.components ?? {})[c.id] === "custom").map((c) => c.name),
  );

  // The one line of text beside the button.
  const message = $derived.by(() => {
    if (!app.baseReady) return "Choose your Diablo II folder in settings";
    if (data.busy === "verifying") return "Verifying files…";
    if (data.busy === "updating") return data.progress?.file ? `Updating ${data.progress.file}` : "Updating…";
    if (data.launched) return `${profile?.name ?? "Game"} started`;
    if (!status) return data.checking ? "Checking files…" : "";
    if (status.error) return status.error;
    if (!status.installed) return `Not installed · ${bytes(status.updateBytes)} to download`;
    if (!status.upToDate) {
      return `Update available · ${status.updateFiles} ${status.updateFiles === 1 ? "file" : "files"}, ${bytes(status.updateBytes)}`;
    }
    return "Game is up to date";
  });

  const action = $derived.by((): { label: string; run: () => void } | null => {
    if (!app.baseReady) return { label: "Settings", run: () => (app.settingsOpen = true) };
    if (!status) return null;
    if (status.error) return { label: "Retry", run: () => data.refresh() };
    if (!status.installed) return { label: "Install", run: () => data.update() };
    if (!status.upToDate) return { label: "Update", run: () => data.update() };
    return { label: "Play", run: () => data.play() };
  });

  async function setChannel(channel: string) {
    try {
      await ServerService.SetChannel(info.id, channel);
    } catch (err) {
      data.error = errorText(err);
    }
    data.refresh();
  }
</script>

<div class="flex h-full">
  <div class="relative flex min-w-0 flex-1 flex-col">
    <div class="flex flex-1 items-start justify-center pt-8">
      {#if profile?.branding?.backgroundHasLogo && data.branding.background}
        <!-- The server's art already shows its logo. -->
      {:else if data.branding.logo}
        <img src={data.branding.logo} alt={profile?.name ?? ""} class="max-h-64 max-w-[260px] object-contain drop-shadow-[0_4px_24px_rgba(0,0,0,0.8)]" />
      {:else}
        <h1 class="title mt-16 text-4xl drop-shadow-[0_2px_12px_rgba(0,0,0,0.9)]">{profile?.name ?? info.id}</h1>
      {/if}
    </div>

    {#if data.copyBytes > 0}
      <div class="mx-8 mb-4 rounded border border-edge bg-panel/95 p-4" role="alertdialog" aria-label="Copy game archives">
        <p class="text-[13px] text-title">
          The game archives can't be linked into this server's folder, so they need copying. This uses {bytes(data.copyBytes)} of disk space.
        </p>
        <div class="mt-3 flex gap-2">
          <button class="title rounded bg-accent px-4 py-1.5 text-[12px] !text-white hover:brightness-125" onclick={() => data.update(true)}>Copy and install</button>
          <button class="title rounded border border-edge px-4 py-1.5 text-[12px]" onclick={() => (data.copyBytes = 0)}>Cancel</button>
        </div>
      </div>
    {/if}

    <div class="shrink-0 px-12">
      <ServerLinks links={profile?.links} />
    </div>

    <div class="flex h-[120px] shrink-0 items-end gap-8 px-12 pb-6">
      <div class="min-w-0 flex-1 pb-3">
        {#if data.busy === "updating"}
          <div class="mb-3 h-2 overflow-hidden rounded-[3px] border border-black bg-track" role="progressbar" aria-valuenow={Math.round(percent)} aria-valuemin="0" aria-valuemax="100">
            <div class="h-full bg-accent transition-[width] duration-200" style:width={`${percent}%`}></div>
          </div>
        {/if}
        <p class="truncate text-[15px] text-title" class:!text-bad={!!status?.error || !!data.error} title={data.error || message}>
          {data.error || message}
        </p>
        {#if data.busy === "updating" && data.progress}
          <p class="mt-1 text-[12px] text-muted">{bytes(data.progress.done)} of {bytes(data.progress.total)}</p>
        {:else if customNames.length}
          <p class="mt-1 text-[12px] text-[#d9b77a]" title="Your own files; not supported by {profile?.name}. Change in Settings, Game.">
            Custom: {customNames.join(", ")}
          </p>
        {/if}
      </div>

      <div class="flex w-[272px] shrink-0 flex-col gap-2">
        {#if channels.length > 1 && data.choices}
          <select
            class="h-9 w-full rounded-[3px] border border-edge bg-panel/95 px-3 text-[12px] font-bold text-title"
            aria-label="Channel"
            value={data.choices.channel}
            disabled={data.busy !== ""}
            onchange={(e) => setChannel(e.currentTarget.value)}
          >
            {#each channels as ch (ch.id)}
              <option value={ch.id}>{ch.name}</option>
            {/each}
          </select>
        {/if}
        <button
          class="title h-12 w-full rounded-[3px] bg-accent text-[15px] !text-white shadow-[0_0_24px_rgba(0,0,0,0.6)] transition hover:brightness-125 disabled:opacity-60 disabled:hover:brightness-100"
          disabled={!action || data.busy !== "" || data.checking}
          onclick={() => action?.run()}
        >
          {#if data.busy === "updating"}
            Updating…
          {:else if data.busy === "launching"}
            Starting…
          {:else if data.busy === "verifying"}
            Verifying…
          {:else}
            {action?.label ?? "…"}
          {/if}
        </button>
      </div>
    </div>
  </div>

  <aside class="relative flex w-[350px] shrink-0 flex-col border-l border-line bg-ink/40">
    <div class="min-h-0 flex-1">
      <NewsList {data} />
    </div>
    {#if app.update}
      {@const u = app.launcherUpdate}
      <div class="absolute right-4 bottom-3 flex items-center gap-3 text-[10px]">
        {#if app.updateError}
          <span class="text-bad" title={app.updateError}>Update failed</span>
        {/if}
        {#if u}
          <span class="title !text-date" role="status">
            {u.total && u.done >= u.total ? "Starting installer…" : `Downloading ${app.update.version} · ${u.total ? Math.floor((u.done / u.total) * 100) : 0}%`}
          </span>
        {:else if app.update.installer}
          <button
            class="title !text-date hover:!text-title"
            title={`Downloads the ${app.update.version} installer (${bytes(app.update.installer.size)}), checks it, and runs it. The launcher closes while it installs.`}
            onclick={() => app.installUpdate()}
          >
            Update to {app.update.version}
          </button>
          <button class="title !text-muted hover:!text-title" title="What's new" aria-label="What's new" onclick={() => Browser.OpenURL(app.update!.url)}>↗</button>
        {:else}
          <button
            class="title !text-date hover:!text-title"
            title="A newer launcher is available. Opens the download page."
            onclick={() => Browser.OpenURL(app.update!.url)}
          >
            {app.update.version} available ↗
          </button>
        {/if}
      </div>
    {:else}
      <p class="title absolute right-4 bottom-3 text-[10px] !text-muted">{app.version}</p>
    {/if}
  </aside>
</div>
