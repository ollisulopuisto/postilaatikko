<script lang="ts">
  import Icon from '@iconify/svelte'
  // @ts-ignore - wailsjs path
  import { folder } from '../../../../wailsjs/go/models'
  // @ts-ignore - wailsjs path
  import { MoveToFolderWithUndo, UndoOperation } from '../../../../wailsjs/go/app/App.js'
  import FolderContextMenu from './FolderContextMenu.svelte'
  import Self from './FolderTreeItem.svelte'
  import { toasts } from '$lib/stores/toast'
  import { _ } from '$lib/i18n'
  import { runMessageMutation } from '$lib/components/list/mutationInFlight'
  import { publishUndoOperationCompleted, publishUndoOperationCreated } from '$lib/components/list/undoMutationEvents'

  interface Props {
    tree: folder.FolderTree
    accountId: string
    selectedFolderId: string
    selectionSource: 'unified' | 'account' | null
    collapsedFolders: Record<string, boolean>
    onFolderSelect?: (f: folder.Folder) => void
    onToggleCollapse?: (folderId: string) => void
    onMessagesMoved?: () => void
  }

  let {
    tree,
    accountId,
    selectedFolderId,
    selectionSource,
    collapsedFolders,
    onFolderSelect,
    onToggleCollapse,
    onMessagesMoved,
  }: Props = $props()

  // Folder type to icon mapping
  const folderIcons: Record<string, string> = {
    inbox: 'mdi:inbox',
    sent: 'mdi:send',
    drafts: 'mdi:file-document-edit-outline',
    trash: 'mdi:delete-outline',
    archive: 'mdi:archive-outline',
    spam: 'mdi:alert-octagon-outline',
    all: 'mdi:email-multiple-outline',
    folder: 'mdi:folder-outline',
  }

  function getFolderIcon(type: string): string {
    return folderIcons[type] || folderIcons.folder
  }

  function isFolderSelected(folderId: string): boolean {
    return selectionSource === 'account' && selectedFolderId === folderId
  }

  let hasChildren = $derived(tree.children && tree.children.length > 0)
  let isCollapsed = $derived(
    hasChildren
      ? collapsedFolders[tree.folder!.id] !== false  // collapsed unless explicitly set to false
      : false
  )

  // Drag-and-drop state for receiving message drops on this folder
  let isDragOver = $state(false)
  const undoInFlight = new Set<string>()

  function hasMessagesPayload(e: DragEvent): boolean {
    return !!e.dataTransfer?.types.includes('application/x-aerion-messages')
  }

  function handleDragEnter(e: DragEvent) {
    if (!hasMessagesPayload(e)) return
    e.preventDefault()
    isDragOver = true
  }

  function handleDragOver(e: DragEvent) {
    if (!hasMessagesPayload(e)) return
    e.preventDefault()  // required to allow drop
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
  }

  function handleDragLeave() {
    isDragOver = false
  }

  function showTargetedUndoToast(message: string, operationID: string, messageIds: string[], action: string) {
    const canUndo = typeof operationID === 'string' && operationID.length > 0
    if (canUndo) publishUndoOperationCreated(operationID, messageIds, action)
    let toastId = ''
    const actions = canUndo
      ? [{ label: $_('common.undo'), onClick: () => { void handleTargetedUndo(toastId, operationID) } }]
      : []
    toastId = toasts.success(message, actions)
  }

  async function handleTargetedUndo(toastId: string, operationID: string) {
    if (!operationID || undoInFlight.has(operationID)) return
    undoInFlight.add(operationID)
    toasts.replace(toastId, { message: 'Undoing...', actions: [], duration: 20_000 })
    try {
      const description = await UndoOperation(operationID)
      publishUndoOperationCompleted(operationID)
      toasts.replace(toastId, {
        message: $_('toast.undone', { values: { description } }),
        type: 'success',
        actions: [],
        duration: 4000,
      })
    } catch (err) {
      console.error('Undo failed:', err)
      toasts.replace(toastId, {
        message: $_('toast.undoFailed'),
        type: 'error',
        actions: [],
        duration: 6000,
      })
    } finally {
      undoInFlight.delete(operationID)
    }
  }

  async function handleDrop(e: DragEvent) {
    isDragOver = false
    const raw = e.dataTransfer?.getData('application/x-aerion-messages')
    if (!raw || !tree.folder) return
    e.preventDefault()

    let payload: { messageIds: string[]; sourceAccountId: string }
    try {
      payload = JSON.parse(raw)
    } catch {
      return
    }
    if (!payload.messageIds || payload.messageIds.length === 0) return

    // Same-folder drop: no-op
    if (tree.folder.id === selectedFolderId && selectionSource === 'account') {
      return
    }

    const folderName = tree.folder.name
    try {
      await runMessageMutation(payload.messageIds, async () => {
        const result = await MoveToFolderWithUndo(payload.messageIds, tree.folder!.id)
        if (result.coalesced) return
        onMessagesMoved?.()
        showTargetedUndoToast(
          $_('toast.movedTo', { values: { folder: folderName } }),
          result.operationId,
          payload.messageIds,
          'move-to-folder',
        )
      })
    } catch (err) {
      console.error('Drag-drop move failed:', err)
      toasts.error($_('toast.failedToMove'))
    }
  }
