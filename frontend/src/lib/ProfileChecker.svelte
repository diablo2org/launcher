<script lang="ts">
  import { ProfileService, type CheckResult } from "../../bindings/github.com/d2org/launcher/internal/app";

  // A tool for server teams: check a profile before submitting it.
  let profile = $state("");
  let result = $state<CheckResult | null>(null);
  let checking = $state(false);
  // A nil slice in Go arrives as null.
  let problems = $derived(result?.problems ?? []);

  async function check() {
    if (profile.trim() === "") return;

    checking = true;
    try {
      result = await ProfileService.Check(profile);
    } catch (err) {
      result = { ok: false, name: "", problems: [String(err)] };
    } finally {
      checking = false;
    }
  }

  async function loadExample() {
    profile = await ProfileService.Example();
    result = null;
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
      event.preventDefault();
      check();
    }
  }
</script>

<section class="flex h-full flex-col gap-4 p-6">
  <header>
    <h1 class="text-xl font-semibold tracking-wide">Profile checker</h1>
    <p class="mt-1 text-sm text-muted">
      For server teams: paste a server profile to check it against the specification. Every problem is listed at once.
    </p>
  </header>

  <div class="grid min-h-0 flex-1 grid-cols-[3fr_2fr] gap-4">
    <div class="flex min-h-0 flex-col gap-3">
      <textarea
        class="min-h-0 flex-1 resize-none rounded border border-line bg-panel p-3 font-mono text-xs leading-relaxed outline-none focus:border-accent"
        placeholder="Paste profile.json here"
        spellcheck="false"
        aria-label="Server profile JSON"
        bind:value={profile}
        onkeydown={onKeydown}
      ></textarea>

      <div class="flex gap-2">
        <button
          class="rounded bg-accent px-4 py-2 text-sm font-medium hover:brightness-110 disabled:opacity-50"
          onclick={check}
          disabled={checking || profile.trim() === ""}
        >
          {checking ? "Checking…" : "Check"}
        </button>
        <button class="rounded border border-line px-4 py-2 text-sm hover:border-muted" onclick={loadExample}>
          Load example
        </button>
        <span class="self-center text-xs text-muted">Ctrl+Enter to check</span>
      </div>
    </div>

    <div class="min-h-0 overflow-auto rounded border border-line bg-panel p-4" aria-live="polite">
      {#if result === null}
        <p class="text-sm text-muted">Results appear here.</p>
      {:else if result.ok}
        <p class="font-medium text-ok">✓ {result.name} is a valid profile.</p>
      {:else}
        <p class="font-medium text-bad">
          {problems.length === 1 ? "1 problem" : `${problems.length} problems`}
        </p>
        <ul class="mt-3 space-y-2">
          {#each problems as problem}
            <li class="border-l-2 border-bad pl-3 font-mono text-xs leading-relaxed whitespace-pre-wrap">{problem}</li>
          {/each}
        </ul>
      {/if}
    </div>
  </div>
</section>
