<script lang="ts">
  import { app } from "./state.svelte";

  // A diablo2org:// link from a web page or chat. Nothing is fetched until the
  // player agrees: the link only names where the profile is.
  const link = $derived(app.link!);
  let error = $state("");
  let adding = $state(false);
  let dialog: HTMLDivElement;

  async function add() {
    const approved = link;
    adding = true;
    error = await app.addFromLink(approved.url);
    adding = false;
    if (!error) app.closeLink(approved);
  }

  function close() {
    if (!adding) app.closeLink(link);
  }

  // Focus starts on Cancel (or Close), so a stray Enter can't add a server,
  // stays inside the dialog, and goes back where it was when it closes.
  $effect(() => {
    const before = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    dialog.querySelector<HTMLElement>("[data-autofocus]")?.focus();
    return () => before?.focus();
  });

  function onKeydown(e: KeyboardEvent) {
    if (e.key === "Escape") {
      close();
      return;
    }
    if (e.key !== "Tab") return;

    const stops = [...dialog.querySelectorAll<HTMLElement>("button:not([disabled])")];
    if (!stops.length) {
      e.preventDefault();
      return;
    }
    const first = stops[0];
    const last = stops[stops.length - 1];
    const inside = dialog.contains(document.activeElement);
    if (e.shiftKey && (!inside || document.activeElement === first)) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && (!inside || document.activeElement === last)) {
      e.preventDefault();
      first.focus();
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/70" role="presentation" onclick={(e) => e.target === e.currentTarget && close()}>
  <div
    class="w-[520px] rounded border border-edge bg-[#0f0f11] px-8 py-6 shadow-[0_20px_80px_rgba(0,0,0,0.8)]"
    role="dialog"
    aria-modal="true"
    aria-labelledby="add-link-title"
    bind:this={dialog}
  >
    {#if link.error}
      <h2 id="add-link-title" class="title text-[18px]">This link can't be used</h2>
      <p class="mt-3 text-[12px] text-muted">{link.error}</p>
      <div class="mt-6 flex justify-end">
        <button class="title rounded-[3px] border border-edge px-4 py-2 text-[12px] hover:border-muted" data-autofocus onclick={close}>Close</button>
      </div>
    {:else}
      <h2 id="add-link-title" class="title text-[18px]">Add a server?</h2>
      <p class="mt-3 text-[12px] text-muted">
        A link asks to add the server described at <span class="text-title">{link.host}</span>. Servers added this way haven't been reviewed for the
        catalog, so only add one you trust.
      </p>
      <p class="mt-3 truncate text-[11px] text-dim" title={link.url}>{link.url}</p>
      {#if error}
        <p class="mt-3 text-[12px] text-bad" role="alert">{error}</p>
      {/if}
      <div class="mt-6 flex justify-end gap-3">
        <button
          class="title rounded-[3px] border border-edge px-4 py-2 text-[12px] hover:border-muted disabled:opacity-45"
          data-autofocus
          disabled={adding}
          onclick={close}>Cancel</button
        >
        <button
          class="title rounded-[3px] bg-accent px-4 py-2 text-[12px] !text-white hover:brightness-125 disabled:opacity-45"
          disabled={adding}
          onclick={add}>{adding ? "Adding…" : "Add server"}</button
        >
      </div>
    {/if}
  </div>
</div>
