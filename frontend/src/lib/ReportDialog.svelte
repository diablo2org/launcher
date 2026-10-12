<script lang="ts">
  import { SupportService } from "../../bindings/github.com/diablo2org/launcher/internal/app";
  import type { Item } from "../../bindings/github.com/diablo2org/launcher/internal/report";
  import reportIcon from "../assets/ui/link-icon-report.png";
  import { bytes, errorText } from "./format";
  import { app } from "./state.svelte";

  // A bug report for the open server. What goes in is listed before anything
  // leaves: a server with a report URL gets it sent, with only its own files;
  // otherwise it's saved as a zip for the player to pass on.
  const info = $derived(app.current);
  const profile = $derived(info?.profile);
  const launcher = app.report === "launcher";
  const sends = $derived(!launcher && !!profile?.report?.url);
  // What a saved zip holds: the launcher's own files, or this server's.
  const scope = $derived(launcher ? "" : (info?.id ?? ""));

  let items = $state<Item[] | null>(null);
  let host = $state("");
  let message = $state("");
  let contact = $state("");
  let showFiles = $state(false);
  let sending = $state(false);
  let error = $state("");

  const total = $derived((items ?? []).reduce((n, it) => n + it.size, 0));

  async function load() {
    error = "";
    try {
      const sr = sends && info ? await SupportService.ServerReport(info.id) : null;
      host = sr?.host ?? "";
      items = sr ? (sr.items ?? []) : ((await SupportService.ReportContents(scope)) ?? []);
    } catch (err) {
      error = errorText(err);
    }
  }
  load();

  // The outcome goes to a toast and the dialog closes, except a failure that
  // left nothing saved: that stays here so the player can try again.
  async function send() {
    sending = true;
    error = "";
    try {
      const r = await SupportService.SendReport(info!.id, message, contact);
      if (!r.error) {
        app.toast({
          kind: "ok",
          title: "Report sent",
          line: r.id ? `Your reference is ${r.id}; quote it if you ask about it.` : r.message || `The ${profile?.name} team has it.`,
          detail: r.id ? r.message : "",
          fades: !r.id,
        });
        app.report = "";
      } else if (r.savedTo) {
        app.toast({
          kind: "bad",
          title: "Couldn't send the report",
          line: r.error,
          detail: `Saved to ${r.savedTo}, for you to send yourself`,
          action: { label: "Show file", run: () => showSaved(r.savedTo) },
        });
        app.report = "";
      } else {
        error = `Couldn't send the report: ${r.error}`;
      }
    } catch (err) {
      error = errorText(err);
    } finally {
      sending = false;
    }
  }

  async function save() {
    error = "";
    try {
      const path = await SupportService.SaveReport(scope);
      if (!path) return;
      app.toast({ kind: "ok", title: "Report saved", detail: path, action: { label: "Show file", run: () => showSaved(path) } });
      app.report = "";
    } catch (err) {
      error = errorText(err);
    }
  }

  async function showSaved(path: string) {
    try {
      await SupportService.ShowSavedReport(path);
    } catch (err) {
      app.toast({ kind: "bad", title: "Couldn't show the file", line: errorText(err), detail: path });
    }
  }

  // Closing mid-send would lose the result, so the dialog stays until it's in.
  function close() {
    if (!sending) app.report = "";
  }
</script>

<svelte:window onkeydown={(e) => e.key === "Escape" && close()} />

