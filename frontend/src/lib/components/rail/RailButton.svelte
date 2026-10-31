<script lang="ts">
  import Icon from '@iconify/svelte'

  interface Props {
    icon: string | null
    initial?: string
    color?: string
    label: string
    active: boolean
    onclick: () => void
    quickId?: string
    draggable?: boolean
    ondragstart?: (event: DragEvent) => void
    ondragover?: (event: DragEvent) => void
    ondrop?: (event: DragEvent) => void
    ondragend?: (event: DragEvent) => void
  }

  const { icon, initial, color, label, active, onclick, quickId, draggable = false, ondragstart, ondragover, ondrop, ondragend }: Props = $props()
</script>

<button
  class="flex items-center justify-center w-12 h-12 border-l-[3px] transition-colors duration-150 {draggable ? 'cursor-grab active:cursor-grabbing' : 'cursor-pointer'} focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary focus-visible:-outline-offset-2 {active
    ? 'border-l-primary text-primary bg-accent/40'
    : 'border-l-transparent text-muted-foreground hover:text-foreground hover:bg-accent/30'}"
  type="button"
  data-quick-id={quickId}
  {draggable}
  title={label}
  aria-label={label}
  aria-pressed={active}
  {onclick}
  {ondragstart}
  {ondragover}
  {ondrop}
  {ondragend}
>
  {#if initial}
    <span class="flex h-7 w-7 items-center justify-center rounded-full border border-white/15 text-[10px] font-semibold leading-none text-white shadow-sm" style:background-color={color || 'hsl(var(--primary))'}>{initial}</span>
  {:else if icon}
    <Icon {icon} width="22" height="22" />
  {/if}
</button>
