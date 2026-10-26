<script lang="ts">
  import Icon from '@iconify/svelte'
  import { Button } from '$lib/components/ui/button'
  import { _ } from '$lib/i18n'
  import { sidebarLayout } from '$lib/stores/sidebarLayout.svelte'
  import type { SidebarBlock } from '$lib/sidebar/layout'

  // Translation key for each sidebar block. Reuses the sidebar's own labels
  // where it has them, so the settings list reads the same as the sidebar.
  const blockLabels: Record<SidebarBlock, string> = {
    unified: 'sidebarLayout.blocks.unified',
    pinned: 'sidebar.pinned',
    virtual: 'sidebarLayout.blocks.virtual',
    calendar: 'sidebar.calendar',
    primary: 'sidebarLayout.blocks.primary',
    secondary: 'sidebar.folders',
  }

  const moveButtonClass = 'flex h-7 w-7 shrink-0 items-center justify-center rounded hover:bg-muted disabled:opacity-40'
</script>

<div class="space-y-2">
  <p class="text-sm font-medium">{$_('sidebarLayout.title')}</p>
  <p class="text-xs text-muted-foreground">{$_('sidebarLayout.help')}</p>

  <ul class="space-y-1">
    {#each sidebarLayout.order as block, index (block)}
      {@const label = $_(blockLabels[block])}
      <li class="flex items-center gap-2 rounded-md border border-border px-2 py-1.5">
        <input
          id={`sidebar-block-${block}`}
          type="checkbox"
          class="h-4 w-4 accent-primary"
          checked={sidebarLayout.isVisible(block)}
          onchange={(event) => sidebarLayout.setVisible(block, event.currentTarget.checked)}
        />
        <label for={`sidebar-block-${block}`} class="min-w-0 flex-1 truncate text-sm">{label}</label>
        <button
          type="button"
          class={moveButtonClass}
          disabled={index === 0}
          onclick={() => sidebarLayout.move(block, -1)}
          aria-label={`${$_('sidebarLayout.moveUp')}: ${label}`}
          title={$_('sidebarLayout.moveUp')}
        >
          <Icon icon="mdi:chevron-up" class="h-4 w-4" />
        </button>
        <button
          type="button"
          class={moveButtonClass}
          disabled={index === sidebarLayout.order.length - 1}
          onclick={() => sidebarLayout.move(block, 1)}
          aria-label={`${$_('sidebarLayout.moveDown')}: ${label}`}
          title={$_('sidebarLayout.moveDown')}
        >
          <Icon icon="mdi:chevron-down" class="h-4 w-4" />
        </button>
      </li>
    {/each}
  </ul>

  <Button variant="outline" size="sm" onclick={() => sidebarLayout.reset()}>
    {$_('sidebarLayout.reset')}
  </Button>
</div>
