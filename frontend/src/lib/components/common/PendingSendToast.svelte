<script lang="ts">
  import Icon from '@iconify/svelte'
  import { _ } from '../../i18n'
  let { dispatchAt, now, dispatching = false, manualRecovery = false, summary, onSendNow, onUndo, onEditRecipients }: {
    dispatchAt: string
    now: number
    manualRecovery?: boolean
    dispatching?: boolean
    onSendNow: () => void
    summary?: { mode: string; to: string; cc: string; bcc: string }
    onUndo: () => void
    onEditRecipients?: () => void
  } = $props()
  // Presentation only. The existing pending-send owner decides when to dispatch.
  const fraction = $derived(Math.max(0, Math.min(1, (new Date(dispatchAt).getTime() - now) / 30_000)) || 0)
</script>

<div data-pending-send-toast class="fixed bottom-4 left-1/2 -translate-x-1/2 z-50 flex w-max max-w-[calc(100vw-2rem)] flex-wrap items-center gap-x-2 gap-y-1 overflow-hidden max-h-[calc(100dvh-2rem)] rounded-lg border border-border bg-popover px-4 py-3 shadow-lg">
  <Icon icon="mdi:send-clock-outline" class="size-4 shrink-0 text-muted-foreground" />
  <span class="min-w-0 flex-1 basis-24 text-sm" role="status" aria-live="polite" aria-atomic="true">{dispatching ? $_('pendingSend.sending') : manualRecovery ? $_('pendingSend.recoveryRequired') : $_('pendingSend.queued')}</span>
  {#if onEditRecipients}
    <button
      type="button"
      data-pending-send-edit-recipients
      class="shrink-0 whitespace-nowrap rounded px-2 py-1 text-sm font-medium hover:bg-muted disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      disabled={dispatching}
      onclick={onEditRecipients}
    >{$_('pendingSend.editRecipients')}</button>
  {/if}
  <button
    type="button"
    class="shrink-0 whitespace-nowrap rounded px-2 py-1 text-sm font-medium hover:bg-muted disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    disabled={dispatching}
    onclick={onSendNow}
  >{$_('pendingSend.sendNow')}</button>
  <button
    type="button"
    class="shrink-0 whitespace-nowrap rounded px-2 py-1 text-sm font-medium text-primary hover:bg-muted disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    disabled={dispatching}
    onclick={onUndo}
  >{$_('pendingSend.undo')}</button>
  {#if summary && (summary.to || summary.cc || summary.bcc)}
    <div data-pending-send-recipients class="min-w-0 basis-full max-h-[min(8rem,30dvh)] overflow-y-auto break-words text-xs text-muted-foreground">
      {#if summary.mode === 'reply-all'}<strong>{$_('pendingSend.replyAll')}</strong>{:else if summary.mode === 'reply'}<strong>{$_('pendingSend.reply')}</strong>{/if}
      {#if summary.to}<p><span class="font-medium">{$_('pendingSend.to')}:</span> {summary.to}</p>{/if}
      {#if summary.cc}<p><span class="font-medium">{$_('pendingSend.cc')}:</span> {summary.cc}</p>{/if}
      {#if summary.bcc}<p><span class="font-medium">{$_('pendingSend.bcc')}:</span> {summary.bcc}</p>{/if}
    </div>
  {/if}
  <div class="pointer-events-none absolute inset-x-0 bottom-0 h-0.5 bg-muted" aria-hidden="true">
    <div data-pending-send-progress class="h-full origin-left bg-primary transition-transform duration-1000 ease-linear motion-reduce:transition-none" style:transform={`scaleX(${dispatching ? 0 : fraction})`}></div>
  </div>
</div>
