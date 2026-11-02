<script lang="ts">
  import { GetAccounts, GetFolders, PreviewSuggestedMoveFilter, CreateSuggestedMoveFilter, DismissMoveFilterSuggestion } from '../../../../wailsjs/go/app/App.js'
  import { _ } from '$lib/i18n'

  type Suggestion = { key: string; accountId: string; sourceFolderId: string; destFolderId: string; destAccountId?: string; requiresGmailFilter?: boolean; fromContains: string[]; toContains: string[]; subjectContains: string[] }
  type Header = { id: string; subject: string; fromName: string; fromEmail: string; date: Date | string }
  let { suggestion, onClose }: { suggestion: Suggestion; onClose: () => void } = $props()
  let fromEnabled = $state(suggestion.fromContains.length > 0)
  let toEnabled = $state(suggestion.toContains.length > 0)
  let subjectEnabled = $state(suggestion.subjectContains.length > 0)
  let fromValue = $state(suggestion.fromContains.join(', '))
  let toValue = $state(suggestion.toContains.join(', '))
  let subjectValue = $state(suggestion.subjectContains.join(', '))
  let ruleName = $state('')
  let accountName = $state(suggestion.accountId)
  let destinationName = $state(suggestion.destFolderId)
  let preview = $state<Header[]>([])
  let error = $state('')
  let busy = $state(false)

  $effect(() => {
    void (async () => {
      try {
        const [accounts, folders] = await Promise.all([GetAccounts(), GetFolders(suggestion.destAccountId || suggestion.accountId)])
        accountName = accounts.find((account: any) => account.id === suggestion.accountId)?.name || suggestion.accountId
        destinationName = folders.find((folder: any) => folder.id === suggestion.destFolderId)?.name || suggestion.destFolderId
        if (!ruleName) ruleName = `Move to ${destinationName}`
      } catch { /* IDs remain visible if labels cannot be loaded */ }
    })()
  })

  function values(enabled: boolean, value: string): string[] { return enabled ? value.split(',').map(part => part.trim()).filter(Boolean) : [] }
  function rule() { return { id: '', name: ruleName, enabled: true, orderIndex: 0, accountId: suggestion.accountId, folderIds: [suggestion.sourceFolderId], fromContains: values(fromEnabled, fromValue), toContains: values(toEnabled, toValue), subjectContains: values(subjectEnabled, subjectValue), destFolderId: suggestion.destFolderId, createdAt: undefined, updatedAt: undefined } }
  async function refreshPreview() {
    busy = true; error = ''
    try { preview = await PreviewSuggestedMoveFilter(suggestion, rule()) as Header[] }
    catch (reason) { error = String(reason) }
    finally { busy = false }
  }
  async function create() {
    busy = true; error = ''
    try { await CreateSuggestedMoveFilter(suggestion, rule()); onClose() }
    catch (reason) { error = String(reason) }
    finally { busy = false }
  }
  async function dismiss() {
    busy = true
    try { await DismissMoveFilterSuggestion(suggestion.key); onClose() }
    catch (reason) { error = String(reason) }
    finally { busy = false }
  }
</script>

<div class="backdrop" role="presentation">
  <section class="dialog" role="dialog" aria-modal="true" aria-labelledby="filter-title">
    <h2 id="filter-title">{$_('suggestedFilter.title')}</h2>
    <p>{$_('suggestedFilter.description')} <strong>{accountName}</strong> → <strong>{destinationName}</strong>. {$_('suggestedFilter.andHint')}</p>
    <label class="name">{$_('suggestedFilter.name')} <input bind:value={ruleName} /></label>
    <fieldset><legend><input type="checkbox" bind:checked={fromEnabled} /> {$_('suggestedFilter.sender')}</legend><input aria-label={$_('suggestedFilter.senderValues')} bind:value={fromValue} disabled={!fromEnabled} /></fieldset>
    <fieldset><legend><input type="checkbox" bind:checked={toEnabled} /> {$_('suggestedFilter.recipient')}</legend><input aria-label={$_('suggestedFilter.recipientValues')} bind:value={toValue} disabled={!toEnabled} /></fieldset>
    <fieldset><legend><input type="checkbox" bind:checked={subjectEnabled} /> {$_('suggestedFilter.subject')}</legend><input aria-label={$_('suggestedFilter.subjectValues')} bind:value={subjectValue} disabled={!subjectEnabled} /></fieldset>
    <div class="actions"><button onclick={refreshPreview} disabled={busy}>{$_('suggestedFilter.preview')}</button><span>{preview.length} {$_('suggestedFilter.matches')}</span></div>
    {#if preview.length}<ul class="preview">{#each preview as item (item.id)}<li><strong>{item.subject || '(no subject)'}</strong><small>{item.fromName || item.fromEmail} · {item.date}</small></li>{/each}</ul>{/if}
    {#if error}<p class="error" role="alert">{error}</p>{/if}
    {#if suggestion.requiresGmailFilter}<p role="status">{$_('suggestedFilter.gmailDeferred')}</p>{/if}
    <footer><button onclick={dismiss} disabled={busy}>{$_('suggestedFilter.dismiss')}</button><button class="primary" onclick={create} disabled={busy || suggestion.requiresGmailFilter || !ruleName.trim() || !(fromEnabled || toEnabled || subjectEnabled)}>{$_('suggestedFilter.create')}</button></footer>
  </section>
</div>

<style>
  .backdrop{position:fixed;inset:0;z-index:10000;background:#0009;display:grid;place-items:center;padding:1rem}.dialog{width:min(34rem,100%);max-height:90vh;overflow:auto;border:1px solid var(--border-color,#444);border-radius:12px;background:var(--surface-1,#222);color:var(--text-primary,#eee);padding:1.25rem;box-shadow:0 20px 70px #0008}h2{margin:0 0 .5rem}.dialog p{color:var(--text-secondary,#aaa);line-height:1.45}.name,fieldset{display:grid;gap:.5rem;margin:1rem 0}.name input,fieldset>input{width:100%;box-sizing:border-box;background:var(--surface-2,#181818);color:inherit;border:1px solid var(--border-color,#555);border-radius:6px;padding:.65rem}fieldset{border:1px solid var(--border-color,#444);border-radius:8px}legend{display:flex;align-items:center;gap:.45rem}.actions,footer{display:flex;align-items:center;justify-content:space-between;gap:.75rem;margin-top:1rem}button{border:1px solid var(--border-color,#555);border-radius:6px;background:var(--surface-2,#333);color:inherit;padding:.55rem .85rem;cursor:pointer}.primary{background:#1476c6}.preview{max-height:12rem;overflow:auto;padding-left:1.3rem}.preview li{margin:.5rem 0}.preview small{display:block;color:var(--text-secondary,#aaa)}.error{color:#ff8c8c!important}
</style>
