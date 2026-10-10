<script lang="ts">
  import { ServerService, SupportService } from "../../bindings/github.com/diablo2org/launcher/internal/app";
  import type { SendResult } from "../../bindings/github.com/diablo2org/launcher/internal/app";
  import type { SettingValue } from "../../bindings/github.com/diablo2org/launcher/internal/core";
  import type { Item } from "../../bindings/github.com/diablo2org/launcher/internal/report";
  import { bytes, errorText } from "./format";
  import Icon from "./Icon.svelte";
  import { app } from "./state.svelte";
  import Toggle from "./Toggle.svelte";

  const info = $derived(app.current);
  const profile = $derived(info?.profile);
  const data = $derived(info ? app.server(info.id) : null);

  // Tabs: the launcher's own, then the current server's game options and one
  // per settings group, like the old GAME / D2GL / MAPHACK tabs.
  // A server's own "Game" (or "General") group joins the built-in Game tab,
  // below the game options, rather than becoming a second tab of that name.
  const groups = $derived.by(() => {
    const out: [string, SettingValue[]][] = [];
    for (const v of data?.settings ?? []) {
      let name = v.setting.group || "Settings";
      if (["game", "general"].includes(name.toLowerCase())) name = "Game";
      let g = out.find(([n]) => n.toLowerCase() === name.toLowerCase());
      if (!g) out.push((g = [name, []]));
      g[1].push(v);
    }
    return out;
  });

  const gameSettings = $derived(groups.find(([n]) => n === "Game")?.[1] ?? []);
  const tabs = $derived(["General", ...(profile ? ["Game", ...groups.map(([n]) => n).filter((n) => n !== "Game")] : [])]);
  // Opens on the server's game options; falls back to General without one.
  let tab = $state("Game");
  $effect(() => {
    if (!tabs.includes(tab)) tab = "General";
  });

  let error = $state("");
  let delay = $state(2000);
  ServerService.LaunchDelay().then((ms) => (delay = ms));

  async function run(action: () => Promise<unknown>) {
    verifyResult = "";
    error = "";
    try {
      await action();
    } catch (err) {
      error = errorText(err);
    }
    await data?.refresh();
  }

  // A bug report is shown in full before it's saved: nothing goes in that
  // the player hasn't seen listed.
  let reportItems = $state<Item[] | null>(null);
  let reportSaved = $state("");
  // A server with a report URL gets its reports sent rather than saved, and
  // only its own files go in.
  const sendsReports = $derived(!!profile?.report?.url);
  let reportHost = $state("");
  let reportMessage = $state("");
  let reportContact = $state("");
  let sending = $state(false);
  let reportSent = $state<SendResult | null>(null);

  async function previewReport() {
    reportSaved = "";
    reportSent = null;
    await run(async () => {
      const sr = sendsReports && info ? await SupportService.ServerReport(info.id) : null;
      reportHost = sr?.host ?? "";
      reportItems = sr ? sr.items : await SupportService.ReportContents();
    });
  }

  async function sendReport() {
    sending = true;
    try {
      await run(async () => {
        reportSent = await SupportService.SendReport(info!.id, reportMessage, reportContact);
        reportItems = null;
        if (!reportSent.error) reportMessage = "";
      });
    } finally {
      sending = false;
    }
  }

  // Brings the list and its Save button into view when it opens.
  function reveal(node: HTMLElement) {
    node.scrollIntoView({ block: "nearest", behavior: "smooth" });
  }

  async function saveReport() {
    await run(async () => {
      const path = await SupportService.SaveReport();
      if (path) {
        reportSaved = path;
        reportItems = null;
      }
    });
  }

  let verifyResult = $state("");

  async function verify() {
    verifyResult = "";
    error = "";
    try {
      verifyResult = await data!.verify();
    } catch (err) {
      error = errorText(err);
    }
  }

  const maxBoxes = $derived(Math.max(1, profile?.launch?.maxInstances ?? 1));
  const d2glOn = $derived((profile?.components ?? []).some((c) => c.kind === "d2gl" && (data?.choices?.components ?? {})[c.id]));

  function close() {
    app.settingsOpen = false;
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === "Escape") close();
  }
