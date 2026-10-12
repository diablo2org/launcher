<script lang="ts">
  import { ProfileService, type CheckResult } from "../../bindings/github.com/diablo2org/launcher/internal/app";

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

<section class="flex h-full flex-col gap-4 px-9 pt-[30px] pb-6">
  <header>
    <h1 class="title text-[20px] tracking-[0.06em]">Profile checker</h1>
    <p class="mt-1.5 text-[13.5px] text-body">
      For server teams: paste a server profile to check it against the specification. Every problem is listed at once.
    </p>
  </header>

  <div class="grid min-h-0 flex-1 grid-cols-[3fr_2fr] gap-4">
    <div class="flex min-h-0 flex-col gap-3">
      <textarea
        class="field min-h-0 flex-1 resize-none p-3 font-mono text-xs leading-relaxed"
        placeholder="Paste profile.json here"
        spellcheck="false"
        aria-label="Server profile JSON"
        bind:value={profile}
        onkeydown={onKeydown}
      ></textarea>

      <div class="flex gap-2">
        <button
          class="btn-stone primary h-9 px-5 text-[12px]"
          onclick={check}
          disabled={checking || profile.trim() === ""}
        >
          {checking ? "CHECKING…" : "CHECK"}
        </button>
        <button class="btn-stone h-9 px-4 text-[12px]" onclick={loadExample}>
          LOAD EXAMPLE
        </button>
        <span class="self-center text-[12px] text-muted">Ctrl+Enter to check</span>
      </div>
    </div>

    <div class="card min-h-0 overflow-auto p-4" aria-live="polite">
      {#if result === null}
        <p class="text-[13px] text-muted">Results appear here.</p>
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
