<script lang="ts">
  import { onDestroy } from 'svelte'
  import * as Dialog from '$lib/components/ui/dialog'
  import { SearchMail, CancelMailSearch, ReauthorizeAccount } from '../../../../wailsjs/go/app/App'
  import { accountStore } from '$lib/stores/accounts.svelte'
  import { app, type message } from '../../../../wailsjs/go/models'
  import { _ } from '$lib/i18n'
  let { open = $bindable(false), onSelect }: { open: boolean; onSelect: (thread: string, folder: string, account: string) => void } = $props()
  let query = $state(''), accountId = $state(''), cursor = $state(''), busy = $state(false)
  let results = $state<message.Conversation[]>([]), issues = $state<app.MailSearchIssue[]>([]), error = $state('')
  let input = $state<HTMLInputElement | null>(null)
  const session = crypto.randomUUID()
  let generation = 0
  let searched = $state(false)
  $effect(() => {
    if (!open) { generation++; void CancelMailSearch(session); busy = false; return }
    const timer = setTimeout(() => input?.focus(), 0)
    return () => clearTimeout(timer)
  })
  onDestroy(() => { void CancelMailSearch(session) })
  async function search(more = false) {
    const current = ++generation
    busy = true; error = ''; searched = true
    if (!more) { results = []; cursor = ''; issues = [] }
    try {
      const page = await SearchMail(new app.MailSearchRequest({ sessionId: session, query: query.trim(), accountIds: accountId ? [accountId] : [], virtualFolderId: '', cursor: more ? cursor : '', limit: 30 }))
      if (current !== generation || !open) return
      const merged = more ? [...results, ...(page.conversations || [])] : page.conversations || []
      results = [...new Map(merged.map(c => [`${c.accountId}:${c.threadId}`, c])).values()].sort((a, b) => new Date(b.latestDate as unknown as string).getTime() - new Date(a.latestDate as unknown as string).getTime())
      cursor = page.cursor || ''; issues = page.issues || []
    } catch (e) { if (current === generation) error = String(e) }
    finally { if (current === generation) busy = false }
  }
</script>
<Dialog.Root bind:open>
  <Dialog.Content class="max-w-3xl max-h-[85vh] overflow-y-auto">
    <Dialog.Header><Dialog.Title>{$_('search.global')}</Dialog.Title></Dialog.Header>
    <form class="flex gap-2" onsubmit={(event) => { event.preventDefault(); void search() }}>
      <input bind:this={input} bind:value={query} oninput={() => { cursor = '' }} type="search" class="flex-1 min-w-0 rounded border border-border bg-background px-3 py-2" placeholder="is:unread label:work" aria-label={$_('search.global')} />
      <select bind:value={accountId} class="max-w-40 rounded border border-border bg-background" aria-label={$_('search.accountScope')} onchange={() => void search()}>
        <option value="">{$_('search.allAccounts')}</option>
        {#each accountStore.accounts as item (item.account.id)}<option value={item.account.id}>{item.account.name || item.account.email}</option>{/each}
      </select>
      <button type="submit" class="rounded bg-primary text-primary-foreground px-3">{$_('search.searchButton')}</button>
    </form>
    <p class="text-xs text-muted-foreground">{$_('search.serverHint')}</p>
    {#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
    {#each issues as issue (issue.accountId + issue.message)}
      <p class="text-xs text-destructive" role="status">{issue.accountName}: {issue.message}
        {#if issue.reauthorize}<button type="button" class="underline" onclick={() => ReauthorizeAccount(issue.accountId).then(() => search())}>{$_('search.reauthorize')}</button>{/if}
      </p>
    {/each}
    <div class="space-y-1" aria-busy={busy}>
      {#each results as item (`${item.accountId}:${item.threadId}`)}
        <button type="button" class="w-full text-left rounded-lg border border-border p-3 hover:bg-muted" onclick={() => { onSelect(item.threadId, item.folderId || '', item.accountId || ''); open = false }}>
          <span class="block text-sm font-medium">{item.subject || $_('messageList.noSubject')}</span>
          <span class="block text-xs text-muted-foreground">{item.accountName} · {item.participants?.map(p => p.name || p.email).join(', ')} · {new Date(item.latestDate as unknown as string).toLocaleString()}</span>
          <span class="block truncate text-sm text-muted-foreground">{item.snippet}</span>
        </button>
      {/each}
    </div>
    {#if searched && !busy && !error && results.length === 0}<p role="status" class="text-sm text-muted-foreground">{$_('search.noMatches')}</p>{/if}
    {#if busy}<p role="status" class="text-sm text-muted-foreground">{$_('common.loading')}</p>{/if}
    {#if cursor}<button type="button" disabled={busy} onclick={() => void search(true)} class="rounded border border-border px-3 py-2">{$_('search.loadMore')}</button>{/if}
  </Dialog.Content>
</Dialog.Root>
