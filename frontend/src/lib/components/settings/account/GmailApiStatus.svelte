<script lang="ts">
  // Read-only status of the Gmail API for one account (P-035). Shows what the
  // last Gmail call found; it does not contact Google itself.
  import { onMount } from 'svelte'
  import Icon from '@iconify/svelte'
  import { _ } from '$lib/i18n'
  import { gmailApiStatusView } from '$lib/gmail/apiStatus'
  // @ts-ignore - wailsjs path
  import { GetGmailAPIStatus } from '../../../../../wailsjs/go/app/App'

  interface Props {
    accountId: string
  }

  let { accountId }: Props = $props()

  let status = $state<{ applicable: boolean; state: string; since?: string } | undefined>(undefined)

  const view = $derived(gmailApiStatusView(status))

  onMount(async () => {
    try {
      status = await GetGmailAPIStatus(accountId)
    } catch (err) {
      console.error('Failed to load Gmail API status:', err)
    }
  })

  function formatSince(value: string): string {
    const date = new Date(value)
    return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
  }
</script>

{#if view}
  <div class="rounded-lg border border-border p-3 text-xs" role="status">
    <div class="flex items-center gap-2 font-medium">
      {#if view.tone === 'good'}
        <Icon icon="mdi:check-circle" class="w-4 h-4 text-green-500" />
      {:else if view.tone === 'warning'}
        <Icon icon="mdi:alert-circle-outline" class="w-4 h-4 text-amber-500" />
      {:else}
        <Icon icon="mdi:information-outline" class="w-4 h-4 text-muted-foreground" />
      {/if}
      <span>{$_('account.gmailApi.title')}</span>
    </div>
    <p class="mt-1 text-muted-foreground">{$_(view.key)}</p>
    {#if view.since}
      <p class="mt-1 text-muted-foreground">{$_('account.gmailApi.since', { values: { time: formatSince(view.since) } })}</p>
    {/if}
  </div>
{/if}
