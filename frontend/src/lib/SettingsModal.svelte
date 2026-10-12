<script lang="ts">
  import { ServerService, SupportService } from "../../bindings/github.com/diablo2org/launcher/internal/app";
  import type { SettingValue } from "../../bindings/github.com/diablo2org/launcher/internal/core";
  import { errorText } from "./format";
  import Select from "./Select.svelte";
  import { app } from "./state.svelte";
  import Toggle from "./Toggle.svelte";

  const info = $derived(app.current);
  const profile = $derived(info?.profile);
  const data = $derived(info ? app.server(info.id) : null);

  // Sections: the launcher's own, then the current server's game options and
  // one per settings group, like the old GAME / D2GL / MAPHACK tabs.
  // A server's own "Game" (or "General") group joins the built-in Game
  // section, below the game options, rather than becoming a second one.
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
  const serverTabs = $derived(profile ? ["Game", ...groups.map(([n]) => n).filter((n) => n !== "Game")] : []);
  const tabs = $derived(["General", ...serverTabs]);
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

  // Up and Down move between sections, as in any vertical tab list.
  function onNavKeydown(e: KeyboardEvent) {
    const i = tabs.indexOf(tab);
    const to = e.key === "ArrowDown" ? i + 1 : e.key === "ArrowUp" ? i - 1 : -1;
    if (to < 0 || to >= tabs.length) return;
    e.preventDefault();
    tab = tabs[to];
    requestAnimationFrame(() => document.querySelector<HTMLElement>(`[data-tab="${tab}"]`)?.focus());
  }
</script>

<!-- An Escape a dropdown already used to close its list leaves Settings open. -->
<svelte:window onkeydown={(e) => e.key === "Escape" && !e.defaultPrevented && close()} />

