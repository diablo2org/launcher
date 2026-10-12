<script lang="ts">
  let {
    checked,
    disabled = false,
    label,
    onchange,
  }: { checked: boolean; disabled?: boolean; label: string; onchange: (on: boolean) => void } = $props();
</script>

<button
  role="switch"
  aria-checked={checked}
  aria-label={label}
  {disabled}
  class="relative h-[22px] w-11 shrink-0 rounded-full border transition-colors disabled:opacity-40"
  class:border-gold-line={checked}
  class:bg-[#1d1407]={checked}
  class:border-edge={!checked}
  class:bg-[#0a0908]={!checked}
  onclick={() => onchange(!checked)}
>
  <span
    class="absolute top-[2px] left-[2px] h-4 w-4 rounded-full transition-transform"
    class:translate-x-[22px]={checked}
    class:knob-on={checked}
    class:bg-[#4f4535]={!checked}
  ></span>
</button>

<style>
  .knob-on {
    background: radial-gradient(circle at 35% 35%, #f6dfa0, #a07a3c);
    box-shadow: 0 0 6px rgba(240, 190, 90, 0.6);
  }
</style>
