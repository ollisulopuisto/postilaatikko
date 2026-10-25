<script lang="ts">
  // CommandPalette.svelte — Alfred-style floating quick command launcher.
  // Triggered by Cmd+P / Ctrl+P (or Cmd+Shift+P / Ctrl+Shift+P) or from the UI.
  import { tick } from 'svelte'
  import Icon from '@iconify/svelte'
  import { _ } from '$lib/i18n'
  import {
    commandPalette,
    closeCommandPalette,
    executeCommand,
  } from '$lib/stores/commandPalette.svelte'
  import { formatShortcut, isMacPlatform } from '$lib/keyboard/format'
  import { getActiveExtension, setActiveExtension } from '$lib/stores/uiState.svelte'
  import { getIsDarkActive } from '$lib/stores/theme.svelte'

  const mac = isMacPlatform()

  let query = $state('')
  let selectedIndex = $state(0)
  let inputEl = $state<HTMLInputElement | null>(null)
  let listEl = $state<HTMLDivElement | null>(null)

  interface PaletteItem {
    id: string
    title: string
    category: string
    icon: string
    shortcut?: string
    action: () => unknown
  }

  // Focus input when opened and reset query
  $effect(() => {
    if (commandPalette.open) {
      query = ''
      selectedIndex = 0
      tick().then(() => {
        inputEl?.focus()
      })
    }
  })

  // Static and registry-backed command list
  const allCommands = $derived.by(() => {
    // Read reactivity dependencies
    const isDark = getIsDarkActive()
    const activeExt = getActiveExtension()

    const list: PaletteItem[] = [
      // Primary Mail commands
      {
        id: 'app.compose',
        title: $_('shortcuts.action.compose'),
        category: $_('commandPalette.categories.mail'),
        icon: 'mdi:pencil-plus-outline',
        shortcut: formatShortcut('app.compose', mac),
        action: () => executeCommand('app.compose'),
      },
      {
        id: 'app.search',
        title: $_('shortcuts.action.search'),
        category: $_('commandPalette.categories.mail'),
        icon: 'mdi:magnify',
        shortcut: formatShortcut('app.search', mac),
        action: () => executeCommand('app.search'),
      },
      {
        id: 'app.syncAll',
        title: $_('shortcuts.action.syncAll'),
        category: $_('commandPalette.categories.mail'),
        icon: 'mdi:refresh',
        shortcut: formatShortcut('app.syncAll', mac),
        action: () => executeCommand('app.syncAll'),
      },
      {
        id: 'app.syncFolder',
        title: $_('shortcuts.action.syncFolder'),
        category: $_('commandPalette.categories.mail'),
        icon: 'mdi:folder-sync-outline',
        shortcut: formatShortcut('app.syncFolder', mac),
        action: () => executeCommand('app.syncFolder'),
      },
      // View switches
      {
        id: 'view.mail',
        title: activeExt === 'mail' ? `${$_('commandPalette.categories.mail')} (${$_('common.active') || 'active'})` : $_('commandPalette.categories.mail'),
        category: $_('commandPalette.categories.view'),
        icon: 'mdi:email-outline',
        action: () => {
          setActiveExtension('mail')
          closeCommandPalette()
        },
      },
      {
        id: 'view.calendar',
        title: activeExt === 'calendar' ? `${$_('commandPalette.categories.calendar')} (${$_('common.active') || 'active'})` : $_('commandPalette.categories.calendar'),
        category: $_('commandPalette.categories.view'),
        icon: 'mdi:calendar-outline',
        action: () => {
          setActiveExtension('calendar')
          closeCommandPalette()
        },
      },
      {
        id: 'view.contacts',
        title: activeExt === 'contacts' ? `${$_('commandPalette.categories.contacts')} (${$_('common.active') || 'active'})` : $_('commandPalette.categories.contacts'),
        category: $_('commandPalette.categories.view'),
        icon: 'mdi:account-box-outline',
        action: () => {
          setActiveExtension('contacts')
          closeCommandPalette()
        },
      },
      // Settings & Shortcuts
      {
        id: 'app.preferences',
        title: $_('shortcuts.action.preferences'),
        category: $_('commandPalette.categories.settings'),
        icon: 'mdi:cog-outline',
        shortcut: formatShortcut('app.preferences', mac),
        action: () => executeCommand('app.preferences'),
      },
      {
        id: 'app.shortcuts',
        title: $_('shortcuts.action.showShortcuts'),
        category: $_('commandPalette.categories.general'),
        icon: 'mdi:keyboard-outline',
        shortcut: formatShortcut('app.shortcuts', mac),
        action: () => executeCommand('app.shortcuts'),
      },
      {
        id: 'theme.toggle',
        title: isDark ? 'Switch to light theme' : 'Switch to dark theme',
        category: $_('commandPalette.categories.view'),
        icon: isDark ? 'mdi:weather-sunny' : 'mdi:weather-night',
        action: () => executeCommand('theme.toggle'),
      },
      // Message Actions
      {
        id: 'mail.reply',
        title: $_('shortcuts.action.reply'),
        category: $_('commandPalette.categories.mail'),
        icon: 'mdi:reply',
        shortcut: formatShortcut('mail.reply', mac),
        action: () => executeCommand('mail.reply'),
      },
      {
        id: 'mail.replyAll',
        title: $_('shortcuts.action.replyAll'),
        category: $_('commandPalette.categories.mail'),
        icon: 'mdi:reply-all',
        shortcut: formatShortcut('mail.replyAll', mac),
        action: () => executeCommand('mail.replyAll'),
      },
      {
        id: 'mail.forward',
        title: $_('shortcuts.action.forward'),
        category: $_('commandPalette.categories.mail'),
        icon: 'mdi:forward',
        shortcut: formatShortcut('mail.forward', mac),
        action: () => executeCommand('mail.forward'),
      },
      {
        id: 'mail.archive',
        title: $_('shortcuts.action.archive'),
        category: $_('commandPalette.categories.mail'),
        icon: 'mdi:archive-outline',
        shortcut: formatShortcut('mail.archive', mac),
        action: () => executeCommand('mail.archive'),
      },
      {
        id: 'mail.spam',
        title: $_('shortcuts.action.spam'),
        category: $_('commandPalette.categories.mail'),
        icon: 'mdi:alert-octagon-outline',
        shortcut: formatShortcut('mail.spam', mac),
        action: () => executeCommand('mail.spam'),
      },
      {
        id: 'mail.trash',
        title: $_('shortcuts.action.trash'),
        category: $_('commandPalette.categories.mail'),
        icon: 'mdi:delete-outline',
        shortcut: formatShortcut('mail.trash', mac),
        action: () => executeCommand('mail.trash'),
      },
      {
        id: 'mail.markRead',
        title: $_('shortcuts.action.markRead'),
        category: $_('commandPalette.categories.mail'),
        icon: 'mdi:email-open-outline',
        shortcut: formatShortcut('mail.markRead', mac),
        action: () => executeCommand('mail.markRead'),
      },
      {
        id: 'mail.markUnread',
        title: $_('shortcuts.action.markUnread'),
        category: $_('commandPalette.categories.mail'),
        icon: 'mdi:email-outline',
        shortcut: formatShortcut('mail.markUnread', mac),
        action: () => executeCommand('mail.markUnread'),
      },
      {
        id: 'mail.star',
        title: $_('shortcuts.action.star'),
        category: $_('commandPalette.categories.mail'),
        icon: 'mdi:star-outline',
        shortcut: formatShortcut('mail.star', mac),
        action: () => executeCommand('mail.star'),
      },
      {
        id: 'app.undo',
        title: $_('shortcuts.action.undo'),
        category: $_('commandPalette.categories.general'),
        icon: 'mdi:undo',
        shortcut: formatShortcut('app.undo', mac),
        action: () => executeCommand('app.undo'),
      },
      // Focus navigation
      {
        id: 'focus.messageList',
        title: $_('shortcuts.action.paneNext'),
        category: $_('commandPalette.categories.navigation'),
        icon: 'mdi:view-list-outline',
        action: () => executeCommand('focus.messageList'),
      },
      {
        id: 'focus.viewer',
        title: $_('shortcuts.action.threadFocus'),
        category: $_('commandPalette.categories.navigation'),
        icon: 'mdi:eye-outline',
        action: () => executeCommand('focus.viewer'),
      },
      {
        id: 'focus.sidebar',
        title: $_('shortcuts.action.folderNext'),
        category: $_('commandPalette.categories.navigation'),
        icon: 'mdi:dock-left',
        action: () => executeCommand('focus.sidebar'),
      },
      {
        id: 'app.quit',
        title: $_('shortcuts.action.quit'),
        category: $_('commandPalette.categories.general'),
        icon: 'mdi:power',
        shortcut: formatShortcut('app.quit', mac),
        action: () => executeCommand('app.quit'),
      },
    ]

    return list
  })

  // Filter commands by query
  const filteredCommands = $derived.by(() => {
    const q = query.trim().toLowerCase()
    if (!q) return allCommands
    return allCommands.filter((cmd) => {
      const matchTitle = cmd.title.toLowerCase().includes(q)
      const matchCat = cmd.category.toLowerCase().includes(q)
      const matchShortcut = cmd.shortcut?.toLowerCase().includes(q)
      return matchTitle || matchCat || matchShortcut
    })
  })

  // Reset selected index when query changes
  $effect(() => {
    // Touch query to trigger
    const _q = query
    selectedIndex = 0
  })

  // Ensure selected index is in range
  $effect(() => {
    if (selectedIndex >= filteredCommands.length) {
      selectedIndex = Math.max(0, filteredCommands.length - 1)
    }
  })

  // Keep selected item in view
  $effect(() => {
    const idx = selectedIndex
    if (!listEl) return
    const el = listEl.querySelector(`[data-index="${idx}"]`)
    if (el) {
      el.scrollIntoView({ block: 'nearest' })
    }
  })

  function handleKeyDown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      closeCommandPalette()
      return
    }

    if (e.key === 'ArrowDown') {
      e.preventDefault()
      if (filteredCommands.length > 0) {
        selectedIndex = (selectedIndex + 1) % filteredCommands.length
      }
      return
    }

    if (e.key === 'ArrowUp') {
      e.preventDefault()
      if (filteredCommands.length > 0) {
        selectedIndex = (selectedIndex - 1 + filteredCommands.length) % filteredCommands.length
      }
      return
    }

    if (e.key === 'Enter') {
      e.preventDefault()
      if (filteredCommands.length > 0 && selectedIndex < filteredCommands.length) {
        runCommand(filteredCommands[selectedIndex])
      } else if (query.trim()) {
        // Fallback: trigger search with the query
        executeCommand('app.search')
      }
    }
  }

  function runCommand(cmd: PaletteItem) {
    closeCommandPalette()
    cmd.action()
  }

  function handleBackdropClick(e: MouseEvent) {
    if (e.target === e.currentTarget) {
      closeCommandPalette()
    }
  }
