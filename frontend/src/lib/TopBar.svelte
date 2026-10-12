<script lang="ts">
  import { Browser, Window } from "@wailsio/runtime";
  import Icon from "./Icon.svelte";
  import { app } from "./state.svelte";

  // Community goes to the server's own place: Discord first, then its forum,
  // then its website.
  const community = $derived.by(() => {
    const links = app.current?.profile?.links;
    return links?.discord || links?.forum || links?.website || "";
  });

  // Kept in step however the window changes: this button, double-clicking
  // the bar, snapping, or Windows+Up.
  // Replies can arrive out of order, so only the newest request's counts.
  let maximised = $state(false);
  let latest = 0;
  async function checkMaximised() {
    const asked = ++latest;
    const m = await Window.IsMaximised();
    if (asked === latest) maximised = m;
  }
  checkMaximised();

  // Wails doesn't always report a maximise as a page resize, so check after
  // toggling too.
  async function toggleMaximised() {
    await Window.ToggleMaximise();
    await checkMaximised();
  }
</script>

{#snippet item(label: string, active: boolean, onclick: () => void, external = false)}
  <button
    class="no-drag title relative flex items-center gap-1.5 text-[14px] transition-colors"
    class:text-title={active}
    class:!text-dim={!active}
    class:hover:!text-muted={!active}
    aria-current={active ? "page" : undefined}
    {onclick}
  >
    {label}
    {#if external}
      <span class="opacity-40"><Icon name="out" size={12} /></span>
    {/if}
  </button>
{/snippet}

<svelte:window onresize={checkMaximised} />

<header
  class={[
    "drag flex items-center gap-10 pr-3",
    app.tabBar ? "h-20 shrink-0 border-b border-line pl-8" : "absolute inset-x-0 top-0 z-30 h-10",
  ]}
>
  {#if app.tabBar}
    {@render item("Launch", app.page === "launch", () => (app.page = "launch"))}
    {#if app.current?.profile?.ladder}
      {@render item("Ladder", app.page === "ladder", () => (app.page = "ladder"))}
    {/if}
    {#if community}
      {@render item("Community", false, () => Browser.OpenURL(community), true)}
    {/if}
  {/if}
  <!-- The launcher's own pages title themselves. -->

  <div class="flex-1"></div>

  <!-- No Settings on the welcome pages: nothing there is needed before set-up
       ends, and the profile checker would leave the flow. Settings is on the rail. -->
  <div class={["no-drag ml-2 flex self-start", app.tabBar ? "pt-1" : "pt-0.5"]}>
    <button class="rounded p-2 text-muted hover:text-title" aria-label="Minimise" onclick={() => Window.Minimise()}>
      <Icon name="min" size={16} />
    </button>
    <button
      class="rounded p-2 text-muted hover:text-title"
      aria-label={maximised ? "Restore" : "Maximise"}
      onclick={toggleMaximised}
    >
      <Icon name={maximised ? "restore" : "max"} size={16} />
    </button>
    <button class="rounded p-2 text-muted hover:text-bad" aria-label="Close" onclick={() => Window.Close()}>
      <Icon name="close" size={16} />
    </button>
  </div>
</header>
