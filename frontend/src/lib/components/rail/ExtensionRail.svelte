<script lang="ts">
  import Icon from '@iconify/svelte'
  import RailButton from './RailButton.svelte'
  import { getRailTabs, isRailVisible } from '$lib/stores/extensionRegistry.svelte'
  import { getActiveExtension, getShowContactsCalendarRail, setActiveExtension } from '$lib/stores/uiState.svelte'
  import { accountStore } from '$lib/stores/accounts.svelte'
  import { sidebarPins } from '$lib/stores/sidebarPins.svelte'
  import { isInputElement } from '$lib/stores/keyboard.svelte'
  import type { PinnedFolder } from '$lib/sidebar/pins'
  // @ts-ignore - Wails model types
  import type { folder } from '../../../../wailsjs/go/models'

  interface Props {
    onFolderSelect?: (accountId: string, folderId: string, folderPath: string, folderName: string, folderType: string) => void
  }

  let { onFolderSelect }: Props = $props()

  // Mail is always present and always first; extensions follow in their
  // registered Order. Rail renders only when at least one extension is enabled
  // (so there's something to switch between).
  let active = $derived(getActiveExtension())
  let pins = $derived(sidebarPins.list.slice(0, 10))
  let extensionItems = $derived(getRailTabs().filter(tab =>
    getShowContactsCalendarRail() || !['contacts', 'calendar'].includes(tab.extensionId.toLowerCase())
  ))
  let visible = $derived(isRailVisible() && extensionItems.length > 0 || pins.length > 0)
  let tabs = $derived(extensionItems)

  function select(name: string) {
    setActiveExtension(name)
  }

  function findFolder(trees: folder.FolderTree[], id: string): folder.Folder | null {
    for (const tree of trees || []) {
      if (tree.folder?.id === id) return tree.folder
      const nested = findFolder(tree.children || [], id)
      if (nested) return nested
    }
    return null
  }

  function selectPin(pin: PinnedFolder): void {
    const account = accountStore.accounts.find(item => item.account?.id === pin.accountId)
    if (!account) return
    const folder = findFolder(account.folders || [], pin.folderId)
    setActiveExtension('mail')
    onFolderSelect?.(pin.accountId, pin.folderId, pin.path, pin.name, folder?.type || 'folder')
  }

  function shortcutFor(index: number): string {
    const key = index === 9 ? '0' : String(index + 1)
    const mac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform)
    return `${mac ? '⌘' : 'Ctrl+'}${key}`
  }

  function handleQuickFolderShortcut(event: KeyboardEvent): void {
    if (event.altKey || event.shiftKey || isInputElement(event.target)) return
    const mac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform)
    if (mac ? !event.metaKey || event.ctrlKey : !event.ctrlKey || event.metaKey) return
    if (!/^[0-9]$/.test(event.key)) return
    const index = event.key === '0' ? 9 : Number(event.key) - 1
    const pin = pins[index]
    if (!pin) return
    event.preventDefault()
    selectPin(pin)
  }
</script>

{#if visible}
  <nav
    class="flex flex-col items-stretch w-12 flex-shrink-0 bg-muted/30 border-r border-border pt-2 overflow-y-auto"
    aria-label="Active extension"
  >
    <RailButton
      icon="mdi:email"
      label="Mail"
      active={active === 'mail'}
      onclick={() => select('mail')}
    />
    {#each pins as pin, index (`${pin.accountId}:${pin.folderId}`)}
      <button
        type="button"
        class="relative flex h-12 w-12 shrink-0 items-center justify-center border-l-[3px] border-l-transparent text-muted-foreground transition-colors hover:bg-accent/30 hover:text-foreground focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary"
        title={`${pin.name} · ${shortcutFor(index)}`}
        aria-label={`${pin.name}, ${shortcutFor(index)}`}
        aria-keyshortcuts={`${/Mac|iPhone|iPad/.test(typeof navigator === 'undefined' ? '' : navigator.platform) ? 'Meta' : 'Control'}+${index === 9 ? '0' : index + 1}`}
        onclick={() => selectPin(pin)}
      >
        <Icon icon="mdi:folder-star-outline" width="22" height="22" />
        <span class="absolute bottom-0.5 right-1 text-[9px] leading-none text-muted-foreground" aria-hidden="true">{index === 9 ? '0' : index + 1}</span>
      </button>
    {/each}
    {#each tabs as tab (tab.extensionId)}
      <RailButton
        icon={tab.icon || 'mdi:puzzle'}
        label={tab.label}
        active={active === tab.extensionId}
        onclick={() => select(tab.extensionId)}
      />
    {/each}
  </nav>
{/if}

<svelte:window onkeydown={handleQuickFolderShortcut} />
