<script lang="ts">
  import { SupportService } from "../../bindings/github.com/diablo2org/launcher/internal/app";
  import { errorText } from "./format";

  // Beside a set-up failure: the launcher's logs say what went wrong, and on
  // the welcome pages Settings, where they otherwise live, isn't reachable.

  let error = $state("");

  async function open() {
    error = "";
    try {
      await SupportService.OpenLogs();
    } catch (err) {
      error = errorText(err);
    }
  }
</script>

<button class="mt-1.5 block text-[12px] text-gold underline decoration-gold-line underline-offset-2 hover:text-[#fff1c8]" onclick={open}>
  Open the launcher's logs
</button>
{#if error}<p class="mt-1 text-[12px] text-bad" role="alert">{error}</p>{/if}
