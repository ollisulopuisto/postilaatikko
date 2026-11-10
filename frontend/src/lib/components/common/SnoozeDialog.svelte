<script lang="ts">
  import Icon from '@iconify/svelte'
  import * as Dialog from '$lib/components/ui/dialog'
  import { Button } from '$lib/components/ui/button'
  import { _ } from '$lib/i18n'
  import { toasts } from '$lib/stores/toast'
  import { dialogGuardOpen, dialogGuardClose } from '$lib/stores/dialogGuard'
  import { snoozePresets, parseCustomSnooze, toDatetimeLocal } from '$lib/snooze/presets'
  import type { SnoozeTarget } from '$lib/stores/snoozeDialog.svelte'
  import { SnoozeConversationWithUndo, UndoOperation } from '../../../../wailsjs/go/app/App'

  let { open = $bindable(false), targets, onDone }: {
    open: boolean
    targets: SnoozeTarget[]
    onDone?: () => void
  } = $props()

  let now = $state(new Date())
  let custom = $state('')
  let customError = $state('')
  let busy = $state(false)

  $effect(() => {
    if (!open) return
    now = new Date()
    const base = new Date(now.getTime() + 24 * 3600 * 1000)
    base.setHours(8, 0, 0, 0)
    custom = toDatetimeLocal(base)
    customError = ''
    dialogGuardOpen()
    return () => dialogGuardClose()
  })

  const presets = $derived(snoozePresets(now))
  const minValue = $derived(toDatetimeLocal(new Date(now.getTime() + 60 * 1000)))

  function describe(date: Date): string {
    return new Intl.DateTimeFormat(undefined, { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' }).format(date)
  }

  async function snoozeUntil(wakeAt: Date) {
    if (busy || targets.length === 0) return
    busy = true
    const iso = wakeAt.toISOString()
    const operationIds: string[] = []
    let changed = 0
    let failures = 0
    for (const target of targets) {
      try {
        const result = await SnoozeConversationWithUndo(target.accountId, target.conversationId, iso)
        if (result.changed) {
          changed += 1
          if (result.operationId) operationIds.push(result.operationId)
        }
      } catch (err) {
        failures += 1
        console.error('Snooze failed:', err)
      }
    }
    busy = false
    if (failures > 0) toasts.error($_('snooze.failed'))
    if (changed > 0) {
      const message = $_(changed === 1 ? 'snooze.doneUntil' : 'snooze.doneUntilMany', { values: { when: describe(wakeAt), count: changed } })
      toasts.success(message, operationIds.length > 0 ? [{
        label: $_('common.undo'),
        onClick: () => { void Promise.all(operationIds.map(id => UndoOperation(id))).catch(() => toasts.error($_('toast.undoFailed'))) },
      }] : undefined)
    }
    if (changed > 0 || failures === 0) {
      open = false
      onDone?.()
    }
  }

  function submitCustom(event: Event) {
    event.preventDefault()
    const parsed = parseCustomSnooze(custom, new Date())
    if (!parsed.ok) {
      customError = $_(`snooze.error.${parsed.reason}`)
      return
    }
    customError = ''
    void snoozeUntil(parsed.wakeAt)
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="max-w-sm">
    <Dialog.Header>
      <Dialog.Title>{$_('snooze.title')}</Dialog.Title>
      <Dialog.Description>{$_('snooze.description')}</Dialog.Description>
    </Dialog.Header>
    <div class="grid gap-1" role="group" aria-label={$_('snooze.presets')}>
      {#each presets as preset (preset.id)}
        <button type="button"
          class="flex items-center justify-between gap-3 rounded-md px-3 py-2 text-left text-sm hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
          disabled={busy}
          onclick={() => void snoozeUntil(preset.wakeAt)}>
          <span class="flex items-center gap-2"><Icon icon="mdi:clock-time-four-outline" class="size-4 text-muted-foreground" />{$_(`snooze.preset.${preset.id}`)}</span>
          <span class="text-xs text-muted-foreground">{describe(preset.wakeAt)}</span>
        </button>
      {/each}
    </div>
    <form class="grid gap-2 border-t border-border pt-3" onsubmit={submitCustom}>
      <label class="grid gap-1 text-sm" for="snooze-custom">
        {$_('snooze.custom')}
        <input id="snooze-custom" type="datetime-local" min={minValue} bind:value={custom}
          aria-invalid={customError ? 'true' : undefined} aria-describedby={customError ? 'snooze-custom-error' : undefined}
          class="rounded-md border border-input bg-background px-3 py-2" />
      </label>
      {#if customError}<p id="snooze-custom-error" role="alert" class="text-xs text-destructive">{customError}</p>{/if}
      <Dialog.Footer>
        <Button type="button" variant="outline" onclick={() => open = false}>{$_('common.cancel')}</Button>
        <Button type="submit" disabled={busy}>{$_('snooze.confirm')}</Button>
      </Dialog.Footer>
    </form>
  </Dialog.Content>
</Dialog.Root>