<div class="fixed inset-0 z-40 flex items-start justify-center bg-[rgba(3,2,2,0.78)] pt-14" role="presentation" onclick={(e) => e.target === e.currentTarget && close()}>
  <div class="frame-dialog relative w-[580px] px-[30px] pt-1 pb-6" role="dialog" aria-modal="true" aria-labelledby="report-title">
    <button class="absolute top-0.5 right-0.5 p-1.5 text-muted hover:text-gold disabled:opacity-45" aria-label="Close" disabled={sending} onclick={close}>
      <svg width="14" height="14" viewBox="0 0 14 14" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><path d="M3 3l8 8M11 3l-8 8" /></svg>
    </button>

    <div class="mt-[18px] flex items-center gap-3.5">
      <span class="link-tile !h-10 !w-10" aria-hidden="true"><span class="link-icon" style:--mask={`url(${reportIcon})`}></span></span>
      <div class="min-w-0">
        <h2 id="report-title" class="title text-[19px] !text-title">{launcher ? "Report a launcher problem" : "Report a problem"}</h2>
        <p class="mt-0.5 truncate text-[13px] text-body">
          {#if sends}
            Sent to the {profile?.name} team{host ? " at " : ""}<span class="text-[#d9c49a]">{host}</span>
          {:else if launcher}
            Saved as a zip, for you to send to the launcher's developers
          {:else}
            Saved as a zip, for you to send to {profile?.name ?? "the server"}'s team
          {/if}
        </p>
      </div>
    </div>

    {#if sends}
      <label for="report-what" class="title mt-[22px] mb-1.5 block text-[12px] tracking-[0.08em] !text-[#c9b48a]">WHAT HAPPENED?</label>
      <textarea
        id="report-what"
        class="field h-[104px] w-full resize-none px-3 py-2.5 text-[13.5px] leading-[1.45]"
        placeholder="What were you doing when it went wrong?"
        maxlength="2000"
        bind:value={message}
      ></textarea>

      <label for="report-contact" class="title mt-3.5 mb-1.5 block text-[12px] tracking-[0.08em] !text-[#c9b48a]">
        YOUR DISCORD NAME <span class="font-sans font-normal tracking-normal text-muted">(optional)</span>
      </label>
      <input id="report-contact" class="field h-9 w-[260px] px-3 text-[13.5px]" placeholder="So the team can reply" maxlength="100" bind:value={contact} />
    {:else}
      <p class="mt-5 text-[13.5px] leading-[1.45] text-body">
        The launcher's logs and Diablo II's crash logs are saved as a zip. Nothing is sent anywhere: you choose where to save it and who to give it to.
      </p>
    {/if}

    <div class="mt-4 border border-[#33291b] bg-[rgba(5,4,3,0.55)] px-3 py-2.5">
      {#if items === null && !error}
        <p class="text-[13px] text-muted">Gathering the files…</p>
      {:else if items}
        <button class="flex w-full items-center gap-2 text-left text-[13px] text-[#c9b48a] hover:text-title" aria-expanded={showFiles} onclick={() => (showFiles = !showFiles)}>
          <svg class="transition-transform" class:rotate-90={showFiles} width="10" height="10" viewBox="0 0 10 10" fill="currentColor" aria-hidden="true"><path d="M3 1.5l4 3.5-4 3.5z" /></svg>
          <span class="flex-1">{items.length} {items.length === 1 ? "file goes" : "files go"} with it</span>
          <span class="text-muted">{bytes(total)}</span>
        </button>
        {#if showFiles}
          <ul class="mt-2 ml-[18px] flex max-h-28 flex-col gap-0.5 overflow-y-auto pr-1 text-[12px]">
            {#each items as it (it.name)}
              <li class="flex gap-3">
                <span class="min-w-0 flex-1 truncate text-parchment">{it.name}</span>
                <span class="text-muted">{it.what}</span>
                <span class="w-14 text-right text-muted">{bytes(it.size)}</span>
              </li>
            {/each}
          </ul>
        {/if}
        <p class="mt-1.5 ml-[18px] text-[12px] text-muted">
          Your Windows user folder is replaced with %USERPROFILE%.{host ? " Large files are cut to their newest part to fit." : ""}
        </p>
      {/if}
    </div>

    {#if error}
      <p class="mt-4 text-[13px] text-bad" role="alert">{error}</p>
    {/if}

    <div class="mt-5 flex justify-end gap-3.5">
      <button class="btn-stone h-[38px] px-[22px] text-[12px]" disabled={sending} onclick={close}>CANCEL</button>
      {#if sends}
        <button class="btn-stone primary h-[38px] px-[26px] text-[12px]" disabled={sending || !items} onclick={send}>{sending ? "SENDING…" : "SEND REPORT"}</button>
      {:else}
        <button class="btn-stone primary h-[38px] px-[26px] text-[12px]" disabled={!items} onclick={save}>SAVE REPORT</button>
      {/if}
    </div>
  </div>
</div>
