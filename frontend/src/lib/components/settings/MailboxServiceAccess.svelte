<script lang="ts">
  import { onMount } from 'svelte'
  import { Button } from '$lib/components/ui/button'
  import { _ } from '$lib/i18n'
  // @ts-ignore - Wails binding path
  import { EnableMailboxServiceWrites, GetMailboxServiceStatus } from '../../../../wailsjs/go/app/App.js'

  interface ServiceStatus {
    connected: boolean
    writeAuthorized: boolean
    errorCode?: string
  }

  let status = $state<ServiceStatus | null>(null)
  let saving = $state(false)
  let error = $state(false)

  async function refreshStatus() {
    try {
      status = await GetMailboxServiceStatus()
    } catch {
      status = { connected: false, writeAuthorized: false, errorCode: 'status_failed' }
    }
  }

  async function enableWrites() {
    if (saving) return
    saving = true
    error = false
    try {
      status = await EnableMailboxServiceWrites()
    } catch {
      error = true
      await refreshStatus()
    } finally {
      saving = false
    }
  }

  onMount(() => {
    void refreshStatus()
  })
</script>

<section class="space-y-3 rounded-lg border border-border p-4">
  <div class="space-y-1">
    <h3 class="text-sm font-medium">{$_('settingsGeneral.mailboxService')}</h3>
    <p class="text-xs text-muted-foreground">{$_('settingsGeneral.mailboxServiceHelp')}</p>
  </div>

  {#if status?.writeAuthorized}
    <p class="text-sm text-muted-foreground">{$_('settingsGeneral.mailboxServiceWritesEnabled')}</p>
    {#if error}
      <p class="text-sm text-destructive" role="alert">{$_('settingsGeneral.mailboxServiceEnableFailed')}</p>
    {/if}
  {:else if status?.connected}
    <p class="text-sm text-muted-foreground">{$_('settingsGeneral.mailboxServiceReadOnly')}</p>
    {#if error}
      <p class="text-sm text-destructive" role="alert">{$_('settingsGeneral.mailboxServiceEnableFailed')}</p>
    {/if}
    <Button onclick={enableWrites} disabled={saving}>
      {saving ? $_('settingsGeneral.mailboxServiceEnabling') : $_('settingsGeneral.mailboxServiceEnableWrites')}
    </Button>
  {:else}
    <p class="text-sm text-muted-foreground">{$_('settingsGeneral.mailboxServiceUnavailable')}</p>
  {/if}
</section>
