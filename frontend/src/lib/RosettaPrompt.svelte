<script lang="ts">
  import type { WineSetup } from "./wine.svelte";

  // Apple Silicon Macs run the game's Wine through Rosetta 2, which macOS
  // installs only when asked.
  // onReady runs once Rosetta is installed and Wine set up, so the page
  // that showed the prompt carries on; onFailed runs when either fails, with
  // the reason in wine.error.
  let { wine, onReady, onFailed }: { wine: WineSetup; onReady: () => void; onFailed?: () => void } = $props();

  async function install() {
    if (await wine.installRosetta()) onReady();
    else onFailed?.();
  }
</script>

<section class="card mt-6 max-w-xl p-5">
  <h2 class="title text-[14px]">Rosetta 2 is needed</h2>
  <p class="mt-2 text-[13px] leading-relaxed text-body">
    Diablo II is a Windows game. On this Mac it runs through Wine, which needs Apple's Rosetta 2 to run on Apple silicon. macOS
    will ask for your password to install it.
  </p>
  <button
    class="btn-stone primary h-[38px] px-6 text-[12px] mt-4"
    disabled={wine.installingRosetta}
    onclick={install}
  >
    {wine.installingRosetta ? "INSTALLING ROSETTA…" : "INSTALL ROSETTA"}
  </button>
</section>
