<script lang="ts">
  import { untrack } from 'svelte'
  import * as AlertDialog from '$lib/components/ui/alert-dialog'
  import type { ComposerApi } from '$lib/composerApi'
  import type { ScheduledJob } from '$lib/scheduledSendApi'
  import { _ } from '$lib/i18n'
  import { buildRepeat, defaultRepeatForm, repeatError, type ListMode, type RepeatRule } from '$lib/scheduledRepeat'

  interface Props {
    open?: boolean
    accountId: string
    api: ComposerApi
    canSchedule: boolean
    sending: boolean
    onSchedule: (due: string, repeat: RepeatRule, listMode: ListMode) => Promise<void>
    setupOnly?: boolean
  }

  let { open = $bindable(false), accountId, api, canSchedule, sending, onSchedule, setupOnly = false }: Props = $props()
  let busy = $state(false)
  let error = $state('')
  let endpoint = $state('http://127.0.0.1:8028')
  let helperKey = $state('')
  let helperAccountId = $state('')
  let clientKey = $state('')
  let keyAvailable = $state(false)
  let keyVerified = $state(false)
  let configured = $state(false)
  let settingsOpen = $state(false)
  let due = $state('')
  let jobs = $state<ScheduledJob[]>([])
  let repeatForm = $state(defaultRepeatForm())
  let listMode = $state<ListMode>('off')
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone

  function defaultTime(): string {
    const value = new Date(Date.now() + 60 * 60 * 1000)
    const pad = (n: number) => String(n).padStart(2, '0')
    return `${value.getFullYear()}-${pad(value.getMonth() + 1)}-${pad(value.getDate())}T${pad(value.getHours())}:${pad(value.getMinutes())}`
  }

  $effect(() => {
    if (!open) return
    const id = accountId
    untrack(() => { load(id) })
  })

  async function load(id: string) {
    busy = true
    error = ''
    due = defaultTime()
    repeatForm = defaultRepeatForm()
    listMode = 'off'
    try {
      const setup = await api.getSendLaterSetup(id)
      if (!open || id !== accountId) return
      endpoint = setup.config.endpoint || 'http://127.0.0.1:8028'
      helperKey = setup.config.helperKey || ''
      helperAccountId = setup.config.helperAccountId || id
      clientKey = setup.clientKey || ''
      keyAvailable = setup.keyAvailable
      configured = !!helperKey && !!setup.config.endpoint && keyAvailable
      keyVerified = configured
      settingsOpen = setupOnly || !configured
      jobs = await api.listScheduledMessages(id) || []
    } catch (err) {
      error = String(err)
    } finally {
      busy = false
    }
  }

  async function initialize() {
    busy = true
    error = ''
    try {
      clientKey = await api.initializeSendLaterClient()
      keyAvailable = true
    } catch (err) {
      error = String(err)
    } finally {
      busy = false
    }
  }

  async function save() {
    busy = true
    error = ''
    try {
      await api.saveSendLaterConfig({ accountId, endpoint, helperKey, helperAccountId })
      configured = true
      settingsOpen = false
    } catch (err) {
      error = String(err)
    } finally {
      busy = false
    }
  }

  async function update(job: ScheduledJob, action: string) {
    busy = true
    error = ''
    try {
      const result = await api.updateScheduledMessage(job.id, action)
      error = result.error || ''
      jobs = await api.listScheduledMessages(accountId) || []
    } catch (err) {
      error = String(err)
    } finally {
      busy = false
    }
  }

  async function schedule() {
    const date = new Date(due)
    if (!Number.isFinite(date.getTime()) || date.getTime() <= Date.now()) {
      error = $_('sendLater.futureTime')
      return
    }
    const problem = repeatError(repeatForm, date)
    if (problem) {
      error = $_(`sendLater.${problem}`)
      return
    }
    open = false
    await onSchedule(date.toISOString(), buildRepeat(repeatForm, timezone), listMode)
  }
</script>

