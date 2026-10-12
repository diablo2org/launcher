<script lang="ts">
  import { Events, type CancellablePromise } from "@wailsio/runtime";
  import { ServerService, type BaseInstallResult } from "../../bindings/github.com/diablo2org/launcher/internal/app";
  import type { Progress } from "../../bindings/github.com/diablo2org/launcher/internal/basegame";
  import type { BaseSetup, ServerInfo } from "../../bindings/github.com/diablo2org/launcher/internal/core";
  import { onMount } from "svelte";
  import Icon from "./Icon.svelte";
  import LogsLink from "./LogsLink.svelte";
  import RosettaPrompt from "./RosettaPrompt.svelte";
  import Select from "./Select.svelte";
  import StepList from "./StepList.svelte";
  import { bytes, errorText } from "./format";
  import { app } from "./state.svelte";
  import { WineSetup, type Row } from "./wine.svelte";

  // The "Get Diablo II" steps of the welcome screen: install the game, or
  // use the one already installed; then, for an install, the CD keys and
  // the install itself.
  let { picked, onBack, onFinish }: { picked: ServerInfo[]; onBack: () => void; onFinish: () => Promise<void> } = $props();

  let setup = $state<BaseSetup | null>(null);
  let step = $state<"choose" | "keys" | "progress">("choose");
  let choice = $state<"install" | "existing">("install");
  let dir = $state("");
  let version = $state("");
  let advanced = $state(false);
  let classicKey = $state("");
  let expansionKey = $state("");
  let owner = $state("");
  let finishing = $state(false);

  // The install, while it runs and once it's done.
  let running: CancellablePromise<BaseInstallResult> | null = null;
  let cancelled = false;
  let installing = $state(false);
  let progress = $state<Progress | null>(null);
  let result = $state<BaseInstallResult | null>(null);
  let failure = $state("");
  let serverState = $state<"pending" | "active" | "done" | "failed">("pending");
  let cacheCleared = $state(false);

  // On macOS and Linux the game runs under Wine, set up before the download.
  const wineSetup = new WineSetup();

  const o = $derived(app.overview);
  const wineNeeded = $derived(!!o?.wine?.needed && !o?.wine?.ready);
  const first = $derived(picked[0]);
  const firstData = $derived(first ? app.server(first.id) : null);

  // Pre-1.14 and 1.14 clients differ in their archives, so a server only
  // runs on a base of its own family.
  const family = (v: string) => (v.startsWith("1.14") ? "1.14" : "pre-1.14");
  const mismatched = $derived(picked.filter((s) => s.profile && family(s.profile.game.version) !== family(version)));

  const keysReady = $derived(norm(classicKey) !== "" && norm(expansionKey) !== "");
  const total = $derived(choice === "install" ? 4 : 2);
  const stepNumber = $derived(step === "choose" ? 2 : step === "keys" ? 3 : 4);

  onMount(() => {
    ServerService.BaseSetup().then((s) => {
      setup = s;
      dir = s.defaultDir;
      owner = s.owner;
      // 1.13c unless every server picked is a 1.14 one.
      const versions = picked.map((p) => p.profile?.game.version ?? "");
      version = versions.length && versions.every((v) => family(v) === "1.14") && (s.versions ?? []).includes("1.14d") ? "1.14d" : s.defaultVersion;
      // A game the launcher found, or one it can't install: use what's there.
      if (app.baseReady || !s.available) choice = "existing";
    });
  });

  $effect(() => {
    return Events.On("base:progress", (ev: { data: Progress }) => {
      progress = ev.data;
    });
  });

  // Trimmed, upper-cased, without spaces or hyphens: what the launcher
  // stores. No other checks: nobody can check a key any more.
  function norm(key: string): string {
    return key.replace(/[\s-]/g, "").toUpperCase();
  }

  async function chooseFolder() {
    try {
      const picked = await ServerService.ChooseInstallFolder();
      if (picked) dir = picked;
    } catch (err) {
      failure = errorText(err);
    }
  }

  async function install() {
    step = "progress";
    installing = true;
    failure = "";
    result = null;
    progress = null;
    serverState = "pending";
    cancelled = false;

    if (wineNeeded && !wineSetup.done) {
      const ok = await wineSetup.run();
      if (!ok) {
        installing = false;
        if (!wineSetup.needsRosetta && !cancelled) failure = wineSetup.error || "Wine couldn't be set up.";
        if (cancelled) step = "keys";
        return;
      }
    }

    try {
      running = ServerService.InstallBase({ version, dir, classicKey: norm(classicKey), expansionKey: norm(expansionKey), owner: owner.trim() });
      result = await running;
    } catch (err) {
      // Cancelling rejects the call too.
      result = { ok: false, installed: false, cancelled, error: cancelled ? "" : errorText(err), warnings: [], cacheBytes: 0 };
    } finally {
      running = null;
      installing = false;
    }

    if (result.cancelled) {
      step = "keys";
      return;
    }
    if (!result.ok) {
      failure = result.error;
      if (result.installed) choice = "existing";
      return;
    }

    // The new folder is the Diablo II folder now.
    try {
      app.overview = await ServerService.Overview();
    } catch (err) {
      // Installed all the same: Back offers it as an existing install.
      failure = errorText(err);
      choice = "existing";
      return;
    }
    if (firstData) {
      serverState = "active";
      try {
        await firstData.update();
        serverState = firstData.error || firstData.copyBytes > 0 ? "failed" : "done";
      } catch {
        serverState = "failed";
      }
    }
  }

  function cancel() {
    cancelled = true;
    wineSetup.cancel();
    running?.cancel();
  }

  async function clearCache() {
    try {
      await ServerService.ClearBaseCache();
      cacheCleared = true;
    } catch (err) {
      failure = errorText(err);
    }
  }

  async function finish() {
    finishing = true;
    const needsWine = wineNeeded && !wineSetup.done;
    await onFinish();
    finishing = false;
    // An install the player already had still needs Wine set up here.
    if (needsWine && app.baseReady) app.page = "wine";
  }

  // Each row of the progress list: done, under way, waiting, or failed.
  const product = (code: string) => (code === "D2XP" ? "Lord of Destruction" : "Diablo II");
  const rows = $derived.by((): Row[] => {
    const p = progress;
    const ok = result?.ok === true;
    const failed = !installing && !!failure;
    const downloaded = ok || (p !== null && p.product === "D2XP" && p.stage !== "download");

    let downloadDetail = "From Blizzard";
    if (p?.stage === "download") {
      downloadDetail = p.total > 0 ? `${product(p.product)}: ${Math.floor((p.done / p.total) * 100)}%` : `${product(p.product)}: starting`;
    }
    let installDetail = "";
    if (p?.stage === "install") installDetail = `Installing ${product(p.product)}`;
    if (p?.stage === "patch") installDetail = `Patching to ${version}`;

    const wineFirst = wineNeeded && !wineSetup.done;
    const list: Row[] = [
      ...(wineNeeded || wineSetup.done ? wineSetup.rows : []),
      {
        label: "Download",
        state: downloaded ? "done" : failed && !wineFirst ? "failed" : wineFirst ? "pending" : "active",
        detail: downloaded || wineFirst ? "" : downloadDetail,
      },
      {
        label: "Install",
        state: ok ? "done" : failed && downloaded ? "failed" : p && p.stage !== "download" ? "active" : "pending",
        detail: ok ? "" : installDetail,
      },
    ];
    if (first) {
      let detail = "";
      if (serverState === "active" && firstData?.progress?.total) detail = `${bytes(firstData.progress.done)} of ${bytes(firstData.progress.total)}`;
      if (serverState === "failed") detail = firstData?.copyBytes ? "Finish installing it from its page" : firstData?.error ?? "";
      list.push({ label: first.profile?.name ?? first.id, state: serverState, detail });
    }
    return list;
  });

  const allDone = $derived(result?.ok === true && (!first || serverState === "done" || serverState === "failed"));
