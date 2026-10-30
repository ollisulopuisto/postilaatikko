<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from '@iconify/svelte'
  import { _ } from '$lib/i18n'
  import { replyLaterQueue } from '$lib/stores/replyLater.svelte'
  import { toasts } from '$lib/stores/toast'
  import { UndoOperation } from '../../../../wailsjs/go/app/App'

  let { accountId, conversationId }: { accountId: string; conversationId: string } = $props()
  const inQueue = $derived(replyLaterQueue.has(accountId, conversationId))
  const busy = $derived(replyLaterQueue.isBusy(accountId, conversationId))
  const label = $derived($_(inQueue ? 'replyLater.remove' : 'replyLater.add'))
  onMount(() => { void replyLaterQueue.load().catch(() => {}) })

  async function toggle(event: MouseEvent) {
    event.preventDefault(); event.stopPropagation()
    const target = !inQueue
    try {
      const result = await replyLaterQueue.set(accountId, conversationId, target)
      if (!result?.changed) return
      toasts.success($_(target ? 'replyLater.added' : 'replyLater.removed'), [{
        label: $_('common.undo'),
        onClick: () => {
          void UndoOperation(result.operationId).catch(() => toasts.error($_('toast.undoFailed')))
        },
      }])
    } catch (err) { toasts.error(`${$_('replyLater.failed')}: ${String(err)}`) }
  }
</script>

<button
  type="button" class="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded text-muted-foreground transition-opacity hover:bg-muted hover:text-foreground focus-visible:opacity-100 {inQueue ? 'text-primary' : 'opacity-0 group-hover:opacity-100'}"
  title={label} aria-label={label} aria-pressed={inQueue} aria-busy={busy}
  disabled={busy || !replyLaterQueue.ready || !accountId || !conversationId}
  onclick={toggle} onkeydown={event => event.stopPropagation()}
>
  <Icon icon={busy ? 'mdi:loading' : 'mdi:clock-outline'} class="h-4 w-4 {busy ? 'animate-spin' : ''}" />
</button>
