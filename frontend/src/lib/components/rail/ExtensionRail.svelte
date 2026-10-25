<script lang="ts">
  import RailButton from './RailButton.svelte'
  import { getRailTabs, isRailVisible } from '$lib/stores/extensionRegistry.svelte'
  import { getActiveExtension, setActiveExtension } from '$lib/stores/uiState.svelte'
  import { getShowExtensionsOnRail } from '$lib/stores/settings.svelte'
  import { accountStore } from '$lib/stores/accounts.svelte'

  // Default folder shortcuts shown on the rail when extensions are hidden.
  // Each entry: { key: '1'..'0', label, icon, accountId, folderId, folderPath, folderName, folderType }
  // The first account's special folders are used as targets.
  interface RailFolderShortcut {
    key: string
    label: string
    icon: string
    accountId: string | null
    folderId: string | null
    folderPath: string | null
    folderName: string
    folderType: string | null
  }

  let active = $derived(getActiveExtension())
  let visible = $derived(isRailVisible())
  let tabs = $derived(getRailTabs())
  let showExtensions = $derived(getShowExtensionsOnRail())

  // Build folder shortcut targets from the first account's folder tree.
  let folderShortcuts = $derived(buildFolderShortcuts())

  function buildFolderShortcuts(): RailFolderShortcut[] {
    const acc = accountStore.accounts[0]
    if (!acc?.account) return []

    const findFolderByType = (trees: typeof acc.folders, type: string): { id: string; path: string; name: string } | null => {
      for (const tree of trees) {
        if (tree.folder?.type === type) return { id: tree.folder.id, path: tree.folder.path, name: tree.folder.name }
        if (tree.children) {
          const found = findFolderByType(tree.children, type)
          if (found) return found
        }
      }
      return null
    }

    const inbox = findFolderByType(acc.folders || [], 'inbox')
    const starred = findFolderByType(acc.folders || [], 'starred')
    const drafts = findFolderByType(acc.folders || [], 'drafts')
    const sent = findFolderByType(acc.folders || [], 'sent')
    const trash = findFolderByType(acc.folders || [], 'trash')

    const shortcuts: RailFolderShortcut[] = []
    const add = (key: string, label: string, icon: string, target: { id: string; path: string; name: string } | null, type: string | null) => {
      if (target) {
        shortcuts.push({ key, label, icon, accountId: acc.account.id, folderId: target.id, folderPath: target.path, folderName: target.name, folderType: type })
      }
    }

    add('1', 'Inbox', 'mdi:inbox', inbox, 'inbox')
    add('2', 'Starred', 'mdi:star-outline', starred, 'starred')
    add('3', 'Drafts', 'mdi:file-outline', drafts, 'drafts')
    add('4', 'Sent', 'mdi:send-outline', sent, 'sent')
    add('5', 'Trash', 'mdi:delete-outline', trash, 'trash')
    return shortcuts
  }

  function select(name: string) {
    setActiveExtension(name)
  }

  interface Props {
    onFolderSelect?: (accountId: string, folderId: string, folderPath: string, folderName: string, folderType: string) => void
  }

  let { onFolderSelect }: Props = $props()
</script>

{#if visible}
  <nav
    class="flex flex-col items-stretch w-12 flex-shrink-0 bg-muted/30 border-r border-border pt-2"
    aria-label="Active extension"
  >
    <RailButton
      icon="mdi:email"
      label="Mail"
      active={active === 'mail'}
      onclick={() => select('mail')}
    />
    {#if showExtensions}
      {#each tabs as tab (tab.extensionId)}
        <RailButton
          icon={tab.icon || 'mdi:puzzle'}
          label={tab.label}
          active={active === tab.extensionId}
          onclick={() => select(tab.extensionId)}
        />
      {/each}
    {:else}
      {#each folderShortcuts as shortcut (shortcut.key)}
        <RailButton
          icon={shortcut.icon}
          label={shortcut.label}
          active={false}
          badge={shortcut.key}
          onclick={() => {
            if (shortcut.accountId && shortcut.folderId && shortcut.folderPath) {
              setActiveExtension('mail')
              onFolderSelect?.(
                shortcut.accountId,
                shortcut.folderId,
                shortcut.folderPath,
                shortcut.folderName,
                shortcut.folderType || 'folder'
              )
            }
          }}
        />
      {/each}
    {/if}
  </nav>
{/if}