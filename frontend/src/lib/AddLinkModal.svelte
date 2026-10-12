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

<div class="fixed inset-0 z-50 flex items-center justify-center bg-[rgba(3,2,2,0.78)]" role="presentation" onclick={(e) => e.target === e.currentTarget && close()}>
  <div
    class="frame-dialog w-[520px] px-[30px] pt-[18px] pb-6"
    role="dialog"
    aria-modal="true"
    aria-labelledby="add-link-title"
    bind:this={dialog}
  >
    {#if link.error}
      <h2 id="add-link-title" class="title text-[19px]">This link can't be used</h2>
      <p class="mt-3 text-[13.5px] text-body">{link.error}</p>
      <div class="mt-6 flex justify-end">
        <button class="btn-stone h-[38px] px-6 text-[12px]" data-autofocus onclick={close}>CLOSE</button>
      </div>
    {:else}
      <h2 id="add-link-title" class="title text-[19px]">Add a server?</h2>
      <p class="mt-3 text-[13.5px] leading-[1.45] text-body">
        A link asks to add the server described at <span class="text-gold">{link.host}</span>. Servers added this way haven't been reviewed for the
        catalog, so only add one you trust.
      </p>
      <p class="mt-3 truncate text-[12px] text-muted" title={link.url}>{link.url}</p>
      {#if error}
        <p class="mt-3 text-[12px] text-bad" role="alert">{error}</p>
      {/if}
      <div class="mt-6 flex justify-end gap-3.5">
        <button
          class="btn-stone h-[38px] px-[22px] text-[12px]"
          data-autofocus
          disabled={adding}
          onclick={close}>CANCEL</button
        >
        <button
          class="btn-stone primary h-[38px] px-[26px] text-[12px]"
          disabled={adding}
          onclick={add}>{adding ? "ADDING…" : "ADD SERVER"}</button
        >
      </div>
    {/if}
  </div>
</div>
