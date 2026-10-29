<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import Icon from '@iconify/svelte'
  import MessageList from './lib/components/list/MessageList.svelte'
  import ConversationViewer from './lib/components/viewer/ConversationViewer.svelte'
  import ToastContainer from './lib/components/ui/toast/ToastContainer.svelte'
  import VirtualFolderDialog from './lib/components/sidebar/VirtualFolderDialog.svelte'
  import { _ } from './lib/i18n'
  import { accountStore } from './lib/stores/accounts.svelte'
  import { virtualFoldersStore } from './lib/stores/virtualFolders.svelte'
  import { getFocusedPane, setFocusedPane, isInputElement } from './lib/stores/keyboard.svelte'
  import { isDialogGuardActive } from './lib/stores/dialogGuard'
  import { KEY } from './lib/keyboard/shortcuts'
  import { createMailWindowState, navigateMailWindow, selectMailWindowConversation, type MailScope } from './lib/windows/mailWindow'
  import { toasts } from './lib/stores/toast'
  import { publishUndoOperationCreated, publishUndoOperationCompleted } from './lib/components/list/undoMutationEvents'
  import { runMessageMutation } from './lib/components/list/mutationInFlight'
  import type { virtualfolder } from '../wailsjs/go/models'
  import { GetAccounts, GetFolders, GetFolderTree, GetMessage, OpenMailWindow, OpenComposerWindow, MarkAsRead, ArchiveWithUndo, UndoOperation } from '../wailsjs/go/app/App'
  import { EventsEmit, WindowSetTitle } from '../wailsjs/runtime/runtime'

  let { config }: { config: MailScope } = $props()
  // A process per window gives each component/store its own search, checked
  // rows, scroll memory and reader mode. Primary UI state is never persisted.
  let windowState = $state(createMailWindowState(untrack(() => config)))
  let scopes = $state<MailScope[]>([])
  let loading = $state(true)
  let navigationError = $state('')
  let listWidth = $state(360)
  let readerOnly = $state(false)
  let showVirtualFolderDialog = $state(false)
  let pendingSearchQuery = $state('')
  let pendingSearchFolderId = $state<string | null>(null)
  let list: MessageList | undefined
  let viewer = $state<ConversationViewer | undefined>(undefined)
  const scopeKey = (scope: MailScope) => JSON.stringify([scope.accountId, scope.folderId])
  const currentKey = $derived(scopeKey(windowState.scope))

  function label(scope: MailScope) {
    if (scope.accountId === 'unified' && scope.folderType === 'all') return $_('mailWindow.allMail')
    if (scope.accountId === 'unified' && scope.folderType === 'spam') return $_('mailWindow.spam')
    if (scope.accountId === 'unified') return $_(`sidebar.${({ inbox: 'allInboxes', archive: 'archived' }[scope.folderType] || scope.folderType)}`)
    return scope.folderName
  }
  async function loadNavigation() {
    try {
      const accounts = (await GetAccounts()) ?? []
      const entries = await Promise.all(accounts.map(async account => ({
        account, folders: (await GetFolderTree(account.id)) ?? [], loading: false, syncing: false, error: null, lastSync: null,
      })))
      accountStore.accounts = entries
      const real = await Promise.all(accounts.map(async account => ((await GetFolders(account.id)) ?? []).map(folder => ({
        accountId: account.id, folderId: folder.id, folderName: `${account.name || account.email} · ${folder.path}`, folderType: folder.type,
      }))))
      await virtualFoldersStore.load(true)
      scopes = [
        ...['inbox', 'sent', 'drafts', 'trash', 'spam', 'archive', 'all', 'starred'].map(type => ({ accountId: 'unified', folderId: type, folderName: '', folderType: type })),
        ...virtualFoldersStore.folders.map(folder => ({ accountId: 'virtual', folderId: folder.id, folderName: folder.name, folderType: 'virtual' })),
        ...real.flat(),
      ]
      navigationError = ''
    } catch (error) { navigationError = String(error) }
    finally { loading = false }
  }
  function navigate(value: string) {
    const scope = scopes.find(scope => scopeKey(scope) === value)
    if (!scope) return
    windowState = navigateMailWindow(windowState, scope)
    readerOnly = false
    setFocusedPane('messageList')
  }
  function select(threadId: string, folderId: string, accountId: string) {
    windowState = selectMailWindowConversation(windowState, threadId, folderId, accountId)
  }
  async function run(action: () => Promise<unknown>) {
    try { await action() }
    catch (error) { toasts.error(String(error)) }
  }
  async function newWindow() {
    const { accountId, folderId, folderName, folderType } = windowState.scope
    await run(() => OpenMailWindow(accountId, folderId, folderName, folderType))
  }
  async function compose() {
    const accountId = accountStore.accounts.some(item => item.account.id === windowState.scope.accountId)
      ? windowState.scope.accountId : accountStore.accounts[0]?.account.id
    if (accountId) await run(() => OpenComposerWindow(accountId, 'new', '', '', ''))
  }
  async function reply(mode: 'reply' | 'reply-all' | 'forward', messageId: string) {
    // Row menus can target an unselected row from a different account.
    await run(async () => {
      const message = await GetMessage(messageId)
      if (!message?.accountId) throw new Error($_('mailWindow.messageUnavailable'))
      await OpenComposerWindow(message.accountId, mode, messageId, '', '')
    })
  }
  function refresh() {
    const scope = windowState.scope
    EventsEmit('messages:updated', { accountId: scope.accountId, folderId: scope.folderId })
    // Cached reads refresh on focus; existing list policy owns explicit header work.
    list?.handleActionComplete(false)
  }
  async function archive(ids: string[]) {
    await run(() => runMessageMutation(ids, async () => {
      const result = await ArchiveWithUndo(ids)
      const operation = result.operationId
      publishUndoOperationCreated(operation, ids, 'archive')
      toasts.success($_('toast.archived'), [{ label: $_('common.undo'), onClick: () => run(async () => {
        await UndoOperation(operation)
        publishUndoOperationCompleted(operation)
        refresh()
      }) }])
      list?.handleActionComplete(true)
    }))
  }
  function closeReader() { readerOnly = false; windowState = { ...windowState, thread: null }; setFocusedPane('messageList') }
  function keydown(event: KeyboardEvent) {
    if (isInputElement(event.target) || isDialogGuardActive() || event.defaultPrevented) return
    if ((event.metaKey || event.ctrlKey) && event.shiftKey && event.key.toLowerCase() === 'n') { event.preventDefault(); void newWindow(); return }
    if (event.ctrlKey || event.metaKey || event.altKey) return
    if (event.key === 'Escape') { closeReader(); return }
    if (event.key === '/') { event.preventDefault(); list?.toggleSearchFocus(); return }
    if (event.key === 'c') { event.preventDefault(); void compose(); return }
    if (getFocusedPane() === 'viewer') {
      if (event.key === 'r') { event.preventDefault(); viewer?.reply(); return }
      if (event.key === 'a') { event.preventDefault(); viewer?.replyAll(); return }
      if (event.key === 'f') { event.preventDefault(); viewer?.forward(); return }
      if (KEY.LIST_NEXT(event)) { event.preventDefault(); viewer?.scrollDown(); return }
      if (KEY.LIST_PREV(event)) { event.preventDefault(); viewer?.scrollUp(); return }
      return
    }
    if (KEY.LIST_NEXT(event)) { event.preventDefault(); list?.selectNext(); return }
    if (KEY.LIST_PREV(event)) { event.preventDefault(); list?.selectPrevious(); return }
    if (KEY.LIST_OPEN(event)) { event.preventDefault(); list?.openSelected(); setFocusedPane('viewer'); return }
    if (KEY.LIST_TOGGLE_CHECK(event)) { event.preventDefault(); list?.toggleCheck() }
  }
  onMount(() => {
    setFocusedPane('messageList')
    void loadNavigation()
  })
  $effect(() => { WindowSetTitle(`${label(windowState.scope)} — Postilaatikko`) })
  $effect(() => {
    const request = virtualFoldersStore.saveSearchRequest
    if (!request) return
    virtualFoldersStore.clearSaveSearchRequest()
    pendingSearchQuery = request.query
    pendingSearchFolderId = request.folderId
    showVirtualFolderDialog = true
  })
