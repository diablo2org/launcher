<script lang="ts">
  import { ServerService } from "../../bindings/github.com/diablo2org/launcher/internal/app";
  import type { ServerInfo } from "../../bindings/github.com/diablo2org/launcher/internal/core";
  import { bytes, errorText } from "./format";
  import NewsList from "./NewsList.svelte";
  import Select from "./Select.svelte";
  import ServerLinks from "./ServerLinks.svelte";
  import { app, type ServerData } from "./state.svelte";

  let { info, data }: { info: ServerInfo; data: ServerData } = $props();

  const profile = $derived(info.profile);
  const status = $derived(data.status);
  const percent = $derived(data.progress && data.progress.total > 0 ? Math.min(100, (data.progress.done / data.progress.total) * 100) : 0);
  const channels = $derived(profile?.channels ?? []);
  const channel = $derived(channels.find((c) => c.id === data.choices?.channel));

  // Components running the player's own files, named so it's never a surprise.
  const customNames = $derived(
    (profile?.components ?? []).filter((c) => (data.choices?.components ?? {})[c.id] === "custom").map((c) => c.name),
  );

  // The status beside PLAY: one line, then a detail line under it.
  const message = $derived.by(() => {
    if (!app.baseReady) return "Choose your Diablo II folder in settings";
    if (data.busy === "verifying") return "Verifying files…";
    if (data.busy === "updating") return `Updating ${profile?.name ?? "game"}`;
    if (data.launched) return `${profile?.name ?? "Game"} started`;
    if (!status) return data.checking ? "Checking files…" : "";
    if (status.error) return status.error;
    if (!status.installed) return `Not installed · ${bytes(status.updateBytes)} to download`;
    if (!status.upToDate) {
      return `Update available · ${status.updateFiles} ${status.updateFiles === 1 ? "file" : "files"}, ${bytes(status.updateBytes)}`;
    }
    return "Ready to play";
  });
  const failed = $derived(!!status?.error || !!data.error);

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

<div class="relative h-full">
  <div class="pointer-events-none absolute inset-x-0 top-0 flex justify-center pt-8">
    {#if profile?.branding?.backgroundHasLogo && data.branding.background}
      <!-- The server's art already shows its logo. -->
    {:else if data.branding.logo}
      <img src={data.branding.logo} alt={profile?.name ?? ""} class="max-h-64 max-w-[260px] object-contain drop-shadow-[0_4px_24px_rgba(0,0,0,0.8)]" />
    {:else}
      <h1 class="title mt-16 text-4xl drop-shadow-[0_2px_12px_rgba(0,0,0,0.9)]">{profile?.name ?? info.id}</h1>
    {/if}
  </div>

  <!-- News grows down to just above the status line, then scrolls. -->
  <div class={["pointer-events-none absolute right-4 bottom-[118px] flex flex-col items-end", app.tabBar ? "top-4" : "top-[46px]"]}>
    <div class="pointer-events-auto flex min-h-0 flex-col"><NewsList {data} /></div>
  </div>

  <div class="absolute inset-x-0 bottom-0 flex flex-col gap-4 px-7 pb-6">
    {#if data.copyBytes > 0}
      <div class="glass mx-auto max-w-[560px] p-4" role="alertdialog" aria-label="Copy game archives">
        <p class="text-[14px] text-parchment">
          The game archives can't be linked into this server's folder, so they need copying. This uses {bytes(data.copyBytes)} of disk space.
        </p>
        <div class="mt-3 flex gap-3">
          <button class="btn-stone primary h-9 px-4 text-[12px]" onclick={() => data.update(true)}>COPY AND INSTALL</button>
          <button class="btn-stone h-9 px-4 text-[12px]" onclick={() => (data.copyBytes = 0)}>CANCEL</button>
        </div>
      </div>
    {/if}

    <!-- Links on the left, PLAY in the middle, the status on the right, all on one baseline. -->
    <div class="grid grid-cols-[1fr_auto_1fr] items-end gap-6">
      <div class="min-w-0">
        <ServerLinks links={profile?.links} ladder={!!profile?.ladder && !app.tabBar} />
      </div>

      <div class="flex w-[286px] flex-col gap-2.5">
        {#if channels.length > 1 && data.choices}
          <div class="h-9">
            <Select
              label="Channel"
              value={data.choices.channel}
              options={channels.map((ch) => ({ value: ch.id, label: ch.name }))}
              disabled={data.busy !== ""}
              onchange={setChannel}
            />
          </div>
        {/if}
        {#if data.busy === "updating"}
          <div>
            <div class="mb-1.5 flex items-baseline justify-between">
              <span class="title text-[12px] tracking-[0.1em] !text-[#c9b48a]">UPDATING</span>
              <span class="title text-[13px] !text-[#d9b56e]">{Math.floor(percent)}%</span>
            </div>
            <div class="h-2 overflow-hidden border border-black bg-track shadow-[inset_0_1px_2px_rgba(0,0,0,0.8)]" role="progressbar" aria-label="Update progress" aria-valuenow={Math.round(percent)} aria-valuemin="0" aria-valuemax="100">
              <div class="h-full bg-gradient-to-b from-[#e2c98f] to-[#a8875a] transition-[width] duration-200" style:width={`${percent}%`}></div>
            </div>
          </div>
        {/if}
        <button class="btn-play h-[68px] w-full text-[21px]" disabled={!action || data.busy !== "" || data.checking} onclick={() => action?.run()}>
          {#if data.busy === "updating"}
            UPDATING…
          {:else if data.busy === "launching"}
            STARTING…
          {:else if data.busy === "verifying"}
            VERIFYING…
          {:else}
            {(action?.label ?? "…").toUpperCase()}
          {/if}
        </button>
      </div>

      <div class="min-w-0 justify-self-end text-right">
        <p class="title truncate text-[17px] !text-parchment [text-shadow:0_1px_8px_rgba(0,0,0,0.9)]" class:!text-bad={failed} title={data.error || message}>
          {data.error || message}
        </p>
        {#if data.busy === "updating" && data.progress}
          <p class="mt-1.5 truncate text-[13px] text-body">
            {data.progress.file ? `${data.progress.file} · ` : ""}{bytes(data.progress.done)} of {bytes(data.progress.total)}
          </p>
        {:else if customNames.length}
          <p class="mt-1.5 text-[13px] text-[#d9b77a]" title="Your own files; not supported by {profile?.name}. Change in Settings, Game.">
            Custom: {customNames.join(", ")}
          </p>
        {:else}
          <p class="mt-1.5 truncate text-[13px] text-body">{[profile?.name, channel?.name].filter(Boolean).join(" · ")}</p>
        {/if}
      </div>
    </div>
  </div>
</div>
