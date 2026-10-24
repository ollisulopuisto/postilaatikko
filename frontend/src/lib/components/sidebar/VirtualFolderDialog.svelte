<script lang="ts">
  import Icon from '@iconify/svelte'
  import * as Dialog from '$lib/components/ui/dialog'
  import { Button } from '$lib/components/ui/button'
  import type { AccountWithFolders } from '$lib/stores/accounts.svelte'
  import type { folder, virtualfolder } from '../../../../wailsjs/go/models'

  interface Props {
    open: boolean
    virtualFolder: virtualfolder.VirtualFolder | null
    accounts: AccountWithFolders[]
    onSave: (value: { id: string; name: string; orderIndex: number; folderIds: string[] }) => Promise<void>
  }

  let { open = $bindable(false), virtualFolder, accounts, onSave }: Props = $props()
  let name = $state('')
  let selectedIDs = $state<string[]>([])
  let saving = $state(false)
  let error = $state('')

  const selectableFolders = $derived.by(() => {
    const result: { id: string; label: string }[] = []
    const visit = (accountName: string, trees: folder.FolderTree[]) => {
      for (const tree of trees || []) {
        if (tree.folder) result.push({ id: tree.folder.id, label: `${accountName} · ${tree.folder.path || tree.folder.name}` })
        if (tree.children?.length) visit(accountName, tree.children)
      }
    }
    for (const account of accounts) visit(account.account.name || account.account.email, account.folders)
    return result
  })

  $effect(() => {
    if (!open) return
    name = virtualFolder?.name || ''
    selectedIDs = [...(virtualFolder?.folderIds || [])]
    error = ''
  })

  function toggleFolder(id: string) {
    selectedIDs = selectedIDs.includes(id) ? selectedIDs.filter((item) => item !== id) : [...selectedIDs, id]
  }

  async function submit() {
    const trimmed = name.trim()
    if (!trimmed) {
      error = 'Enter a name for this virtual folder.'
      return
    }
    if (selectedIDs.length === 0) {
      error = 'Choose at least one folder.'
      return
    }
    saving = true
    error = ''
    try {
      await onSave({ id: virtualFolder?.id || '', name: trimmed, orderIndex: virtualFolder?.orderIndex || 0, folderIds: selectedIDs })
      open = false
    } catch (cause) {
      console.error('Failed to save virtual folder:', cause)
      error = 'Could not save the virtual folder.'
    } finally {
      saving = false
    }
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="max-w-lg">
    <Dialog.Header>
      <Dialog.Title>{virtualFolder ? 'Edit virtual folder' : 'New virtual folder'}</Dialog.Title>
    </Dialog.Header>
    <label class="grid gap-1 text-sm">
      <span>Name</span>
      <input bind:value={name} maxlength="100" class="h-10 rounded-md border border-input bg-background px-3" />
    </label>
    <div class="max-h-72 overflow-y-auto rounded-md border border-border p-2">
      {#if selectableFolders.length === 0}
        <p class="p-3 text-sm text-muted-foreground">Folders are still loading, or no folders are available.</p>
      {:else}
        {#each selectableFolders as item (item.id)}
          <label class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-muted/60">
            <input type="checkbox" checked={selectedIDs.includes(item.id)} onchange={() => toggleFolder(item.id)} />
            <span class="truncate">{item.label}</span>
          </label>
        {/each}
      {/if}
    </div>
    {#if error}<p class="text-sm text-destructive" role="alert">{error}</p>{/if}
    <Dialog.Footer>
      <Button variant="outline" onclick={() => (open = false)}>Cancel</Button>
      <Button disabled={saving || selectableFolders.length === 0} onclick={submit}>
        {#if saving}<Icon icon="mdi:loading" class="mr-1 h-4 w-4 animate-spin" />{/if}
        Save
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
