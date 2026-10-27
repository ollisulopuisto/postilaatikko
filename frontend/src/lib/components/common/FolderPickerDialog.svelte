<script lang="ts">
  import Icon from '@iconify/svelte'
  import * as Dialog from '$lib/components/ui/dialog'
  import * as Select from '$lib/components/ui/select'
  import { Button } from '$lib/components/ui/button'
  import { folder, account } from '../../../../wailsjs/go/models'
  import { GetFolders } from '../../../../wailsjs/go/app/App'
  import { _ } from '$lib/i18n'
  import { filterFolders, type FolderChoice } from '$lib/sidebar/folderSearch'

  interface Props {
    open: boolean
    title: string
    initialAccountId: string
    accounts: account.Account[]
    excludeFolderId?: string
    onSelect: (folderId: string, folderName: string, accountId: string) => void
  }
  let { open = $bindable(false), title, initialAccountId, accounts, excludeFolderId = '', onSelect }: Props = $props()
  let searchQuery = $state('')
  let selectedAccountId = $state('all')
  let folders = $state<FolderChoice[]>([])
  let foldersLoading = $state(false)
  let failedAccounts = $state<string[]>([])
  let focusedIndex = $state(-1)
  let searchInput = $state<HTMLInputElement | null>(null)
  let listEl = $state<HTMLDivElement | null>(null)
  let active = $state(false)
  let generation = 0

  // GetFolders reads local metadata only. One unavailable account does not hide others.
  $effect(() => {
    if (!open) { generation++; searchQuery = ''; active = false; return }
    const current = ++generation
    selectedAccountId = 'all'
    foldersLoading = true
    folders = []
    failedAccounts = []
    const jobs = accounts.map(async item => {
      const result = await GetFolders(item.id)
      return (result || []).map((f: folder.Folder): FolderChoice => ({ id: f.id, name: f.name, path: f.path, type: f.type, accountId: item.id, accountName: item.name, accountEmail: item.email, label: `${item.name} ${item.email} ${f.path} ${f.name}` }))
    })
    Promise.allSettled(jobs).then(results => {
      if (current !== generation || !open) return
      folders = results.flatMap(result => result.status === 'fulfilled' ? result.value : [])
      failedAccounts = results.flatMap((result, index) => result.status === 'rejected' ? [accounts[index].name] : [])
      foldersLoading = false
    })
    const timer = setTimeout(() => { active = true; searchInput?.focus() }, 0)
    return () => clearTimeout(timer)
  })
  const sourceAccount = $derived(accounts.find(item => item.id === initialAccountId))
  const allMatches = $derived(filterFolders(folders.filter(item => item.id !== excludeFolderId && (selectedAccountId === 'all' || item.accountId === selectedAccountId)), searchQuery)
    .sort((a, b) => Number(b.accountId === initialAccountId) - Number(a.accountId === initialAccountId) || a.accountName.localeCompare(b.accountName) || a.accountEmail.localeCompare(b.accountEmail) || a.accountId.localeCompare(b.accountId) || a.path.localeCompare(b.path)))
  const displayFolders = $derived(allMatches.slice(0, 200))
  $effect(() => { void displayFolders; focusedIndex = displayFolders.length ? 0 : -1 })
  $effect(() => { if (focusedIndex >= 0) listEl?.querySelectorAll('button')[focusedIndex]?.scrollIntoView({ block: 'nearest' }) })
  function select(item: FolderChoice) { onSelect(item.id, item.name, item.accountId) }
  function handleKeydown(event: KeyboardEvent) {
    if (!open || !active || event.altKey || event.metaKey || event.ctrlKey) return
    const target = event.target as HTMLElement | null
    if (target !== searchInput && (!target || !listEl?.contains(target))) return
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault(); event.stopPropagation()
      if (displayFolders.length) focusedIndex = (focusedIndex + (event.key === 'ArrowDown' ? 1 : -1) + displayFolders.length) % displayFolders.length
    }
    if (event.key === 'Enter' && focusedIndex >= 0 && displayFolders[focusedIndex]) {
      event.preventDefault(); event.stopPropagation(); select(displayFolders[focusedIndex])
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />
<Dialog.Root bind:open>
  <Dialog.Content class="max-w-lg">
    <Dialog.Header><Dialog.Title>{title}</Dialog.Title></Dialog.Header>
    <p class="text-xs text-muted-foreground">{$_('search.sourceAccount')}: {sourceAccount?.name || sourceAccount?.email || initialAccountId}</p>
    <input bind:this={searchInput} bind:value={searchQuery} type="search" aria-label={$_('contextMenu.searchFolders')}
      placeholder={$_('contextMenu.searchFolders')} class="h-10 w-full rounded-md border border-input bg-background px-3 text-sm" />
    {#if accounts.length > 1}
      <Select.Root value={selectedAccountId} onValueChange={(value) => { if (value) selectedAccountId = value }}>
        <Select.Trigger><Select.Value>{selectedAccountId === 'all' ? $_('search.allAccounts') : accounts.find(item => item.id === selectedAccountId)?.name}</Select.Value></Select.Trigger>
        <Select.Content>
          <Select.Item value="all" label={$_('search.allAccounts')} />
          {#each accounts as item (item.id)}<Select.Item value={item.id} label={item.name || item.email} />{/each}
        </Select.Content>
      </Select.Root>
    {/if}
    {#if failedAccounts.length}<p class="text-xs text-destructive" role="status">{$_('search.unavailableAccounts')}: {failedAccounts.join(', ')}</p>{/if}
    <div bind:this={listEl} class="border border-border rounded-md max-h-72 overflow-y-auto" role="listbox" aria-label={title} aria-busy={foldersLoading}>
      {#if foldersLoading}
        <p class="p-3 text-sm text-muted-foreground">{$_('common.loading')}</p>
      {:else}
        {#each displayFolders as item, index (item.id)}
          {#if index === 0 || displayFolders[index - 1].accountId !== item.accountId}
            <div class="px-3 pt-4 pb-2" data-account-heading={item.accountId}>
              <p class="text-sm font-medium text-muted-foreground">{item.accountName || item.accountEmail}</p>
              <p class="text-xs text-muted-foreground">{item.accountEmail}</p>
              {#if item.accountId !== initialAccountId}
                <p class="mt-1 text-xs text-amber-700 dark:text-amber-300" data-cross-account>
                  {$_('search.crossAccount')}: {sourceAccount?.name || sourceAccount?.email || initialAccountId} → {item.accountName || item.accountEmail}
                </p>
              {/if}
            </div>
          {/if}
          <button type="button" role="option" aria-selected={index === focusedIndex}
            class="w-full flex gap-2 items-start px-5 py-2 text-left hover:bg-muted/50 {index === focusedIndex ? 'bg-muted/50' : ''}"
            aria-label={`${item.path || item.name} · ${item.accountName} · ${item.accountEmail}`}
            onclick={() => select(item)}>
            <Icon icon="mdi:folder-outline" class="w-4 h-4 shrink-0 mt-0.5" />
            <span class="min-w-0 flex-1 text-sm break-words">{item.path || item.name}</span>
          </button>
        {:else}<p class="p-3 text-sm text-muted-foreground">{$_('contextMenu.noFoldersAvailable')}</p>{/each}
      {/if}
    </div>
    {#if allMatches.length > 200}<p class="text-xs text-muted-foreground">{$_('search.refineFolders')}</p>{/if}
    <Dialog.Footer><Button variant="outline" onclick={() => (open = false)}>{$_('common.cancel')}</Button></Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
