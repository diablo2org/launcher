<script lang="ts">
  // A dropdown drawn in the launcher's own style. The native <select> opens a
  // list the system draws (white and blue on Windows) that can't be styled.
  // Keyboard: Enter, Space or Down opens; Up/Down/Home/End move; Enter or
  // Space picks; Escape or Tab closes; typing a letter jumps to it.
  let {
    value,
    options,
    label,
    disabled = false,
    onchange,
  }: {
    value: string;
    options: { value: string; label: string }[];
    label: string;
    disabled?: boolean;
    onchange: (value: string) => void;
  } = $props();

  const id = `select-${Math.random().toString(36).slice(2, 8)}`;
  let open = $state(false);
  let active = $state(0);
  let root: HTMLDivElement;
  let button: HTMLButtonElement;

  // The highlighted option stays in view as the keys move it.
  $effect(() => {
    if (open) document.getElementById(`${id}-${active}`)?.scrollIntoView({ block: "nearest" });
  });

  const current = $derived(options.find((o) => o.value === value));

  function show() {
    if (disabled || !options.length) return;
    active = Math.max(0, options.findIndex((o) => o.value === value));
    open = true;
  }

  function pick(i: number) {
    const o = options[i];
    open = false;
    button.focus();
    if (o && o.value !== value) onchange(o.value);
  }

  function onkeydown(e: KeyboardEvent) {
    if (!open) {
      if (["Enter", " ", "ArrowDown", "ArrowUp"].includes(e.key)) {
        e.preventDefault();
        show();
      }
      return;
    }
    if (e.key === "ArrowDown") active = Math.min(options.length - 1, active + 1);
    else if (e.key === "ArrowUp") active = Math.max(0, active - 1);
    else if (e.key === "Home") active = 0;
    else if (e.key === "End") active = options.length - 1;
    else if (e.key === "Enter" || e.key === " ") pick(active);
    else if (e.key === "Escape") {
      open = false;
    } else if (e.key === "Tab") {
      open = false;
      return;
    } else if (e.key.length === 1) {
      const k = e.key.toLowerCase();
      const n = options.length;
      for (let step = 1; step <= n; step++) {
        const i = (active + step) % n;
        if (options[i].label.toLowerCase().startsWith(k)) {
          active = i;
          break;
        }
      }
    } else return;
    e.preventDefault();
  }

  // A click anywhere else closes the list.
  function onWindowPointer(e: PointerEvent) {
    if (open && !root.contains(e.target as Node)) open = false;
  }
</script>

<svelte:window onpointerdown={onWindowPointer} />

<div class="relative" bind:this={root}>
  <button
    bind:this={button}
    type="button"
    role="combobox"
    class="btn-stone select relative flex h-full w-full items-center pr-9 pl-3 text-left"
    aria-haspopup="listbox"
    aria-expanded={open}
    aria-controls={id}
    aria-label={label}
    aria-activedescendant={open ? `${id}-${active}` : undefined}
    {disabled}
    onclick={() => (open ? (open = false) : show())}
    {onkeydown}
  >
    <span class="truncate">{current?.label ?? ""}</span>
    <svg class="absolute top-1/2 right-3 -translate-y-1/2 text-[#d9c49a] transition-transform" class:rotate-180={open} width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
      <path d="M2.5 4.5L6 8l3.5-3.5" />
    </svg>
  </button>

  {#if open}
    <ul {id} role="listbox" aria-label={label} class="select-list absolute top-full right-0 left-0 z-50 mt-1 max-h-60 overflow-y-auto py-1">
      {#each options as o, i (o.value)}
        <!-- The button owns the keyboard (aria-activedescendant); options only take the pointer. -->
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <li
          id={`${id}-${i}`}
          role="option"
          aria-selected={o.value === value}
          class="select-option"
          class:active={i === active}
          onpointerenter={() => (active = i)}
          onclick={() => pick(i)}
        >
          {o.label}
        </li>
      {/each}
    </ul>
  {/if}
</div>
