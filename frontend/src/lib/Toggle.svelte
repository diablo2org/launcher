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
  class="relative h-6 w-11 shrink-0 rounded-full border border-edge transition-colors disabled:opacity-40"
  class:bg-accent={checked}
  class:bg-[#1f1f1f]={!checked}
  onclick={() => onchange(!checked)}
>
  <span
    class="absolute top-[3px] left-[3px] h-4 w-4 rounded-full bg-title shadow transition-transform"
    class:translate-x-5={checked}
  ></span>
</button>