<AlertDialog.Root bind:open>
  <AlertDialog.Content class="max-w-xl max-h-[85vh] overflow-y-auto">
    <AlertDialog.Header>
      <AlertDialog.Title>{$_('sendLater.title')}</AlertDialog.Title>
      <AlertDialog.Description>{$_('sendLater.description')}</AlertDialog.Description>
    </AlertDialog.Header>

    {#if error}<p role="alert" class="text-sm text-destructive break-words">{error}</p>{/if}

    {#if !setupOnly}<div class="space-y-2">
      <label class="block text-sm" for="scheduled-time">{$_('sendLater.deliveryTime')}</label>
      <input id="scheduled-time" type="datetime-local" bind:value={due} disabled={busy || sending}
        class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm" />
      <p class="text-xs text-muted-foreground">{$_('sendLater.timezone')}: {timezone}</p>
      {#if due && Number.isFinite(new Date(due).getTime())}
        <p class="text-xs text-muted-foreground">{new Date(due).toLocaleString(undefined, { timeZoneName: 'short' })}</p>
      {/if}

      <div class="mt-3 space-y-2">
        <label class="flex items-center gap-2 text-sm">
          <input type="checkbox" bind:checked={repeatForm.enabled} disabled={busy || sending} />
          {$_('sendLater.repeat')}
        </label>
        {#if repeatForm.enabled}
          <div class="flex flex-wrap items-center gap-2 text-sm">
            <span>{$_('sendLater.repeatEvery')}</span>
            <input type="number" min="1" max="366" bind:value={repeatForm.every} aria-label={$_('sendLater.repeatEvery')}
              class="w-20 rounded-md border border-border bg-background px-2 py-1" />
            <select bind:value={repeatForm.unit} aria-label={$_('sendLater.repeatUnit')} class="rounded-md border border-border bg-background px-2 py-1">
              <option value="day">{$_('sendLater.unit.day')}</option>
              <option value="week">{$_('sendLater.unit.week')}</option>
              <option value="month">{$_('sendLater.unit.month')}</option>
            </select>
            <select bind:value={repeatForm.ends} aria-label={$_('sendLater.repeatEnds')} class="rounded-md border border-border bg-background px-2 py-1">
              <option value="never">{$_('sendLater.endsNever')}</option>
              <option value="count">{$_('sendLater.endsCount')}</option>
              <option value="until">{$_('sendLater.endsUntil')}</option>
            </select>
            {#if repeatForm.ends === 'count'}
              <input type="number" min="2" max="1000" bind:value={repeatForm.count} aria-label={$_('sendLater.endsCount')}
                class="w-20 rounded-md border border-border bg-background px-2 py-1" />
            {:else if repeatForm.ends === 'until'}
              <input type="date" bind:value={repeatForm.until} aria-label={$_('sendLater.endsUntil')}
                class="rounded-md border border-border bg-background px-2 py-1" />
            {/if}
          </div>
          <p class="text-xs text-muted-foreground">{$_('sendLater.repeatHelp')}</p>
        {/if}
      </div>

      <div class="mt-3 space-y-1">
        <label class="block text-sm" for="list-mode">{$_('sendLater.listMode')}</label>
        <select id="list-mode" bind:value={listMode} disabled={busy || sending} class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm">
          <option value="off">{$_('sendLater.listOff')}</option>
          <option value="bcc">{$_('sendLater.listBcc')}</option>
          <option value="cc">{$_('sendLater.listCc')}</option>
        </select>
        {#if listMode !== 'off'}<p class="text-xs text-muted-foreground">{$_('sendLater.listHelp')}</p>{/if}
      </div>
    </div>{/if}

    <details bind:open={settingsOpen} class="rounded-md border border-border p-3">
      <summary class="cursor-pointer text-sm font-medium">{$_('sendLater.connection')}</summary>
      <div class="mt-3 space-y-3">
        <p class="text-xs text-muted-foreground">{$_('sendLater.pairingHelp')}</p>
        {#if !keyAvailable}
          {#if clientKey}<p class="text-xs text-destructive">{$_('sendLater.keyUnavailable')}</p>{/if}
          <button type="button" onclick={initialize} disabled={busy || !!clientKey}
            class="rounded-md border border-border px-3 py-2 text-sm disabled:opacity-50">{$_('sendLater.createKey')}</button>
        {/if}
        {#if clientKey}
          <label class="block text-xs" for="client-public-key">{$_('sendLater.deviceKey')}</label>
          <input id="client-public-key" value={clientKey} readonly class="w-full rounded border border-border bg-muted p-2 font-mono text-xs" />
        {/if}
        <label class="block text-xs" for="helper-endpoint">{$_('sendLater.endpoint')}</label>
        <input id="helper-endpoint" bind:value={endpoint} oninput={() => configured = false}
          class="w-full rounded border border-border bg-background p-2 text-sm" />
        <label class="block text-xs" for="helper-public-key">{$_('sendLater.helperKey')}</label>
        <input id="helper-public-key" bind:value={helperKey} oninput={() => { configured = false; keyVerified = false }}
          class="w-full rounded border border-border bg-background p-2 font-mono text-xs" />
        <label class="block text-xs" for="helper-account">{$_('sendLater.helperAccount')}</label>
        <input id="helper-account" bind:value={helperAccountId} oninput={() => configured = false}
          class="w-full rounded border border-border bg-background p-2 text-sm" />
        <label class="flex items-start gap-2 text-xs">
          <input type="checkbox" bind:checked={keyVerified} class="mt-0.5" />
          {$_('sendLater.keyVerified')}
        </label>
        <button type="button" onclick={save} disabled={busy || !keyAvailable || !keyVerified || !helperKey || !helperAccountId}
          class="rounded-md border border-border px-3 py-2 text-sm disabled:opacity-50">{$_('sendLater.saveConnection')}</button>
      </div>
    </details>

    {#if jobs.length && !setupOnly}
      <div class="space-y-2">
        <h3 class="text-sm font-medium">{$_('sendLater.jobs')}</h3>
        <p class="text-xs text-muted-foreground">{$_('sendLater.lastConfirmed')}</p>
        <ul class="max-h-52 space-y-2 overflow-y-auto">
          {#each jobs as job (job.id)}
            <li class="rounded-md border border-border p-2 text-xs">
              <div class="flex justify-between gap-2">
                <span>{new Date(job.dueAt).toLocaleString()}</span>
                <span class="font-mono text-muted-foreground">{job.id.slice(0, 8)}</span>
              </div>
              <p class="my-1">{$_(`sendLater.status.${job.state}`)}</p>
              {#if job.code}<p class="text-muted-foreground">{$_(`sendLater.errors.${job.code}`)}</p>{/if}
              <div class="flex gap-3">
                <button type="button" onclick={() => update(job, 'refresh')} disabled={busy} class="underline disabled:opacity-50">{$_('sendLater.refresh')}</button>
                {#if ['awaiting_ack', 'cancellation_awaiting_ack', 'reschedule_awaiting_ack'].includes(job.state)}
                  <button type="button" onclick={() => update(job, 'retry')} disabled={busy} class="underline disabled:opacity-50">{$_('sendLater.retry')}</button>
                {/if}
                {#if ['queued', 'retry', 'awaiting_ack', 'reschedule_awaiting_ack'].includes(job.state)}
                  <button type="button" onclick={() => update(job, 'cancel')} disabled={busy} class="underline disabled:opacity-50">{$_('sendLater.cancelDelivery')}</button>
                {/if}
              </div>
            </li>
          {/each}
        </ul>
      </div>
    {/if}

    <AlertDialog.Footer>
      <AlertDialog.Cancel disabled={sending}>{$_('common.close')}</AlertDialog.Cancel>
      {#if !setupOnly}<button type="button" onclick={schedule} disabled={busy || sending || !configured || !canSchedule || !due}
        class="rounded-md bg-primary px-4 py-2 text-sm text-primary-foreground disabled:opacity-50">{$_('sendLater.schedule')}</button>{/if}
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