{#snippet row(title: string, description: string, disabled: boolean)}
  <div class="min-w-0 flex-1" class:opacity-45={disabled}>
    <p class="title text-[14px] !text-parchment">{title}</p>
    {#if description}<p class="mt-1 text-[13px] leading-[1.4] text-body">{description}</p>{/if}
  </div>
{/snippet}

{#snippet navItem(name: string)}
  <button
    role="tab"
    data-tab={name}
    aria-selected={tab === name}
    tabindex={tab === name ? 0 : -1}
    class="nav-item"
    class:on={tab === name}
    onclick={() => (tab = name)}
    onkeydown={onNavKeydown}>{name}</button
  >
{/snippet}

{#snippet settingRow(v: SettingValue)}
  {@const s = v.setting}
  <div class="settings-row" title={v.reason || undefined}>
    {@render row(s.label, (v.available ? s.description : v.reason) ?? "", !v.available)}
    {#if s.type === "bool"}
      <Toggle label={s.label} checked={v.value === true} disabled={!v.available} onchange={(on) => run(() => ServerService.SetSetting(info!.id, s.id, on))} />
    {:else if s.type === "choice"}
      <div class="h-[34px] w-40">
        <Select
          label={s.label}
          value={String(v.value ?? "")}
          options={(s.options ?? []).map((o) => ({ value: String(o.value), label: o.label }))}
          disabled={!v.available}
          onchange={(value) => run(() => ServerService.SetSetting(info!.id, s.id, value))}
        />
      </div>
    {:else if s.type === "int"}
      <input
        type="number"
        class="field h-[34px] w-24 px-2.5 text-[13.5px]"
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
        class="field h-[34px] w-44 px-2.5 text-[13.5px]"
        aria-label={s.label}
        value={v.value}
        maxlength={s.maxLength || undefined}
        disabled={!v.available}
        onchange={(e) => run(() => ServerService.SetSetting(info!.id, s.id, e.currentTarget.value))}
      />
    {/if}
  </div>
{/snippet}

<div class="fixed inset-0 z-40 flex items-center justify-center bg-[rgba(3,2,2,0.78)]" role="presentation" onclick={(e) => e.target === e.currentTarget && close()}>
  <div class="frame-dialog flex h-[540px] w-[860px] flex-col" role="dialog" aria-modal="true" aria-labelledby="settings-title">
    <button class="absolute top-0.5 right-0.5 z-10 p-1.5 text-muted hover:text-gold" aria-label="Close settings" onclick={close}>
      <svg width="14" height="14" viewBox="0 0 14 14" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><path d="M3 3l8 8M11 3l-8 8" /></svg>
    </button>

    <!-- The title starts where the side list's headings do. -->
    <div class="border-b border-gold-line/40 py-3 pr-10 pl-[17px]">
      <h2 id="settings-title" class="title text-[19px] !text-title">Settings</h2>
    </div>

    <div class="flex min-h-0 flex-1">
      <div class="flex w-[190px] shrink-0 flex-col border-r border-gold-line/25 pt-[18px] pb-5">
        <div role="tablist" aria-orientation="vertical" aria-label="Settings sections">
          <p class="nav-head" role="presentation">LAUNCHER</p>
          {@render navItem("General")}
          {#if serverTabs.length}
            <p class="nav-head mt-[22px] truncate" role="presentation">{profile!.name.toUpperCase()}</p>
            {#each serverTabs as t (t)}
              {@render navItem(t)}
            {/each}
          {/if}
        </div>
        <div class="flex-1"></div>
        <p class="title px-[17px] text-[10px] tracking-[0.1em] !text-[#6f6556]">LAUNCHER {app.version}</p>
      </div>

      <div class="flex min-w-0 flex-1 flex-col">
        <div class="min-h-0 flex-1 overflow-y-auto px-[30px] py-1.5" role="tabpanel" aria-label={tab}>
          {#if tab === "General"}
            <p class="mt-2.5 mb-0.5 text-[12.5px] text-muted">These apply to every server.</p>
            <div class="settings-row">
              {@render row(
                "Diablo II folder",
                app.overview?.base
                  ? app.overview.baseError || ((app.overview.report?.missing ?? []).length ? `Missing ${(app.overview.report?.missing ?? []).join(", ")}` : app.overview.base)
                  : "Where Diablo II and Lord of Destruction are installed. It is never modified; each server gets its own folder inside it.",
                false,
              )}
              <button class="btn-stone h-[34px] w-[132px] text-[12px]" onclick={() => app.chooseBase()}>{app.overview?.base ? "CHANGE" : "CHOOSE"}</button>
            </div>
            {#if app.overview?.warning}
              <p class="settings-row text-[12.5px] text-date">{app.overview.warning}</p>
            {/if}

            <div class="settings-row">
              {@render row("Box launch delay", "Wait between starting each box, for every server.", false)}
              <div class="h-[34px] w-[132px]">
                <Select
                  label="Box launch delay"
                  value={String(delay)}
                  options={[1000, 2000, 3000, 4000, 5000].map((ms) => ({ value: String(ms), label: `${ms / 1000} sec` }))}
                  onchange={(value) => {
                    delay = Number(value);
                    run(() => ServerService.SetLaunchDelay(delay));
                  }}
                />
              </div>
            </div>

            <div class="settings-row">
              {@render row("Logs", "The launcher's own log files.", false)}
              <button class="btn-stone h-[34px] w-[132px] text-[12px]" onclick={() => run(() => SupportService.OpenLogs())}>OPEN</button>
            </div>

            <div class="settings-row">
              {@render row("Launcher problem", "Saves the launcher's logs as a zip for its developers. A problem with a server's game goes to its team: use Report on its page.", false)}
              <button
                class="btn-stone h-[34px] w-[132px] text-[12px]"
                onclick={() => {
                  close();
                  app.report = "launcher";
                }}>REPORT</button
              >
            </div>

            <div class="settings-row">
              {@render row("Profile checker", "For server teams: check a server profile against the specification.", false)}
              <button
                class="btn-stone h-[34px] w-[132px] text-[12px]"
                onclick={() => {
                  app.page = "checker";
                  close();
                }}>OPEN</button
              >
            </div>
          {:else if tab === "Game" && profile && data?.choices}
            {#each profile.components ?? [] as c (c.id)}
              {@const custom = (data.choices.components ?? {})[c.id] === "custom"}
              <div class="settings-row flex-col !items-stretch">
                <div class="flex items-center gap-5">
                  {@render row(c.name, custom ? "Using your own files." : "Changes are installed on the next update.", false)}
                  <div class="h-[34px] w-56">
                    <Select
                      label={c.name}
                      value={(data.choices.components ?? {})[c.id] ?? ""}
                      options={[
                        { value: "", label: "Off" },
                        ...(c.versions ?? []).map((v) => ({ value: v.id, label: v.id })),
                        { value: "custom", label: "Custom (my own files)" },
                      ]}
                      onchange={(value) => run(() => ServerService.SetComponent(info!.id, c.id, value))}
                    />
                  </div>
                </div>

                {#if c.kind === "d2gl" && app.overview?.wine?.needed && !(data.choices.components ?? {})[c.id]}
                  <p class="mt-2 text-[13px] text-date" role="note">
                    {c.name} is recommended on this system. Without it the game runs in a window, as full screen under Wine can
                    change your display's resolution.
                  </p>
                {/if}

                {#if custom}
                  <div class="mt-3 flex items-start gap-4 border border-[#6b4a1c] bg-[#1a140b] p-3" role="note">
                    <p class="flex-1 text-[13px] leading-snug text-[#d9b77a]">
                      ⚠ Custom {c.name} isn't supported by {profile.name}. Use at your own risk. The launcher won't install,
                      update, repair or remove these files. Put your own files in the server's folder; choose a version above
                      to go back to {profile.name}'s.
                    </p>
                    <button class="btn-stone h-[34px] shrink-0 px-4 text-[12px]" onclick={() => run(() => ServerService.OpenServerFolder(info!.id))}>OPEN FOLDER</button>
                  </div>
                {/if}
              </div>
            {/each}

            {#if maxBoxes > 1}
              <div class="settings-row">
                {@render row("Boxes to launch", `How many copies of ${profile.name} start when you press Play.`, false)}
                <div class="h-[34px] w-20">
                  <Select
                    label="Boxes to launch"
                    value={String(data.choices.instances)}
                    options={Array.from({ length: maxBoxes }, (_, i) => ({ value: String(i + 1), label: String(i + 1) }))}
                    onchange={(value) => run(() => ServerService.SetInstances(info!.id, Number(value), data!.choices!.splitD2gl))}
                  />
                </div>
              </div>
            {/if}

            {#if d2glOn && maxBoxes > 1}
              <div class="settings-row">
                {@render row("Separate D2GL profiles", "The first box uses d2gl_main.ini and the rest d2gl_loader.ini, so each can keep its own resolution.", false)}
                <Toggle
                  label="Separate D2GL profiles"
                  checked={data.choices.splitD2gl}
                  onchange={(on) => run(() => ServerService.SetInstances(info!.id, data!.choices!.instances, on))}
                />
              </div>

              {#if data.choices.splitD2gl}
                {#each [["Main box resolution", "main"], ["Loader box resolution", "loader"]] as [label, which] (which)}
                  <div class="settings-row">
                    {@render row(label, "Written to that box's D2GL profile before launch. Default leaves it as D2GL has it.", false)}
                    <div class="h-[34px] w-40">
                      <Select
                        {label}
                        value={(which === "main" ? data.choices.d2glMainResolution : data.choices.d2glLoaderResolution) ?? ""}
                        options={[{ value: "", label: "Default" }, ...(data.choices.d2glResolutions ?? []).map((r) => ({ value: r, label: r.replace("x", " × ") }))]}
                        onchange={(value) => {
                          const c = data!.choices!;
                          const main = which === "main" ? value : c.d2glMainResolution;
                          const loader = which === "loader" ? value : c.d2glLoaderResolution;
                          run(() => ServerService.SetD2GLResolutions(info!.id, main, loader));
                        }}
                      />
                    </div>
                  </div>
                {/each}
              {/if}
            {/if}

            {#each gameSettings as v (v.setting.id)}
              {@render settingRow(v)}
            {/each}

            {#if !(profile.components ?? []).length && maxBoxes <= 1 && !gameSettings.length}
              <p class="settings-row text-[13px] text-muted">{profile.name} has no game options.</p>
            {/if}

            <div class="settings-row !border-b-0">
              {@render row(
                "Verify files",
                verifyResult ||
                  (data.status?.installed
                    ? `Check every file against ${profile.name}'s and repair any that differ. Your own settings files and custom components are left alone.`
                    : `Install ${profile.name} first.`),
                !data.status?.installed,
              )}
              <button class="btn-stone h-[34px] w-[132px] text-[12px]" disabled={!data.status?.installed || data.busy !== ""} onclick={verify}>
                {data.busy === "verifying" ? "VERIFYING…" : data.busy === "updating" ? "REPAIRING…" : "VERIFY"}
              </button>
            </div>
          {:else}
            {#each groups.find(([n]) => n === tab)?.[1] ?? [] as v (v.setting.id)}
              {@render settingRow(v)}
            {/each}
          {/if}
        </div>

        <div class="flex items-center gap-4 px-[26px] pt-3 pb-5">
          <p class="min-w-0 flex-1 text-[13px] text-bad" role="alert">{error}</p>
          <button class="btn-stone primary h-[42px] w-[180px] text-[14px] tracking-[0.14em]" onclick={close}>DONE</button>
        </div>
      </div>
    </div>
  </div>
</div>