</script>

{#if tree.folder}
  <FolderContextMenu folderId={tree.folder.id} {accountId} folder={tree.folder}>
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="w-full flex items-center rounded-md transition-colors {isFolderSelected(tree.folder.id)
        ? 'bg-primary/10 text-primary font-medium'
        : 'text-foreground hover:bg-muted/50'} {isDragOver ? 'ring-2 ring-primary ring-inset' : ''}"
      data-sidebar-item="folder"
      data-folder-id={tree.folder.id}
      data-folder-type={tree.folder.type}
      data-selected={isFolderSelected(tree.folder.id) ? 'true' : undefined}
      data-has-children={hasChildren ? 'true' : undefined}
      ondragenter={handleDragEnter}
      ondragover={handleDragOver}
      ondragleave={handleDragLeave}
      ondrop={handleDrop}
    >
      {#if hasChildren}
        <button
          type="button"
          class="flex-shrink-0 p-1 ml-1 rounded hover:bg-muted/80 text-muted-foreground transition-colors"
          title={isCollapsed ? $_('sidebar.expand') : $_('sidebar.collapse')}
          aria-label={isCollapsed ? $_('sidebar.expand') : $_('sidebar.collapse')}
          aria-expanded={!isCollapsed}
          onclick={(e: MouseEvent) => {
            e.stopPropagation()
            onToggleCollapse?.(tree.folder!.id)
          }}
        >
          <Icon
            icon={isCollapsed ? 'mdi:chevron-right' : 'mdi:chevron-down'}
            class="w-4 h-4"
          />
        </button>
      {:else}
        <span class="w-6 flex-shrink-0"></span>
      {/if}

      <button
        type="button"
        class="flex-1 min-w-0 flex items-center gap-2 px-1 py-1.5 text-sm text-left truncate"
        onclick={() => {
          onFolderSelect?.(tree.folder!)
          if (hasChildren && isCollapsed) {
            onToggleCollapse?.(tree.folder!.id)
          }
        }}
      >
        <Icon
          icon={getFolderIcon(tree.folder.type)}
          class="w-4 h-4 flex-shrink-0"
        />
        <span data-sidebar-label class="truncate text-left">{tree.folder.type === 'inbox' ? $_('sidebar.inbox') : tree.folder.name}</span>
      </button>

      {#if tree.folder.unreadCount > 0}
        <span
          data-sidebar-badge
          class="mr-2 px-1.5 py-0.5 text-xs font-medium rounded-full bg-primary text-primary-foreground flex-shrink-0"
        >
          {tree.folder.unreadCount}
        </span>
      {/if}
    </div>
  </FolderContextMenu>

  {#if hasChildren && !isCollapsed}
    <div class="ml-4">
      {#each tree.children as childTree (childTree.folder?.id ?? 'unknown')}
        <Self
          tree={childTree}
          {accountId}
          {selectedFolderId}
          {selectionSource}
          {collapsedFolders}
          {onFolderSelect}
          {onToggleCollapse}
          {onMessagesMoved}
        />
      {/each}
    </div>
  {/if}
{/if}
