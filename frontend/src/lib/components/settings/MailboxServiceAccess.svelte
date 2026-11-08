<script lang="ts">
  import { onMount } from 'svelte'
  import { Button } from '$lib/components/ui/button'
  import { _ } from '$lib/i18n'
  // @ts-ignore - Wails binding path
  import {
    EnableMailboxServiceWrites,
    GetMailboxServiceLifecycleStatus,
    GetMailboxServiceStatus,
    InstallMailboxService,
    StartMailboxService,
    StopMailboxService,
  } from '../../../../wailsjs/go/app/App.js'

  interface ServiceStatus {
    connected: boolean
    writeAuthorized: boolean
    errorCode?: string
  }

  interface LifecycleStatus {
    supported: boolean
    helperAvailable: boolean
    installed: boolean
    running: boolean
  }

  type LifecycleAction = 'install' | 'start' | 'stop'

  let status = $state<ServiceStatus | null>(null)
  let lifecycle = $state<LifecycleStatus | null>(null)
  let saving = $state(false)
  let error = $state(false)
  let lifecycleAction = $state<LifecycleAction | null>(null)
  let lifecycleError = $state(false)

  async function refreshStatus() {
    try {
      status = await GetMailboxServiceStatus()
    } catch {
      status = { connected: false, writeAuthorized: false, errorCode: 'status_failed' }
    }
    try {
      lifecycle = await GetMailboxServiceLifecycleStatus()
    } catch {
      lifecycle = { supported: true, helperAvailable: false, installed: false, running: false }
      lifecycleError = true
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

  async function manageService(action: LifecycleAction) {
    if (lifecycleAction) return
    lifecycleAction = action
    lifecycleError = false
    try {
      switch (action) {
        case 'install':
          lifecycle = await InstallMailboxService()
          break
        case 'start':
          lifecycle = await StartMailboxService()
          break
        case 'stop':
          lifecycle = await StopMailboxService()
          break
      }
      await refreshStatus()
    } catch {
      lifecycleError = true
      await refreshStatus()
    } finally {
      lifecycleAction = null
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

  {#if lifecycle?.supported}
    <div class="space-y-2 border-t border-border pt-3">
      {#if !lifecycle.helperAvailable}
        <p class="text-sm text-muted-foreground">{$_('settingsGeneral.mailboxServiceHelperMissing')}</p>
      {:else if lifecycle.running}
        <p class="text-sm text-muted-foreground">{$_('settingsGeneral.mailboxServiceAgentRunning')}</p>
        <Button onclick={() => manageService('stop')} disabled={lifecycleAction !== null}>
          {lifecycleAction === 'stop' ? $_('settingsGeneral.mailboxServiceManaging') : $_('settingsGeneral.mailboxServiceAgentStop')}
        </Button>
      {:else if lifecycle.installed}
        <p class="text-sm text-muted-foreground">{$_('settingsGeneral.mailboxServiceAgentStopped')}</p>
        <Button onclick={() => manageService('start')} disabled={lifecycleAction !== null}>
          {lifecycleAction === 'start' ? $_('settingsGeneral.mailboxServiceManaging') : $_('settingsGeneral.mailboxServiceAgentStart')}
        </Button>
      {:else}
        <p class="text-sm text-muted-foreground">{$_('settingsGeneral.mailboxServiceAgentNotInstalled')}</p>
        <Button onclick={() => manageService('install')} disabled={lifecycleAction !== null}>
          {lifecycleAction === 'install' ? $_('settingsGeneral.mailboxServiceManaging') : $_('settingsGeneral.mailboxServiceAgentInstall')}
        </Button>
      {/if}
    </div>
  {/if}
  {#if lifecycleError && lifecycle?.supported}
    <p class="text-sm text-destructive" role="alert">{$_('settingsGeneral.mailboxServiceLifecycleFailed')}</p>
  {/if}

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
