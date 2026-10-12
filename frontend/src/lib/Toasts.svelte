<script lang="ts">
  import { Browser } from "@wailsio/runtime";
  import { bytes } from "./format";
  import { app } from "./state.svelte";
  import Toast from "./Toast.svelte";

  // A newer launcher stays on show (it comes from the update check, not an
  // event), unless closed for this run. While it downloads it can't be closed.
  const u = $derived(app.launcherUpdate);
  const showUpdate = $derived(!!app.update && (!app.updateHidden || !!u));
  const linkClass = "text-gold underline decoration-gold-line underline-offset-2 hover:text-[#fff1c8]";
</script>

<!-- Top centre of the page area, above dialogs. -->
<div class="pointer-events-none absolute inset-x-0 top-3 z-50 flex flex-col items-center gap-2" aria-live="polite">
  {#if showUpdate && app.update}
    {@const r = app.update}
    {#if u}
      <Toast kind="info" title={u.total && u.done >= u.total ? "Starting the installer…" : `Downloading launcher ${r.version}`}>
        {u.total ? `${Math.floor((u.done / u.total) * 100)}% · ${bytes(u.done)} of ${bytes(u.total)}` : "Starting…"} · the launcher closes while it installs.
      </Toast>
    {:else}
      <Toast kind={app.updateError ? "bad" : "info"} title={app.updateError ? "The launcher update failed" : `Launcher ${r.version} is ${r.installer ? "ready" : "out"}`} onclose={() => (app.updateHidden = true)}>
        {#if app.updateError}<span class="block truncate" title={app.updateError}>{app.updateError}</span>{/if}
        {#if r.installer}
          <button class={linkClass} title={`Downloads the ${r.version} installer (${bytes(r.installer.size)}), checks it, and runs it.`} onclick={() => app.installUpdate()}>
            {app.updateError ? "Try again" : "Update and restart"}
          </button>
        {:else if r.download}
          <!-- macOS and Linux: the new package downloads in the browser, to install as the first. -->
          <button class={linkClass} title={`Downloads ${r.version} in your browser. Install it as you did this one.`} onclick={() => Browser.OpenURL(r.download)}>Download</button>
        {/if}
        {#if r.installer || r.download}
          <span class="text-dim">·</span>
          <button class={linkClass} onclick={() => Browser.OpenURL(r.url)}>What's new ↗</button>
        {:else}
          <button class={linkClass} onclick={() => Browser.OpenURL(r.url)}>Open the download page ↗</button>
        {/if}
      </Toast>
    {/if}
  {/if}

  {#each app.toasts as t (t.id)}
    <Toast kind={t.kind} title={t.title} fades={t.fades} onclose={() => app.dismiss(t.id)}>
      {#if t.line}<span class="block">{t.line}</span>{/if}
      {#if t.detail}<span class="block truncate text-muted" title={t.detail}>{t.detail}</span>{/if}
      {#if t.action}
        {@const a = t.action}
        <button class={linkClass} onclick={a.run}>{a.label}</button>
      {/if}
    </Toast>
  {/each}
</div>