</script>

<svelte:window onkeydown={onKeydown} />

{#snippet row(title: string, description: string, disabled: boolean)}
  <div class="min-w-0 flex-1" class:opacity-45={disabled}>
    <p class="title text-[13px]">{title}</p>
    {#if description}<p class="mt-1 text-[11px] text-muted">{description}</p>{/if}
  </div>
{/snippet}

{#snippet settingRow(v: SettingValue)}
  {@const s = v.setting}
  <div class="flex items-center gap-4 border-b border-line py-4" title={v.reason || undefined}>
    {@render row(s.label, (v.available ? s.description : v.reason) ?? "", !v.available)}
    {#if s.type === "bool"}
      <Toggle label={s.label} checked={v.value === true} disabled={!v.available} onchange={(on) => run(() => ServerService.SetSetting(info!.id, s.id, on))} />
    {:else if s.type === "choice"}
      <select
        class="h-8 w-36 rounded-[3px] border border-edge bg-panel px-2 text-[12px]"
        aria-label={s.label}
        value={v.value}
        disabled={!v.available}
        onchange={(e) => run(() => ServerService.SetSetting(info!.id, s.id, e.currentTarget.value))}
      >
        {#each s.options ?? [] as o (o.value)}
          <option value={o.value}>{o.label}</option>
        {/each}
      </select>
    {:else if s.type === "int"}
      <input
        type="number"
        class="h-8 w-24 rounded-[3px] border border-edge bg-panel px-2 text-[12px]"
        aria-label={s.label}
        value={v.value}
        min={s.min ?? undefined}
        max={s.max ?? undefined}
        disabled={!v.available}
        onchange={(e) => run(() => ServerService.SetSetting(info!.id, s.id, Number(e.currentTarget.value)))}
      />
    {:else}
      <input
        type="text"
        class="h-8 w-44 rounded-[3px] border border-edge bg-panel px-2 text-[12px]"
        aria-label={s.label}
        value={v.value}
        maxlength={s.maxLength || undefined}
        disabled={!v.available}
        onchange={(e) => run(() => ServerService.SetSetting(info!.id, s.id, e.currentTarget.value))}
      />
    {/if}
  </div>
{/snippet}

<div class="fixed inset-0 z-40 flex items-center justify-center bg-black/70" role="presentation" onclick={(e) => e.target === e.currentTarget && close()}>
  <div
    class="flex h-[520px] w-[860px] flex-col rounded border border-edge bg-[#0f0f11] shadow-[0_20px_80px_rgba(0,0,0,0.8)]"
    role="dialog"
    aria-modal="true"
    aria-label="Settings"
  >
    <div class="flex items-center gap-8 border-b border-line px-8 pt-6 pb-4">
      <h2 class="title text-[18px]">{profile ? profile.name : "Settings"}</h2>
      <div class="flex flex-1 gap-6" role="tablist">
        {#each tabs as t (t)}
          <button role="tab" aria-selected={tab === t} class="title text-[13px] uppercase" class:!text-dim={tab !== t} onclick={() => (tab = t)}>
            {t}
          </button>
        {/each}
      </div>
      <button class="rounded p-1.5 text-muted hover:text-title" aria-label="Close settings" onclick={close}>
        <Icon name="close" />
      </button>
    </div>

    <div class="min-h-0 flex-1 overflow-y-auto px-8 py-2">
      {#if tab === "General"}
        <div class="flex items-center gap-4 border-b border-line py-4">
          {@render row(
            "Diablo II folder",
            app.overview?.base
              ? app.overview.baseError || ((app.overview.report?.missing ?? []).length ? `Missing ${(app.overview.report?.missing ?? []).join(", ")}` : app.overview.base)
              : "Where Diablo II and Lord of Destruction are installed. It is never modified; each server gets its own folder inside it.",
            false,
          )}
          <button class="title flex items-center gap-2 rounded-[3px] border border-edge px-4 py-2 text-[12px] hover:border-muted" onclick={() => app.chooseBase()}>
            <Icon name="folder" size={14} /> {app.overview?.base ? "Change" : "Choose"}
          </button>
        </div>
        {#if app.overview?.warning}
          <p class="border-b border-line py-3 text-[11px] text-date">{app.overview.warning}</p>
        {/if}

        <div class="flex items-center gap-4 border-b border-line py-4">
          {@render row("Box launch delay", "Wait between starting each box, for every server.", false)}
          <select
            class="h-8 w-24 rounded-[3px] border border-edge bg-panel px-2 text-[12px]"
            aria-label="Box launch delay"
            value={delay}
            onchange={(e) => {
              delay = Number(e.currentTarget.value);
              run(() => ServerService.SetLaunchDelay(delay));
            }}
          >
            {#each [1000, 2000, 3000, 4000, 5000] as ms}
              <option value={ms}>{ms / 1000} sec</option>
            {/each}
          </select>
        </div>

        <div class="flex items-center gap-4 border-b border-line py-4">
          {@render row("Profile checker", "For server teams: check a server profile against the specification.", false)}
          <button
            class="title flex items-center gap-2 rounded-[3px] border border-edge px-4 py-2 text-[12px] hover:border-muted"
            onclick={() => {
              app.page = "checker";
              close();
            }}
          >
            <Icon name="tool" size={14} /> Open
          </button>
        </div>

        <div class="flex items-center gap-4 border-b border-line py-4">
          {@render row(
            "Bug report",
            sendsReports
              ? `Send the launcher's logs and Diablo II's crash logs to ${profile?.name}, to help them fix a problem.`
              : "Save the launcher's logs and Diablo II's crash logs as a zip, to attach when asking for help.",
            false,
          )}
          <button
            class="title flex items-center gap-2 rounded-[3px] border border-edge px-4 py-2 text-[12px] hover:border-muted"
            onclick={() => run(() => SupportService.OpenLogs())}
          >
            <Icon name="folder" size={14} /> Logs
          </button>
          <button class="title rounded-[3px] border border-edge px-4 py-2 text-[12px] hover:border-muted" onclick={previewReport}>Create</button>
        </div>
        {#if reportItems}
          <div class="border-b border-line py-4 text-[11px]" use:reveal>
            {#if reportHost}
              <p class="text-muted">
                The report holds these files and goes to {reportHost}. Your Windows user folder is replaced with %USERPROFILE%. Large files are cut to their
                newest part to fit.
              </p>
            {:else}
              <p class="text-muted">
                The report holds these files. Your Windows user folder is replaced with %USERPROFILE%. Nothing is sent anywhere: you choose where to save
                it and who to give it to.
              </p>
            {/if}
            <ul class="mt-3 flex flex-col gap-1">
              {#each reportItems as it (it.name)}
                <li class="flex gap-4">
                  <span class="min-w-0 flex-1 truncate">{it.name}</span>
                  <span class="text-muted">{it.what}</span>
                  <span class="w-16 text-right text-muted">{bytes(it.size)}</span>
                </li>
              {/each}
            </ul>
            {#if reportHost}
              <textarea
                class="mt-4 h-20 w-full resize-none rounded-[3px] border border-edge bg-panel p-2 text-[12px]"
                aria-label="What happened"
                placeholder="What happened? What were you doing?"
                maxlength="2000"
                bind:value={reportMessage}
              ></textarea>
              <input
                class="mt-2 h-8 w-64 rounded-[3px] border border-edge bg-panel px-2 text-[12px]"
                aria-label="Contact"
                placeholder="Your name on Discord (optional)"
                maxlength="100"
                bind:value={reportContact}
              />
            {/if}
            <div class="mt-4 flex gap-3">
              {#if reportHost}
                <button
                  class="title rounded-[3px] bg-accent px-4 py-2 text-[12px] !text-white hover:brightness-125 disabled:opacity-45"
                  disabled={sending}
                  onclick={sendReport}>{sending ? "Sending…" : "Send report"}</button
                >
              {:else}
                <button class="title rounded-[3px] bg-accent px-4 py-2 text-[12px] !text-white hover:brightness-125" onclick={saveReport}>Save report</button>
              {/if}
              <button class="title rounded-[3px] border border-edge px-4 py-2 text-[12px] hover:border-muted" onclick={() => (reportItems = null)}>Cancel</button>
            </div>
          </div>
        {/if}
        {#if reportSaved}
          <p class="border-b border-line py-3 text-[11px] text-muted">Saved to {reportSaved}</p>
        {/if}
        {#if reportSent && !reportSent.error}
          <p class="border-b border-line py-3 text-[11px] text-muted">
            Sent{reportSent.id ? `. Your reference is ${reportSent.id}` : ""}.{reportSent.message ? ` ${reportSent.message}` : ""}
          </p>
        {:else if reportSent}
          <div class="flex items-center gap-4 border-b border-line py-3 text-[11px]">
            <p class="min-w-0 flex-1 text-date">
              Couldn't send the report: {reportSent.error}
              {#if reportSent.savedTo}
                <span class="text-muted">It was saved to {reportSent.savedTo}, so you can send it to {profile?.name} yourself.</span>
              {/if}
            </p>
            {#if reportSent.savedTo}
              <button
                class="title flex items-center gap-2 rounded-[3px] border border-edge px-4 py-2 text-[12px] hover:border-muted"
                onclick={() => run(() => SupportService.ShowSavedReport(reportSent!.savedTo))}
              >
                <Icon name="folder" size={14} /> Show
              </button>
            {/if}
          </div>
        {/if}

        <p class="py-4 text-[11px] text-muted">Launcher {app.version}</p>
      {:else if tab === "Game" && profile && data?.choices}
        {#each profile.components ?? [] as c (c.id)}
          {@const custom = (data.choices.components ?? {})[c.id] === "custom"}
          <div class="border-b border-line py-4">
            <div class="flex items-center gap-4">
              {@render row(c.name, custom ? "Using your own files." : "Changes are installed on the next update.", false)}
              <select
                class="h-8 w-56 rounded-[3px] border border-edge bg-panel px-2 text-[12px]"
                aria-label={c.name}
                value={(data.choices.components ?? {})[c.id] ?? ""}
                onchange={(e) => run(() => ServerService.SetComponent(info!.id, c.id, e.currentTarget.value))}
              >
                <option value="">Off</option>
                {#each c.versions ?? [] as v (v.id)}
                  <option value={v.id}>{v.id}</option>
                {/each}
                <option value="custom">Custom (my own files)</option>
              </select>
            </div>

            {#if custom}
              <div class="mt-3 flex items-start gap-4 rounded-[3px] border border-[#6b4a1c] bg-[#1a140b] p-3" role="note">
                <p class="flex-1 text-[12px] leading-snug text-[#d9b77a]">
                  ⚠ Custom {c.name} isn't supported by {profile.name}. Use at your own risk. The launcher won't install,
                  update, repair or remove these files. Put your own files in the server's folder; choose a version above
                  to go back to {profile.name}'s.
                </p>
                <button
                  class="title flex shrink-0 items-center gap-2 rounded-[3px] border border-edge px-3 py-1.5 text-[12px] hover:border-muted"
                  onclick={() => run(() => ServerService.OpenServerFolder(info!.id))}
                >
                  <Icon name="folder" size={14} /> Open folder
                </button>
              </div>
            {/if}
          </div>
        {/each}

        {#if maxBoxes > 1}
          <div class="flex items-center gap-4 border-b border-line py-4">
            {@render row("Boxes to launch", `How many copies of ${profile.name} start when you press Play.`, false)}
            <select
              class="h-8 w-20 rounded-[3px] border border-edge bg-panel px-2 text-[12px]"
              aria-label="Boxes to launch"
              value={data.choices.instances}
              onchange={(e) => run(() => ServerService.SetInstances(info!.id, Number(e.currentTarget.value), data!.choices!.splitD2gl))}
            >
              {#each Array.from({ length: maxBoxes }, (_, i) => i + 1) as n}
                <option value={n}>{n}</option>
              {/each}
            </select>
          </div>
        {/if}

        {#if d2glOn && maxBoxes > 1}
          <div class="flex items-center gap-4 border-b border-line py-4">
            {@render row("Separate D2GL profiles", "The first box uses d2gl_main.ini and the rest d2gl_loader.ini, so each can keep its own resolution.", false)}
            <Toggle
              label="Separate D2GL profiles"
              checked={data.choices.splitD2gl}
              onchange={(on) => run(() => ServerService.SetInstances(info!.id, data!.choices!.instances, on))}
            />
          </div>

          {#if data.choices.splitD2gl}
            {#each [["Main box resolution", "main"], ["Loader box resolution", "loader"]] as [label, which] (which)}
              <div class="flex items-center gap-4 border-b border-line py-4">
                {@render row(label, "Written to that box's D2GL profile before launch. Default leaves it as D2GL has it.", false)}
                <select
                  class="h-8 w-36 rounded-[3px] border border-edge bg-panel px-2 text-[12px]"
                  aria-label={label}
                  value={which === "main" ? data.choices.d2glMainResolution : data.choices.d2glLoaderResolution}
                  onchange={(e) => {
                    const c = data!.choices!;
                    const main = which === "main" ? e.currentTarget.value : c.d2glMainResolution;
                    const loader = which === "loader" ? e.currentTarget.value : c.d2glLoaderResolution;
                    run(() => ServerService.SetD2GLResolutions(info!.id, main, loader));
                  }}
                >
                  <option value="">Default</option>
                  {#each data.choices.d2glResolutions ?? [] as r (r)}
                    <option value={r}>{r.replace("x", " × ")}</option>
                  {/each}
                </select>
              </div>
            {/each}
          {/if}
        {/if}

        {#each gameSettings as v (v.setting.id)}
          {@render settingRow(v)}
        {/each}

        {#if !(profile.components ?? []).length && maxBoxes <= 1 && !gameSettings.length}
          <p class="border-b border-line py-4 text-[12px] text-muted">{profile.name} has no game options.</p>
        {/if}

        <div class="flex items-center gap-4 py-4">
          {@render row(
            "Verify files",
            verifyResult ||
              (data.status?.installed
                ? `Check every file against ${profile.name}'s and repair any that differ. Your own settings files and custom components are left alone.`
                : `Install ${profile.name} first.`),
            !data.status?.installed,
          )}
          <button
            class="title rounded-[3px] border border-edge px-4 py-2 text-[12px] hover:border-muted disabled:opacity-45 disabled:hover:border-edge"
            disabled={!data.status?.installed || data.busy !== ""}
            onclick={verify}
          >
            {data.busy === "verifying" ? "Verifying…" : data.busy === "updating" ? "Repairing…" : "Verify"}
          </button>
        </div>
      {:else}
        {#each groups.find(([n]) => n === tab)?.[1] ?? [] as v (v.setting.id)}
          {@render settingRow(v)}
        {/each}
      {/if}
    </div>

    <div class="flex items-center justify-between border-t border-line px-8 py-3">
      <p class="text-[12px] text-bad" role="alert">{error}</p>
      <button class="title rounded-[3px] bg-accent px-6 py-2 text-[12px] !text-white hover:brightness-125" onclick={close}>Done</button>
    </div>
  </div>
</div>
