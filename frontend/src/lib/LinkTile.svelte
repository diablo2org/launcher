<script lang="ts" module>
  import discord from "../assets/ui/link-icon-discord.png";
  import donate from "../assets/ui/link-icon-donate.png";
  import report from "../assets/ui/link-icon-report.png";

  // Icon art, how much of the tile it fills (some fill more of the opening than
  // others), and the colour the face takes on hover.
  const kinds: Record<string, { icon: string; scale?: string; tint: string }> = {
    discord: { icon: discord, scale: "86%", tint: "rgba(88, 101, 242, 0.38)" },
    donate: { icon: donate, scale: "86%", tint: "rgba(228, 172, 38, 0.5)" },
    report: { icon: report, tint: "rgba(46, 125, 62, 0.42)" },
  };
</script>

<script lang="ts">
  import Icon from "./Icon.svelte";

  let {
    kind,
    label,
    title,
    glyph = "out",
    onclick,
  }: { kind: string; label: string; title?: string; glyph?: "out" | "rank"; onclick: () => void } = $props();

  const art = $derived(kinds[kind]);
</script>

<button class="link-row title group flex items-center gap-3.5 text-left" {title} {onclick}>
  <span
    class="link-tile"
    style:--tint={art?.tint}
    aria-hidden="true"
  >
    {#if art}
      <span class="link-icon" style:--mask={`url(${art.icon})`} style:--scale={art.scale}></span>
    {:else}
      <!-- No art for this kind of link yet: a plain glyph in the icon colour. -->
      <span class="absolute inset-0 flex items-center justify-center text-link-icon"><Icon name={glyph} size={18} /></span>
    {/if}
  </span>
  <span class="text-[14px] tracking-[0.06em] !text-parchment [text-shadow:0_1px_6px_#000] group-hover:!text-[#fff0c4]">{label}</span>
</button>
