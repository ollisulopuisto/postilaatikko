<script lang="ts">
  import Icon from '@iconify/svelte'
  import { ContextMenu as ContextMenuPrimitive } from 'bits-ui'
  import {
    ContextMenuContent,
    ContextMenuItem,
  } from '$lib/components/ui/context-menu'
  import {
    MarkAllFolderMessagesAsRead,
    MarkAllFolderMessagesAsUnread,
  } from '../../../../wailsjs/go/app/App'
  import { toasts } from '$lib/stores/toast'
  import { sidebarPins } from '$lib/stores/sidebarPins.svelte'
  import type { Snippet } from 'svelte'
  import { _ } from '$lib/i18n'

  interface Props {
    folderId: string
    // Set together to offer pinning the folder to the sidebar (P-007).
    accountId?: string
    folder?: { id: string; path: string; name: string }
    children?: Snippet
  }

  let {
    folderId,
    accountId,
    folder,
    children,
  }: Props = $props()

  let pinned = $derived(accountId !== undefined && sidebarPins.has(accountId, folderId))

  function handleTogglePin() {
    if (!accountId || !folder) return
    sidebarPins.toggle(accountId, folder)
  }

  async function handleMarkAllRead() {
    try {
      await MarkAllFolderMessagesAsRead(folderId)
      toasts.success($_('toast.markedAllAsRead'))
    } catch (err) {
      console.error('Mark all as read failed:', err)
      toasts.error($_('toast.failedToMarkAllAsRead'))
    }
  }

  async function handleMarkAllUnread() {
    try {
      await MarkAllFolderMessagesAsUnread(folderId)
      toasts.success($_('toast.markedAllAsUnread'))
    } catch (err) {
      console.error('Mark all as unread failed:', err)
      toasts.error($_('toast.failedToMarkAllAsUnread'))
    }
  }
</script>

<ContextMenuPrimitive.Root>
  <ContextMenuPrimitive.Trigger>
    {#if children}
      {@render children()}
    {/if}
  </ContextMenuPrimitive.Trigger>

  <ContextMenuContent>
    <ContextMenuItem onSelect={handleMarkAllRead}>
      <Icon icon="mdi:email-check-outline" class="mr-2 h-4 w-4" />
      {$_('contextMenu.markAllAsRead')}
    </ContextMenuItem>
    <ContextMenuItem onSelect={handleMarkAllUnread}>
      <Icon icon="mdi:email-outline" class="mr-2 h-4 w-4" />
      {$_('contextMenu.markAllAsUnread')}
    </ContextMenuItem>
    {#if accountId !== undefined && folder !== undefined}
      <ContextMenuItem onSelect={handleTogglePin}>
        <Icon icon={pinned ? 'mdi:pin-off-outline' : 'mdi:pin-outline'} class="mr-2 h-4 w-4" />
        {pinned ? $_('contextMenu.unpinFromSidebar') : $_('contextMenu.pinToSidebar')}
      </ContextMenuItem>
    {/if}
  </ContextMenuContent>
</ContextMenuPrimitive.Root>
