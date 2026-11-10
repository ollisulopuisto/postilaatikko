<script lang="ts">
  import Icon from '@iconify/svelte'
  import { _ } from '../../i18n'
  let { dispatchAt, now, dispatching = false, onSendNow, onUndo }: {
    dispatchAt: string
    now: number
    dispatching?: boolean
    onSendNow: () => void
    onUndo: () => void
  } = $props()
  // Presentation only. The existing pending-send owner decides when to dispatch.
  const fraction = $derived(Math.max(0, Math.min(1, (new Date(dispatchAt).getTime() - now) / 30_000)) || 0)
</script>

<div data-pending-send-toast class="fixed bottom-4 left-1/2 -translate-x-1/2 z-50 grid w-max max-w-[calc(100vw-2rem)] grid-cols-[auto_minmax(0,1fr)_auto_auto] items-center gap-2 overflow-hidden rounded-lg border border-border bg-popover px-4 py-3 shadow-lg">
  <Icon icon="mdi:send-clock-outline" class="size-4 shrink-0 text-muted-foreground" />
  <span class="min-w-0 text-sm" role="status" aria-live="polite" aria-atomic="true">{dispatching ? $_('pendingSend.sending') : $_('pendingSend.queued')}</span>
  <button
    type="button"
    class="shrink-0 rounded px-2 py-1 text-sm font-medium hover:bg-muted disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    disabled={dispatching}
    onclick={onSendNow}
  >{$_('pendingSend.sendNow')}</button>
  <button
    type="button"
    class="shrink-0 rounded px-2 py-1 text-sm font-medium text-primary hover:bg-muted disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    disabled={dispatching}
    onclick={onUndo}
  >{$_('pendingSend.undo')}</button>
  <div class="pointer-events-none absolute inset-x-0 bottom-0 h-0.5 bg-muted" aria-hidden="true">
    <div data-pending-send-progress class="h-full origin-left bg-primary transition-transform duration-1000 ease-linear motion-reduce:transition-none" style:transform={`scaleX(${dispatching ? 0 : fraction})`}></div>
  </div>
</div>
