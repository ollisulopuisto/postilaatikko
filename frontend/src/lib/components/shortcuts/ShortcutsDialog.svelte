<script lang="ts">
  // Keyboard shortcuts overlay. Opened with `?` (App.svelte) or from the
  // "Keyboard shortcuts" entries in the mail sidebar and extension sidebar
  // footers. Everything shown comes from $lib/keyboard/registry, so this
  // component holds no shortcut data of its own.
  //
  // By default it lists the shortcuts that apply to the active rail pane
  // (Mail, Calendar, Contacts); "Show all views" or a search query widens
  // that to every pane.
  import * as Dialog from '$lib/components/ui/dialog'
  import { Input } from '$lib/components/ui/input'
  import Icon from '@iconify/svelte'
  import { _ } from '$lib/i18n'
  import { dialogGuardOpen, dialogGuardClose } from '$lib/stores/dialogGuard'
  import { shortcutsOverlay } from '$lib/stores/shortcutsOverlay.svelte'
  import { getActiveExtension } from '$lib/stores/uiState.svelte'
  import { SHORTCUTS, SHORTCUT_GROUPS, appliesToPane } from '$lib/keyboard/registry'
  import { comboCaps, formatCombo, formatShortcut, isAvailable, isMacPlatform, spokenCombo } from '$lib/keyboard/format'

  const mac = isMacPlatform()

  let query = $state('')
  let showAll = $state(false)

  const pane = $derived(getActiveExtension())

  // Keep App.svelte's global key handler out of the way while the overlay
  // is open, like the other modal dialogs do.
  let guarded = false
  $effect(() => {
    const open = shortcutsOverlay.open
    if (open && !guarded) {
      dialogGuardOpen()
      guarded = true
      query = ''
    }
    if (!open && guarded) {
      dialogGuardClose()
      guarded = false
    }
  })

  $effect(() => () => {
    if (guarded) dialogGuardClose()
  })

  interface Row {
    id: string
    label: string
    combos: { caps: string[]; spoken: string }[]
  }

  const sections = $derived.by(() => {
    const q = query.trim().toLowerCase()
    const result: { id: string; label: string; rows: Row[] }[] = []
    for (const group of SHORTCUT_GROUPS) {
      const groupLabel = $_(group.label)
      const rows: Row[] = []
      for (const def of SHORTCUTS) {
        if (def.group !== group.id || !isAvailable(def, mac)) continue
        if (!q && !showAll && !appliesToPane(def, pane)) continue
        const label = $_(def.label)
        if (q) {
          const haystack = [
            label,
            groupLabel,
            ...def.combos.map(c => formatCombo(c, mac)),
            ...def.combos.map(c => formatCombo(c, false)),
          ].join(' ').toLowerCase()
          if (!haystack.includes(q)) continue
        }
        rows.push({
          id: def.id,
          label,
          combos: def.combos.map(c => ({ caps: comboCaps(c, mac), spoken: spokenCombo(c, mac) })),
        })
      }
      if (rows.length > 0) result.push({ id: group.id, label: groupLabel, rows })
    }
    return result
  })
</script>

<Dialog.Root open={shortcutsOverlay.open} onOpenChange={(value) => (shortcutsOverlay.open = value)}>
  <Dialog.Content class="max-w-3xl h-[min(640px,85vh)] overflow-hidden flex flex-col" preventCloseAutoFocus>
    <Dialog.Header class="pr-6 shrink-0">
      <Dialog.Title>{$_('shortcuts.title')}</Dialog.Title>
      <Dialog.Description>{$_('shortcuts.description')}</Dialog.Description>
    </Dialog.Header>

    <div class="flex flex-col gap-3 sm:flex-row sm:items-center shrink-0">
      <div class="relative flex-1">
        <Icon icon="mdi:magnify" class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          type="search"
          class="pl-9"
          bind:value={query}
          placeholder={$_('shortcuts.search')}
          aria-label={$_('shortcuts.search')}
          aria-controls="shortcuts-dialog-list"
          autofocus
        />
      </div>
      <label class="flex shrink-0 cursor-pointer select-none items-center gap-2 text-sm text-muted-foreground">
        <input type="checkbox" class="h-4 w-4 rounded border-input accent-primary" bind:checked={showAll} />
        {$_('shortcuts.showAll')}
      </label>
    </div>

    <div id="shortcuts-dialog-list" class="-mx-6 flex-1 min-h-0 overflow-y-auto px-6 scrollbar-thin" aria-live="polite">
      {#if sections.length === 0}
        <p class="py-8 text-center text-sm text-muted-foreground">
          {$_('shortcuts.noResults', { values: { query: query.trim() } })}
        </p>
      {:else}
        <div class="gap-x-8 sm:columns-2">
          {#each sections as section (section.id)}
            <section class="mb-5 break-inside-avoid" aria-labelledby="shortcuts-group-{section.id}">
              <h3 id="shortcuts-group-{section.id}" class="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                {section.label}
              </h3>
              <ul class="divide-y divide-border/60">
                {#each section.rows as row (row.id)}
                  <li class="flex items-start justify-between gap-4 py-1.5 text-sm">
                    <span class="min-w-0 text-foreground">{row.label}</span>
                    <span class="flex shrink-0 flex-wrap items-center justify-end gap-1">
                      {#each row.combos as combo, i (i)}
                        {#if i > 0}
                          <span class="px-0.5 text-xs text-muted-foreground">{$_('shortcuts.or')}</span>
                        {/if}
                        <span class="sr-only">{combo.spoken}</span>
                        <span class="inline-flex items-center gap-0.5" aria-hidden="true">
                          {#each combo.caps as cap, j (j)}
                            <kbd class="inline-flex h-6 min-w-6 items-center justify-center rounded border border-border bg-muted px-1.5 font-sans text-xs font-medium leading-none text-foreground shadow-[inset_0_-1px_0_hsl(var(--border))]">{cap}</kbd>
                          {/each}
                        </span>
                      {/each}
                    </span>
                  </li>
                {/each}
              </ul>
            </section>
          {/each}
        </div>
      {/if}
    </div>

    <p class="text-xs text-muted-foreground shrink-0 mt-3">
      {$_('shortcuts.footer', { values: { key: formatShortcut('app.shortcuts', mac) } })}
    </p>
  </Dialog.Content>
</Dialog.Root>