</script>

<p class="title text-[11px] tracking-[0.12em] !text-date">STEP {stepNumber} OF {total}</p>

{#if step === "choose"}
  <h1 class="title mt-2 text-[26px]">Get Diablo II</h1>
  <p class="mt-3 max-w-2xl text-[13px] leading-relaxed text-body">
    Servers run on top of Diablo II and Lord of Destruction. Each server gets its own folder inside your game, so the game itself
    is never changed.
  </p>

  <div class="mt-6 grid grid-cols-2 gap-3">
    <button
      class="card pick p-5 text-left"
      aria-pressed={choice === "install"}
      disabled={!setup?.available}
      title={setup && !setup.available ? setup.unavailable : undefined}
      onclick={() => (choice = "install")}
    >
      <p class="title text-[14px] !text-parchment">Install Diablo II for me</p>
      <p class="mt-2 text-[12px] leading-snug text-body">Downloads Diablo II and Lord of Destruction from Blizzard (about 2 GB).</p>
    </button>
    <button
      class="card pick p-5 text-left"
      aria-pressed={choice === "existing"}
      onclick={() => (choice = "existing")}
    >
      <p class="title text-[14px] !text-parchment">I already have Diablo II installed</p>
      <p class="mt-2 text-[12px] leading-snug text-body">Use the folder where Diablo II and Lord of Destruction are installed.</p>
    </button>
  </div>

  {#if choice === "install"}
    <section class="mt-4 card p-5">
      <h2 class="title text-[12px] tracking-[0.08em] !text-[#c9b48a]">INSTALL INTO</h2>
      <p class="mt-2 truncate text-[13.5px] text-parchment" title={dir}>{dir}</p>
      <button
        class="btn-stone mt-4 flex h-9 items-center gap-2 px-4 text-[11px]"
        onclick={chooseFolder}
      >
        <Icon name="folder" size={14} />
        CHOOSE ANOTHER FOLDER
      </button>

      <button class="title mt-5 text-[12px] !text-muted hover:!text-title" aria-expanded={advanced} onclick={() => (advanced = !advanced)}>
        {advanced ? "Hide advanced" : "Advanced"}
      </button>
      {#if advanced && setup}
        <div class="mt-3 flex items-center gap-3">
          <span class="text-[12px] text-body">Game version</span>
          <div class="h-8 w-[200px]">
            <Select
              label="Game version"
              value={version}
              options={(setup.versions ?? []).map((v) => ({ value: v, label: v + (v === setup!.defaultVersion ? " (recommended)" : "") }))}
              onchange={(v) => (version = v)}
            />
          </div>
        </div>
        {#if mismatched.length}
          <p class="mt-2 text-[12px] text-bad">
            {mismatched.map((s) => s.profile?.name ?? s.id).join(", ")}
            {mismatched.length === 1 ? "runs" : "run"} on Diablo II {mismatched[0].profile?.game.version}, which won't work with a {version} install.
          </p>
        {/if}
      {/if}
    </section>
  {:else}
    <section class="mt-4 card p-5">
      <h2 class="title text-[12px] tracking-[0.08em] !text-[#c9b48a]">YOUR DIABLO II FOLDER</h2>
      {#if !o?.base}
        <p class="mt-2 text-[13px] text-body">Diablo II wasn't found. Choose the folder where Diablo II and Lord of Destruction are installed.</p>
      {:else}
        <p class="mt-2 truncate text-[13.5px] text-parchment" title={o.base}>{o.base}</p>
        {#if o.baseError}
          <p class="mt-2 text-[12px] text-bad">{o.baseError}</p>
        {:else if o.report?.missing?.length}
          <p class="mt-2 text-[12px] text-bad">
            Missing {o.report.missing.join(", ")}. Install Diablo II and Lord of Destruction there, or choose another folder.
          </p>
        {:else}
          <p class="mt-2 flex items-center gap-1.5 text-[12px] text-date"><Icon name="check" size={13} /> Diablo II and Lord of Destruction found.</p>
        {/if}
      {/if}
      {#if o?.warning}
        <p class="mt-2 text-[12px] text-date">{o.warning}</p>
      {/if}
      <button
        class="btn-stone mt-4 flex h-9 items-center gap-2 px-4 text-[11px]"
        onclick={() => app.chooseBase()}
      >
        <Icon name="folder" size={14} />
        {o?.base ? "CHOOSE ANOTHER FOLDER" : "CHOOSE FOLDER"}
      </button>
    </section>
  {/if}

  {#if failure}
    <p class="mt-4 text-[12px] text-bad">{failure}</p>
    <LogsLink />
  {/if}

  <div class="mt-8 flex items-center gap-4">
    {#if choice === "install"}
      <button
        class="btn-stone primary h-[38px] px-6 text-[12px]"
        disabled={!setup?.available || !dir}
        onclick={() => (step = "keys")}
      >
        NEXT
      </button>
    {:else}
      <button
        class="btn-stone primary h-[38px] px-6 text-[12px]"
        disabled={!app.baseReady || finishing}
        onclick={finish}
      >
        DONE
      </button>
      {#if !app.baseReady}
        <button class="title text-[12px] !text-muted hover:!text-title" onclick={finish}>Skip for now</button>
      {/if}
    {/if}
    <button class="title text-[12px] !text-muted hover:!text-title" onclick={onBack}>Back</button>
  </div>
{:else if step === "keys"}
  <h1 class="title mt-2 text-[26px]">Your CD keys</h1>
  <p class="mt-3 max-w-2xl text-[13px] leading-relaxed text-body">
    Enter the CD keys from your own copies of Diablo II and Lord of Destruction. You'll find them in your Blizzard account under
    Games &amp; Subscriptions. The launcher doesn't check them — nobody can any more — but playing needs a copy you own. Keys are
    stored only inside your game files on this computer.
  </p>

  <section class="mt-6 grid max-w-xl gap-4 card p-5">
    <label class="grid gap-1.5">
      <span class="title text-[11px] tracking-[0.08em] !text-[#c9b48a]">DIABLO II CD KEY</span>
      <input type="text" class="field h-9 px-2.5 font-mono text-[13px] uppercase" autocomplete="off" spellcheck="false" bind:value={classicKey} />
    </label>
    <label class="grid gap-1.5">
      <span class="title text-[11px] tracking-[0.08em] !text-[#c9b48a]">LORD OF DESTRUCTION CD KEY</span>
      <input type="text" class="field h-9 px-2.5 font-mono text-[13px] uppercase" autocomplete="off" spellcheck="false" bind:value={expansionKey} />
    </label>
    <label class="grid gap-1.5">
      <span class="title text-[11px] tracking-[0.08em] !text-[#c9b48a]">OWNER NAME</span>
      <input type="text" class="field h-9 px-2.5 text-[13px]" autocomplete="off" spellcheck="false" bind:value={owner} />
    </label>
  </section>

  <div class="mt-8 flex items-center gap-4">
    <button
      class="btn-stone primary h-[38px] px-6 text-[12px]"
      disabled={!keysReady}
      onclick={install}
    >
      INSTALL
    </button>
    <button class="title text-[12px] !text-muted hover:!text-title" onclick={() => (step = "choose")}>Back</button>
  </div>
{:else}
  <h1 class="title mt-2 text-[26px]">{allDone ? "Ready to play" : "Installing Diablo II"}</h1>
  <p class="mt-3 max-w-2xl text-[13px] leading-relaxed text-body">
    {#if allDone}
      Diablo II is installed in <span class="text-title">{dir}</span>.
    {:else}
      This can take a while. If it stops, installing again carries on where it left off.
    {/if}
  </p>

  <StepList {rows} />
  {#if wineSetup.needsRosetta}
    <!-- install skips Wine, now set up, and carries on with the download. -->
    <RosettaPrompt wine={wineSetup} onReady={install} onFailed={() => (failure = wineSetup.error || "Wine couldn't be set up.")} />
  {/if}

  {#if failure}
    <p class="mt-4 max-w-xl whitespace-pre-line text-[12px] text-bad">{failure}</p>
    <LogsLink />
  {/if}
  {#if result?.warnings?.length}
    <ul class="mt-4 max-w-xl text-[12px] text-date">
      {#each result.warnings as w (w)}
        <li>Note: {w}. A Diablo II key is 16 or 26 letters and digits.</li>
      {/each}
    </ul>
  {/if}

  <div class="mt-8 flex items-center gap-4">
    {#if installing}
      <button class="btn-stone h-[38px] px-6 text-[12px]" onclick={cancel}>CANCEL</button>
    {:else if failure}
      <!-- Once the game is installed, its folder is no longer one to install into. -->
      {#if !result?.installed}
        <button class="btn-stone primary h-[38px] px-6 text-[12px]" onclick={install}>TRY AGAIN</button>
      {/if}
      <button class="title text-[12px] !text-muted hover:!text-title" onclick={() => (step = "choose")}>Back</button>
    {:else if allDone}
      <button class="btn-stone primary h-[38px] px-6 text-[12px]" disabled={finishing} onclick={finish}>
        PLAY
      </button>
      {#if result && result.cacheBytes > 0 && !cacheCleared}
        <button class="title text-[12px] !text-muted hover:!text-title" onclick={clearCache}>
          Free up {bytes(result.cacheBytes)} (delete the download)
        </button>
      {/if}
    {/if}
  </div>
{/if}
