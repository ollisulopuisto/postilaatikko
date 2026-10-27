<script lang="ts">
  import Icon from '@iconify/svelte'
  import { navigationCards as nav } from '$lib/stores/navigationCards.svelte'
  import { _ } from '$lib/i18n'
  import type { NavigationCard, NavigationEntry } from '$lib/sidebar/cards'
  let { selectedFolderId = null, onSelect, onEdit, onDelete }: { selectedFolderId?: string | null; onSelect: (entry: NavigationEntry) => void; onEdit?: (entry: NavigationEntry) => void; onDelete?: (entry: NavigationEntry) => void } = $props()
  function cardName(card: NavigationCard): string {
    const names: Record<string, [string, string]> = { virtual: ['Virtual folders', 'navigation.virtualCard'], primary: ['Mail', 'navigation.mailCard'], secondary: ['More', 'navigation.moreCard'], pinned: ['Pinned folders', 'navigation.pinnedCard'] }
    return names[card.id]?.[0] === card.name ? $_(names[card.id][1]) : card.name
  }
  let editing = $state(false)
  let newName = $state('')
  function drop(event: DragEvent, card: string, index: number) {
    event.preventDefault(); event.stopPropagation()
    const entry = event.dataTransfer?.getData('application/x-postilaatikko-entry')
    if (entry) { nav.moveEntry(entry, card, index); return }
    const source = event.dataTransfer?.getData('application/x-postilaatikko-card')
    const from = nav.state.cards.findIndex(c => c.id === source), to = nav.state.cards.findIndex(c => c.id === card)
    if (source && from >= 0 && to >= 0) nav.moveCard(source, to - from)
  }
  function addCard() {
    if (!newName.trim()) return
    nav.update({ ...nav.state, cards: [...nav.state.cards, { id: crypto.randomUUID(), name: newName.trim(), hidden: false, collapsed: false, entries: [] }] })
    newName = ''
  }
</script>
<div class="flex items-center justify-end px-2 py-1">
  <button type="button" class="text-xs text-muted-foreground hover:text-foreground" aria-pressed={editing} onclick={() => editing = !editing}>{$_('navigation.organize')}</button>
