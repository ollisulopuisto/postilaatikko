<script lang="ts">
  import Icon from '@iconify/svelte'
  import { Button } from '$lib/components/ui/button'
  import { _ } from '$lib/i18n'
  import { navigationCards as nav } from '$lib/stores/navigationCards.svelte'
  import type { NavigationCard } from '$lib/sidebar/cards'

  const moveButtonClass = 'flex h-7 w-7 shrink-0 items-center justify-center rounded hover:bg-muted disabled:opacity-40'

  // Editable scratch name for adding a new card
  let newName = $state('')

  function addCard() {
    if (!newName.trim()) return
    nav.update({
      ...nav.state,
      cards: [...nav.state.cards, { id: crypto.randomUUID(), name: newName.trim(), hidden: false, collapsed: false, entries: [] }],
    })
    newName = ''
  }

  function cardName(card: NavigationCard): string {
    const names: Record<string, [string, string]> = {
      virtual: ['Virtual folders', 'navigation.virtualCard'],
      primary: ['Mail', 'navigation.mailCard'],
      secondary: ['More', 'navigation.moreCard'],
      pinned: ['Pinned folders', 'navigation.pinnedCard'],
      favorites: ['Starred folders', 'navigation.favoritesCard'],
    }
    return names[card.id]?.[0] === card.name ? $_(names[card.id][1]) : card.name
  }
</script>

<div class="space-y-4">
  <div>
    <p class="text-sm font-medium">{$_('navigation.cardsTitle')}</p>
    <p class="text-xs text-muted-foreground">{$_('navigation.cardsHelp')}</p>
  </div>

  <ul class="space-y-1">
    {#each nav.state.cards as card, index (card.id)}
      {@const label = cardName(card)}
      <li class="flex items-center gap-2 rounded-md border border-border px-2 py-1.5">
        <input
          id={`sidebar-card-${card.id}`}
          type="checkbox"
          class="h-4 w-4 accent-primary"
          checked={!card.hidden}
          onchange={(event) => nav.patchCard(card.id, { hidden: !event.currentTarget.checked })}
          aria-label={label}
        />
        <label for={`sidebar-card-${card.id}`} class="min-w-0 flex-1 truncate text-sm">{label}</label>
        <span class="ml-auto text-xs text-muted-foreground">{card.entries.length} entry{card.entries.length === 1 ? '' : 's'}</span>
        <button
          type="button"
          class={moveButtonClass}
          disabled={index === 0 || card.hidden}
          onclick={() => nav.moveCard(card.id, -1)}
          aria-label={`${$_('sidebarLayout.moveUp')}: ${label}`}
          title={$_('sidebarLayout.moveUp')}
        >
          <Icon icon="mdi:chevron-up" class="h-4 w-4" />
        </button>
        <button
          type="button"
          class={moveButtonClass}
          disabled={index === nav.state.cards.length - 1 || card.hidden}
          onclick={() => nav.moveCard(card.id, 1)}
          aria-label={`${$_('sidebarLayout.moveDown')}: ${label}`}
          title={$_('sidebarLayout.moveDown')}
        >
          <Icon icon="mdi:chevron-down" class="h-4 w-4" />
        </button>
        <button
          type="button"
          class={moveButtonClass}
          aria-expanded={!card.collapsed}
          onclick={() => nav.patchCard(card.id, { collapsed: !card.collapsed })}
          title={card.collapsed ? $_('sidebar.expand') : $_('sidebar.collapse')}
        >
          <Icon icon={card.collapsed ? 'mdi:chevron-right' : 'mdi:chevron-down'} class="h-4 w-4" />
        </button>
      </li>
    {/each}
  </ul>

  <form class="flex gap-1 items-end" onsubmit={(event) => { event.preventDefault(); addCard() }}>
    <input
      class="w-full min-w-0 rounded border border-border bg-background px-2 text-sm"
      bind:value={newName}
      placeholder={$_('navigation.groupName')}
      aria-label={$_('navigation.groupName')}
      maxlength="100"
    />
    <button type="submit" class="p-2" aria-label={$_('navigation.addGroup')} disabled={!newName.trim()}>
      <Icon icon="mdi:plus" />
    </button>
  </form>

  <Button variant="outline" size="sm" onclick={() => nav.reset()}>
    {$_('navigation.resetCards')}
  </Button>
</div>