</script>

<svelte:window onkeydown={keydown} onfocus={() => { refresh(); void loadNavigation() }} />
<div class="mail-window h-screen flex flex-col overflow-hidden bg-background text-foreground">
  <header class="flex items-center gap-2 border-b border-border p-2">
    <select class="min-w-0 flex-1 rounded border border-border bg-background p-2 text-sm" aria-label={$_('mailWindow.folder')} value={currentKey} onchange={event => navigate(event.currentTarget.value)} disabled={loading}>
      {#if !scopes.some(scope => scopeKey(scope) === currentKey)}<option value={currentKey}>{label(windowState.scope)}</option>{/if}
      {#each scopes as scope (scopeKey(scope))}<option value={scopeKey(scope)}>{label(scope)}</option>{/each}
    </select>
    <button class="rounded p-2 hover:bg-muted" title={$_('mailWindow.newWindow')} aria-label={$_('mailWindow.newWindow')} onclick={newWindow}><Icon icon="mdi:open-in-new" width="20" /></button>
    <button class="rounded p-2 hover:bg-muted" title={$_('mailWindow.compose')} aria-label={$_('mailWindow.compose')} onclick={compose}><Icon icon="mdi:pencil" width="20" /></button>
    <label class="width-control text-xs">{$_('mailWindow.listWidth')}<input type="range" min="280" max="600" step="10" bind:value={listWidth} /></label>
    {#if windowState.thread}<button class="rounded p-2 hover:bg-muted" onclick={() => readerOnly = !readerOnly} aria-pressed={readerOnly} title={$_('mailWindow.focus')} aria-label={$_('mailWindow.focus')}><Icon icon="mdi:arrow-expand" width="20" /></button>{/if}
  </header>
  {#if navigationError}<div role="alert" class="p-2 text-sm">{navigationError}<button class="underline ml-2" onclick={loadNavigation}>{$_('common.retry')}</button></div>{/if}
  <main class:reader-only={readerOnly} class:has-thread={!!windowState.thread} class="flex flex-1 min-h-0 overflow-hidden" style={`--list-width: ${listWidth}px`}>
    <section class="mail-list min-h-0 overflow-hidden border-r border-border" role="presentation" onclick={() => setFocusedPane('messageList')}>
      <MessageList bind:this={list} accountId={windowState.scope.accountId} folderId={windowState.scope.folderId} folderName={label(windowState.scope)} folderType={windowState.scope.folderType}
        isFocused={getFocusedPane() === 'messageList'} onConversationSelect={select} onReply={reply}
        onRowActionComplete={auto => { if (auto) windowState = { ...windowState, thread: null } }}
        onBulkMarkRead={ids => void run(() => MarkAsRead(ids))} onBulkArchive={archive} />
    </section>
    <section class="mail-reader flex-1 min-w-0 min-h-0" role="presentation" onclick={() => setFocusedPane('viewer')}>
      {#if windowState.thread}
        {@const thread = windowState.thread}
        <ConversationViewer bind:this={viewer} threadId={thread.threadId} folderId={thread.folderId} accountId={thread.accountId} folderType={windowState.scope.folderType}
          isFocused={getFocusedPane() === 'viewer'} showBackButton onBack={closeReader} onReply={reply}
          onEditDraft={draftId => run(() => OpenComposerWindow(thread.accountId, 'new', '', draftId, ''))}
          onToggleThreadFocus={() => readerOnly = !readerOnly} inFocusMode={readerOnly}
          onActionComplete={() => list?.handleActionComplete(true)}
          onComposeToAddress={address => run(() => OpenComposerWindow(thread.accountId, 'new', '', '', `mailto:${encodeURIComponent(address)}`))} />
      {:else}<div class="h-full flex items-center justify-center text-muted-foreground text-sm">{$_('mailWindow.selectMessage')}</div>{/if}
    </section>
  </main>
</div>
<ToastContainer />
<VirtualFolderDialog bind:open={showVirtualFolderDialog} virtualFolder={null} accounts={accountStore.accounts}
  initialSearchQuery={pendingSearchQuery} initialFolderId={pendingSearchFolderId}
  onSave={async value => { await virtualFoldersStore.save(value as virtualfolder.VirtualFolder); await loadNavigation() }} />
<style>
  .mail-list { width: var(--list-width); flex-shrink: 0; }
  .reader-only .mail-list { display: none; }
  .width-control { display: flex; align-items: center; gap: 0.5rem; }
  .width-control input { width: 80px; }
  @media (max-width: 800px) {
    .mail-list { width: 100%; }
    .has-thread .mail-list { display: none; }
    main:not(.has-thread) .mail-reader { display: none; }
    .width-control { display: none; }
  }
</style>