</div>
{#each nav.state.cards as card, cardIndex (card.id)}
  {#if editing || (!card.hidden && card.entries.length > 0)}
    <section class="mx-2 mb-3 rounded-xl border border-border bg-muted/20 overflow-hidden {card.hidden ? 'opacity-50' : ''}"
      aria-label={cardName(card)} data-navigation-card={card.id}>
      <div class="flex items-center gap-1 px-2 py-2" draggable={editing}
        role="group" aria-label={cardName(card)}
        ondragstart={(event) => { event.dataTransfer?.setData('application/x-postilaatikko-card', card.id) }}
        ondragover={(event) => event.preventDefault()} ondrop={(event) => drop(event, card.id, card.entries.length)}>
        <button type="button" class="flex flex-1 items-center gap-1 text-sm font-medium text-left min-w-0" aria-expanded={!card.collapsed} onclick={() => nav.patchCard(card.id, { collapsed: !card.collapsed })}>
          <Icon icon={card.collapsed ? 'mdi:chevron-right' : 'mdi:chevron-down'} class="h-4 w-4 shrink-0" /><span class="truncate">{cardName(card)}</span>
        </button>
        {#if editing}
          <button type="button" class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded hover:bg-muted disabled:opacity-30" disabled={cardIndex === 0} aria-label={$_('navigation.up')} onclick={() => nav.moveCard(card.id, -1)}><Icon icon="mdi:arrow-up" /></button>
          <button type="button" class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded hover:bg-muted disabled:opacity-30" disabled={cardIndex === nav.state.cards.length - 1} aria-label={$_('navigation.down')} onclick={() => nav.moveCard(card.id, 1)}><Icon icon="mdi:arrow-down" /></button>
          <button type="button" class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded hover:bg-muted" aria-label={$_(card.hidden ? 'navigation.show' : 'navigation.hide')} onclick={() => nav.patchCard(card.id, { hidden: !card.hidden })}><Icon icon={card.hidden ? 'mdi:eye-outline' : 'mdi:eye-off-outline'} /></button>
        {/if}
      </div>
      {#if !card.collapsed || editing}
        <div class="min-h-3 pb-1" role="group" aria-label={cardName(card)} ondragover={(event) => event.preventDefault()} ondrop={(event) => drop(event, card.id, card.entries.length)}>
          {#each card.entries as id, index (id)}
            {@const entry = nav.entries.find(e => e.id === id)}
            {#if entry}
              <div class="group flex items-center gap-1 px-2 py-0.5" role="group" aria-label={entry.name} draggable={editing}
                ondragstart={(event) => { event.stopPropagation(); event.dataTransfer?.setData('application/x-postilaatikko-entry', id) }}
                ondragover={(event) => event.preventDefault()} ondrop={(event) => drop(event, card.id, index)}>
                <button type="button" class="sidebar-primary-link min-w-0 flex-1 {selectedFolderId === entry.folderId || (entry.kind === 'system' && selectedFolderId === entry.folderType) || (entry.kind === 'inboxes' && selectedFolderId === 'inbox') ? 'sidebar-primary-link-selected' : ''}" onclick={() => onSelect(entry)} title={entry.path || entry.name}>
                  <Icon icon={entry.kind === 'virtual' ? 'mdi:folder-multiple-outline' : entry.kind === 'inboxes' ? 'mdi:inbox-multiple-outline' : 'mdi:folder-outline'} class="h-5 w-5 shrink-0" />
                  <span class="truncate">{entry.name}</span>
                </button>
                {#if entry.kind === 'virtual'}
                  <button type="button" class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded opacity-0 group-hover:opacity-100 focus-visible:opacity-100 hover:bg-muted" aria-label={`Edit ${entry.name}`} onclick={() => onEdit?.(entry)}><Icon icon="mdi:pencil-outline" /></button>
                  <button type="button" class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded opacity-0 group-hover:opacity-100 focus-visible:opacity-100 hover:bg-muted" aria-label={`Delete ${entry.name}`} onclick={() => onDelete?.(entry)}><Icon icon="mdi:delete-outline" /></button>
                {/if}
                {#if editing}
                  <button type="button" class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded hover:bg-muted" aria-label={$_('navigation.quickAccess')} aria-pressed={nav.state.quick.includes(id)} disabled={!nav.state.quick.includes(id) && nav.state.quick.length >= 10} onclick={() => nav.toggleQuick(id)}><Icon icon={nav.state.quick.includes(id) ? 'mdi:star' : 'mdi:star-outline'} /></button>
                  <select class="w-20 min-w-0 rounded border border-border bg-background text-xs" aria-label={$_('navigation.moveGroup')} value={card.id} onchange={(event) => nav.moveEntry(id, event.currentTarget.value, 999)}>
                    {#each nav.state.cards as target (target.id)}<option value={target.id}>{cardName(target)}</option>{/each}
                  </select>
                  <button type="button" class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded hover:bg-muted disabled:opacity-30" disabled={index === 0} aria-label={$_('navigation.up')} onclick={() => nav.moveEntry(id, card.id, index - 1)}><Icon icon="mdi:arrow-up" /></button>
                  <button type="button" class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded hover:bg-muted disabled:opacity-30" disabled={index === card.entries.length - 1} aria-label={$_('navigation.down')} onclick={() => nav.moveEntry(id, card.id, index + 1)}><Icon icon="mdi:arrow-down" /></button>
                {/if}
              </div>
            {/if}
          {/each}
        </div>
      {/if}
    </section>
  {/if}
{/each}
{#if editing}
  <form class="mx-2 mb-2 flex gap-1" onsubmit={(event) => { event.preventDefault(); addCard() }}>
    <input class="w-full min-w-0 rounded border border-border bg-background px-2 text-sm" bind:value={newName} placeholder={$_('navigation.groupName')} aria-label={$_('navigation.groupName')} maxlength="100" />
    <button type="submit" class="p-2" aria-label={$_('navigation.addGroup')}><Icon icon="mdi:plus" /></button>
  </form>
  <label class="mx-3 text-xs flex gap-2"><input type="checkbox" checked={nav.state.showApps} onchange={(event) => nav.update({ ...nav.state, showApps: event.currentTarget.checked })} />{$_('navigation.showApps')}</label>
{/if}
