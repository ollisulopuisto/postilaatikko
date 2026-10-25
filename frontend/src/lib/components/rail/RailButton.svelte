<script lang="ts">
  import Icon from '@iconify/svelte'

  interface Props {
    icon: string
    label: string
    active: boolean
    onclick: () => void
    /** Optional badge shown in the top-right corner (e.g. a shortcut number). */
    badge?: string
  }

  const { icon, label, active, onclick, badge }: Props = $props()
</script>

<button
  class="flex items-center justify-center relative w-12 h-12 border-l-[3px] transition-colors duration-150 cursor-pointer focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary focus-visible:-outline-offset-2 {active
    ? 'border-l-primary text-primary bg-accent/40'
    : 'border-l-transparent text-muted-foreground hover:text-foreground hover:bg-accent/30'}"
  type="button"
  title={label}
  aria-label={label}
  aria-pressed={active}
  {onclick}
>
  <Icon {icon} width="22" height="22" />
  {#if badge}
    <span class="absolute top-1 right-1 text-[10px] font-bold text-primary">{badge}</span>
  {/if}
</button>