</script>

{#if commandPalette.open}
  <!-- Backdrop -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div
    class="fixed inset-0 z-50 flex items-start justify-center pt-[14vh] bg-black/50 backdrop-blur-sm"
    onclick={handleBackdropClick}
  >
    <!-- Alfred-like floating card -->
    <div
      tabindex="-1"
      class="w-full max-w-xl mx-4 bg-popover text-popover-foreground border border-border shadow-2xl rounded-2xl overflow-hidden flex flex-col h-[min(480px,80vh)] animate-in fade-in-0 zoom-in-95 duration-150"
      onclick={(e) => e.stopPropagation()}
      onkeydown={handleKeyDown}
      role="dialog"
      aria-modal="true"
      aria-label={$_('commandPalette.title')}
    >
      <!-- Search input bar -->
      <div class="flex items-center gap-3 px-4 py-3.5 border-b border-border bg-card shrink-0">
        <Icon icon="mdi:magnify" class="w-5 h-5 text-muted-foreground shrink-0" />
        <input
          bind:this={inputEl}
          bind:value={query}
          type="text"
          class="flex-1 bg-transparent border-none outline-none text-base placeholder:text-muted-foreground/60 text-foreground"
          placeholder={$_('commandPalette.placeholder')}
          aria-label={$_('commandPalette.placeholder')}
          autocomplete="off"
          autocorrect="off"
          spellcheck="false"
        />
        {#if query}
          <button
            type="button"
            class="p-1 rounded-md text-muted-foreground hover:text-foreground transition-colors"
            onclick={() => (query = '')}
            aria-label="Clear query"
          >
            <Icon icon="mdi:close-circle" class="w-4 h-4" />
          </button>
        {/if}
        <kbd class="px-1.5 py-0.5 text-[10px] font-sans font-medium rounded border border-border bg-muted text-muted-foreground select-none">
          ESC
        </kbd>
      </div>

      <!-- Commands list container -->
      <div
        bind:this={listEl}
        class="flex-1 min-h-0 overflow-y-auto p-2 scrollbar-thin"
        role="listbox"
        aria-label={$_('commandPalette.title')}
      >
        {#if filteredCommands.length === 0}
          <div class="py-12 px-4 text-center">
            <Icon icon="mdi:clipboard-search-outline" class="w-8 h-8 text-muted-foreground/50 mx-auto mb-2" />
            <p class="text-sm text-muted-foreground">
              {$_('commandPalette.noResults', { values: { query: query.trim() } })}
            </p>
            {#if query.trim()}
              <button
                type="button"
                class="mt-3 inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg bg-primary/10 text-primary hover:bg-primary/20 transition-colors"
                onclick={() => runCommand({
                  id: 'app.search',
                  title: `${$_('shortcuts.action.search')}: "${query.trim()}"`,
                  category: $_('commandPalette.categories.mail'),
                  icon: 'mdi:magnify',
                  action: () => executeCommand('app.search'),
                })}
              >
                <Icon icon="mdi:magnify" class="w-3.5 h-3.5" />
                <span>Search messages for "{query.trim()}"</span>
              </button>
            {/if}
          </div>
        {:else}
          <div class="space-y-0.5">
            {#each filteredCommands as cmd, i (cmd.id)}
              <!-- svelte-ignore a11y_click_events_have_key_events -->
              <div
                tabindex="-1"
                data-index={i}
                role="option"
                aria-selected={i === selectedIndex}
                class="flex items-center gap-3 px-3 py-2 rounded-xl text-sm cursor-pointer transition-colors select-none {i === selectedIndex
                  ? 'bg-primary/15 text-primary font-medium'
                  : 'hover:bg-muted/60 text-foreground'}"
                onclick={() => runCommand(cmd)}
                onmouseenter={() => (selectedIndex = i)}
              >
                <!-- Icon badge -->
                <div class="flex items-center justify-center w-7 h-7 rounded-lg {i === selectedIndex ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground'} shrink-0 transition-colors">
                  <Icon icon={cmd.icon} class="w-4 h-4" />
                </div>

                <!-- Label and Category -->
                <div class="flex-1 min-w-0">
                  <div class="truncate leading-tight">{cmd.title}</div>
                  <div class="text-[11px] {i === selectedIndex ? 'text-primary/70' : 'text-muted-foreground'} truncate">
                    {cmd.category}
                  </div>
                </div>

                <!-- Shortcut pill -->
                {#if cmd.shortcut}
                  <div class="shrink-0">
                    <kbd class="inline-flex h-5 items-center justify-center rounded border border-border/80 bg-muted/80 px-1.5 font-sans text-[11px] font-medium text-foreground shadow-[inset_0_-1px_0_hsl(var(--border))]">
                      {cmd.shortcut}
                    </kbd>
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      </div>

      <!-- Alfred-like Footer Hints -->
      <footer class="h-9 px-4 border-t border-border bg-muted/40 flex items-center justify-between text-[11px] text-muted-foreground select-none shrink-0">
        <div class="flex items-center gap-3">
          <span class="inline-flex items-center gap-1">
            <kbd class="px-1 py-0.2 rounded border border-border bg-muted font-sans text-[10px]">↑</kbd>
            <kbd class="px-1 py-0.2 rounded border border-border bg-muted font-sans text-[10px]">↓</kbd>
            <span>{$_('commandPalette.hints.navigate')}</span>
          </span>
          <span class="inline-flex items-center gap-1">
            <kbd class="px-1 py-0.2 rounded border border-border bg-muted font-sans text-[10px]">↵</kbd>
            <span>{$_('commandPalette.hints.select')}</span>
          </span>
          <span class="inline-flex items-center gap-1">
            <kbd class="px-1 py-0.2 rounded border border-border bg-muted font-sans text-[10px]">esc</kbd>
            <span>{$_('commandPalette.hints.close')}</span>
          </span>
        </div>
        <div class="hidden sm:block font-medium text-[10px] uppercase tracking-wider text-muted-foreground/70">
          Postilaatikko
        </div>
      </footer>
    </div>
  </div>
{/if}
