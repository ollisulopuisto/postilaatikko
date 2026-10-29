<script lang="ts">
  import { untrack } from 'svelte'
  import { _ } from 'svelte-i18n'
  import { StartVirtualFolderOnlineCount, GetVirtualFolderOnlineCount, CancelVirtualFolderOnlineCount } from '../../../../wailsjs/go/app/App'
  import type { app } from '../../../../wailsjs/go/models'

  let { folderId, revision }: { folderId: string; revision: number } = $props()
  let countState = $state<app.VirtualFolderOnlineCount | null>(null)
  let error = $state('')
  let refreshing = $state(false)
  $effect(() => {
    const view = folderId
    void revision
    const session = crypto.randomUUID()
    let active = true
    let timer: ReturnType<typeof setTimeout> | undefined
    refreshing = true
    error = ''
    if (untrack(() => countState?.virtualFolderId) !== view) countState = null
    async function poll() {
      try {
        const next = await GetVirtualFolderOnlineCount(session)
        if (!active) return
        countState = next
        refreshing = next.running
        if (next.running) timer = setTimeout(() => void poll(), 1000)
      } catch (err) {
        if (active) { error = String(err); refreshing = false }
      }
    }
    void StartVirtualFolderOnlineCount(session, view).then(() => {
      if (active) void poll()
      else void CancelVirtualFolderOnlineCount(session)
    }).catch(err => {
      if (active) { error = String(err); refreshing = false }
    })
    return () => {
      active = false
      clearTimeout(timer)
      void CancelVirtualFolderOnlineCount(session)
    }
  })
</script>

<div class="px-3 py-1 text-xs text-muted-foreground" role="status" aria-live="polite">
  {#if refreshing}
    {$_('onlineCount.counting')}
  {:else if countState?.complete}
    {$_('onlineCount.complete', { values: { total: countState.total, unread: countState.unread } })}
  {:else}
    {$_('onlineCount.incomplete')}
  {/if}
  {#if error}<span class="text-destructive"> {error}</span>{/if}
  {#each countState?.issues || [] as issue (issue.accountId)}
    <span class="text-destructive"> {issue.accountName}: {issue.message}</span>
  {/each}
</div>
