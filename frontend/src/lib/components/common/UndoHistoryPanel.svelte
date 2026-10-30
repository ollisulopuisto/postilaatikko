<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from '@iconify/svelte'
  import * as Dialog from '$lib/components/ui/dialog'
  import { _ } from '$lib/i18n'
  import { addToast } from '$lib/stores/toast'
  import { publishUndoOperationCompleted } from '$lib/components/list/undoMutationEvents'
  // @ts-ignore - wailsjs path
  import { GetUndoHistory, UndoOperation } from '../../../../wailsjs/go/app/App.js'
  // @ts-ignore - wailsjs runtime
  import { EventsOn } from '../../../../wailsjs/runtime/runtime'

  type HistoryEntry = {
    operationId: string
    action: string
    description: string
    createdAt: string
    expiresAt: string
    messageCount: number
  }

  let { open = $bindable(false) } = $props<{ open?: boolean }>()
  let entries = $state<HistoryEntry[]>([])
  let loading = $state(false)
  let error = $state(false)
  let busyIds = $state<string[]>([])
  let refreshQueued = false

  async function refresh() {
    if (loading) {
      refreshQueued = true
      return
    }
    loading = true
    error = false
    try {
      entries = await GetUndoHistory(50) as HistoryEntry[]
    } catch (err) {
      console.error('Failed to load undo history:', err)
      error = true
    } finally {
      loading = false
      if (refreshQueued) {
        refreshQueued = false
        void refresh()
      }
    }
  }

  async function undo(entry: HistoryEntry) {
    if (busyIds.includes(entry.operationId)) return
    busyIds = [...busyIds, entry.operationId]
    try {
      const description = await UndoOperation(entry.operationId)
      publishUndoOperationCompleted(entry.operationId)
      addToast({ type: 'success', message: $_('toast.undone', { values: { description } }) })
      entries = entries.filter(item => item.operationId !== entry.operationId)
    } catch (err) {
      console.error('Undo history action failed:', err)
      addToast({ type: 'error', message: $_('toast.undoFailed') })
      await refresh()
    } finally {
      busyIds = busyIds.filter(id => id !== entry.operationId)
    }
  }

  function timeLabel(value: string): string {
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return ''
    return date.toLocaleString()
  }

  function remainingLabel(value: string): string {
    const expires = new Date(value).getTime()
    if (Number.isNaN(expires)) return ''
    const seconds = Math.max(0, Math.ceil((expires - Date.now()) / 1000))
    const minutes = Math.ceil(seconds / 60)
    if (minutes < 60) return $_('undoHistory.minutesLeft', { values: { count: minutes } })
    const hours = Math.ceil(minutes / 60)
    return $_('undoHistory.hoursLeft', { values: { count: hours } })
  }

  onMount(() => {
    EventsOn('undo:history-changed', () => { if (open) void refresh() })
    EventsOn('undo:completed', () => { if (open) void refresh() })
  })
</script>

<Dialog.Root bind:open onOpenChange={(value) => { if (value) void refresh() }}>
  <Dialog.Content class="flex max-h-[min(620px,85vh)] max-w-xl flex-col overflow-hidden">
    <Dialog.Header>
      <Dialog.Title>{$_('undoHistory.title')}</Dialog.Title>
      <Dialog.Description>{$_('undoHistory.description')}</Dialog.Description>
    </Dialog.Header>

    <div class="min-h-0 flex-1 overflow-y-auto scrollbar-thin" aria-live="polite">
      {#if loading && entries.length === 0}
        <p class="py-8 text-center text-sm text-muted-foreground">{$_('undoHistory.loading')}</p>
      {:else if error && entries.length === 0}
        <div class="py-8 text-center">
          <p class="text-sm text-destructive">{$_('undoHistory.loadFailed')}</p>
          <button type="button" class="mt-3 text-sm text-primary hover:underline" onclick={() => void refresh()}>{$_('undoHistory.retry')}</button>
        </div>
      {:else if entries.length === 0}
        <p class="py-8 text-center text-sm text-muted-foreground">{$_('undoHistory.empty')}</p>
      {:else}
        <ol class="divide-y divide-border">
          {#each entries as entry (entry.operationId)}
            <li class="flex items-center gap-3 py-3">
              <div class="min-w-0 flex-1">
                <p class="truncate text-sm font-medium" title={entry.description}>{entry.description || entry.action}</p>
                <p class="mt-1 text-xs text-muted-foreground">
                  {timeLabel(entry.createdAt)}
                  {#if entry.messageCount > 0}
                    <span> · {$_('undoHistory.messages', { values: { count: entry.messageCount } })}</span>
                  {/if}
                  <span> · {remainingLabel(entry.expiresAt)}</span>
                </p>
              </div>
              <button type="button" class="inline-flex shrink-0 items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-sm hover:bg-muted disabled:opacity-50" disabled={busyIds.includes(entry.operationId)} onclick={() => void undo(entry)}>
                {#if busyIds.includes(entry.operationId)}
                  <Icon icon="lucide:loader-circle" class="h-4 w-4 animate-spin" />
                {:else}
                  <Icon icon="lucide:undo-2" class="h-4 w-4" />
                {/if}
                {$_('undoHistory.undo')}
              </button>
            </li>
          {/each}
        </ol>
      {/if}
    </div>
  </Dialog.Content>
</Dialog.Root>
