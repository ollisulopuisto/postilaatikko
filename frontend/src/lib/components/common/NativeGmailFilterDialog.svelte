<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import * as Dialog from '$lib/components/ui/dialog'
  import { Button } from '$lib/components/ui/button'
  import { _ } from '$lib/i18n'
  import { app } from '../../../../wailsjs/go/models'
  import { GetNativeGmailWorkspace, NativeGmailReview, AuthorizeNativeGmailWorkspace } from '../../../../wailsjs/go/app/App'

  let { open = $bindable(true), ruleId, onChanged }: {open?: boolean; ruleId: string; onChanged?: () => void} = $props()
  let context = $state<app.NativeGmailContext | null>(null)
  let plan = $state<app.NativeGmailPublicationStatus | null>(null)
  let preview = $state<app.NativeGmailPreview | null>(null)
  let busy = $state(false)
  let error = $state('')
  let query = $state('')
  let markRead = $state(false)
  let archive = $state(false)
  let trash = $state(false)
  let labels = $state<string[]>([])
  let workspace = ''
  let owner = $state<app.NativeGmailWorkspace | null>(null)
  let alive = true
  onDestroy(() => { alive = false })
  const draftKey = $derived(JSON.stringify([query, markRead, archive, trash, labels]))
  $effect(() => { void draftKey; preview = null })

  function draft() { return app.NativeGmailDraft.createFrom({ruleId, query, markRead, archive, trash, labelIds: [...labels]}) }
  async function run(action: () => Promise<void>) {
    if (busy) return
    busy = true
    error = ''
    try { await action() } catch (err) { if (alive) error = String(err) }
    finally { if (alive) busy = false }
  }
  async function request(operation: string, values: Partial<app.NativeGmailReviewRequest> = {}) {
    if (!workspace) throw new Error('Gmail workspace unavailable; reopen review')
    return NativeGmailReview(app.NativeGmailReviewRequest.createFrom({workspace, operation, ruleId, accountId: plan?.accountId || context?.accountId || '', ...values}))
  }
  async function load() {
    if (!workspace) { const selected = await GetNativeGmailWorkspace(); workspace = selected.key; if (alive) owner = selected }
    const status = (await request('status')).publication
    if (!alive) return
    plan = status?.id ? status : null
    const next = (await request('context')).context
    if (alive) context = next || null
  }
  onMount(() => { void run(load) })
  async function consent() {
    if (!context) return
    await AuthorizeNativeGmailWorkspace(workspace, context.accountId)
    if (alive) await load()
  }
  async function checkPreview() {
    const next = (await request('preview', {draft: draft()})).preview
    if (alive) preview = next || null
  }
  async function review() {
    if (!preview) return
    const next = (await request('prepare', {draft: draft(), fingerprint: preview.fingerprint})).publication
    if (alive) plan = next || null
  }
  async function recover() {
    if (!plan) return
    const next = (await request('reconcile', {id: plan.id})).publication
    if (alive) { plan = next || null; onChanged?.() }
  }
  async function publish() {
    if (!plan || plan.state !== 'prepared') return
    try {
      const next = (await request('publish', {id: plan.id, fingerprint: plan.fingerprint})).publication
      if (alive) { plan = next || null; onChanged?.() }
    } catch (err) {
      // The write may have happened. Reload the durable state before offering
      // any further action, never turn a create error into another create.
      try { const status = (await request('status')).publication; if (alive) plan = status || null } catch { if (alive && plan) plan = {...plan, state: 'unknown'} }
      throw err
    }
  }
  async function cancelReview() {
    if (!plan || plan.state !== 'prepared') return
    await request('cancel', {id: plan.id})
    if (alive) { plan = null; preview = null; onChanged?.() }
  }
  function labelName(id: string) {
    const system: Record<string, string> = {UNREAD: $_('gmailFilters.unread'), INBOX: $_('gmailFilters.inbox'), TRASH: $_('gmailFilters.trash')}
    return context?.labels.find(label => label.id === id)?.name || system[id] || id
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="relative sm:max-w-[640px] max-h-[94vh] overflow-y-auto">
    <Dialog.Header class="pr-6">
      <Dialog.Title>{$_('gmailFilters.title')}</Dialog.Title>
      <Dialog.Description>{$_('gmailFilters.description')}</Dialog.Description>
    </Dialog.Header>
    {#if error}<p role="alert" class="native-gmail-error text-sm break-words">{error}</p>{/if}
    {#if busy}<p role="status" class="text-sm text-muted-foreground">{$_('gmailFilters.working')}</p>{/if}
    {#if owner}<p class="text-xs text-muted-foreground break-all">{owner.target === 'helper' ? `${$_('filters.targetHelper')}: ${owner.endpoint}` : $_('filters.targetLocal')}</p>{/if}
    {#if context}<p class="text-sm break-all">{plan?.accountEmail || preview?.accountEmail || context.email}</p>{/if}
    {#if plan?.id}
      <section class="flex flex-col gap-3 rounded-md border border-border p-3">
        <h3 class="font-medium">{$_('gmailFilters.finalReview')}</h3>
        <p class="text-sm whitespace-pre-wrap break-words">{plan.query}</p>
        <p class="text-sm break-words">{$_('gmailFilters.addLabels')}: {(plan.addLabelIds || []).map(labelName).join(', ') || '−'}</p>
        <p class="text-sm break-words">{$_('gmailFilters.removeLabels')}: {(plan.removeLabelIds || []).map(labelName).join(', ') || '−'}</p>
        <p class="text-xs text-muted-foreground">{$_('gmailFilters.nativeSemantics')}</p>
        <p class="text-xs text-muted-foreground">{$_('gmailFilters.sourceStopped')}</p>
        <p class="text-xs text-muted-foreground">{$_('gmailFilters.reserved')}</p>
        {#if plan.state === 'published'}
          <p role="status" class="text-sm">{$_('gmailFilters.published')} {plan.providerId}</p>
        {:else if plan.state !== 'prepared'}
          <p role="status" class="text-sm">{$_('gmailFilters.uncertain')}</p>
        {:else}
          <p class="text-sm">{$_('gmailFilters.confirmFuture')}</p>
        {/if}
      </section>
      <div class="flex flex-wrap justify-end gap-2">
        {#if plan.state === 'prepared'}
          <Button disabled={busy} variant="outline" onclick={() => void run(cancelReview)}>{$_('gmailFilters.cancelReview')}</Button>
          <Button disabled={busy} onclick={() => void run(publish)}>{$_('gmailFilters.publish')}</Button>
        {:else if plan.state !== 'published'}
          <Button disabled={busy} onclick={() => void run(recover)}>{$_('gmailFilters.checkResult')}</Button>
        {/if}
        <Button variant="outline" disabled={busy} onclick={() => { open = false }}>{$_('common.close')}</Button>
      </div>
    {:else if context?.consentRequired}
      <p class="text-sm">{$_('gmailFilters.consent')}</p>
      <Button disabled={busy} onclick={() => void run(consent)}>{$_('gmailFilters.authorize')}</Button>
    {:else if context}
      <p class="rounded-md border border-border p-3 text-sm">{$_('gmailFilters.nativeSemantics')} {$_('gmailFilters.sourceStopped')}</p>
      <fieldset disabled={busy} class="flex flex-col gap-3">
        <label class="flex flex-col gap-1 text-sm">{$_('gmailFilters.query')}<textarea class="min-h-20 w-full rounded-md border border-border bg-background p-2" bind:value={query} maxlength="2048"></textarea></label>
        <div class="flex flex-wrap gap-3 text-sm">
          <label class="flex items-center gap-2"><input type="checkbox" bind:checked={markRead} />{$_('filters.actionMarkRead')}</label>
          <label class="flex items-center gap-2"><input type="checkbox" bind:checked={archive} />{$_('filters.actionArchive')}</label>
          <label class="flex items-center gap-2"><input type="checkbox" bind:checked={trash} />{$_('filters.actionTrash')}</label>
        </div>
        {#if context.labels.length}
          <div class="max-h-36 overflow-y-auto rounded-md border border-border p-2" aria-label={$_('gmailFilters.addLabels')}>
            {#each context.labels as label (label.id)}<label class="flex items-center gap-2 py-1 text-sm"><input type="checkbox" bind:group={labels} value={label.id} />{label.name}</label>{/each}
          </div>
        {/if}
      </fieldset>
      <p class="text-xs text-muted-foreground">{$_('gmailFilters.previewCaveat')}</p>
      {#if preview}<p role="status" class="text-sm">{preview.found}{preview.truncated ? '+' : ''} {$_('gmailFilters.matchesFound')}</p>{/if}
      <div class="flex flex-wrap justify-end gap-2">
        <Button variant="outline" disabled={busy || !query.trim()} onclick={() => void run(checkPreview)}>{$_('gmailFilters.preview')}</Button>
        <Button disabled={busy || !preview} onclick={() => void run(review)}>{$_('gmailFilters.review')}</Button>
      </div>
    {:else if !busy}
      <Button variant="outline" onclick={() => void run(load)}>{$_('common.retry')}</Button>
    {/if}
  </Dialog.Content>
</Dialog.Root>

<style>
  .native-gmail-error { color: #b91c1c; }
  :global([data-theme="dark"]) .native-gmail-error { color: #fca5a5; }
</style>
