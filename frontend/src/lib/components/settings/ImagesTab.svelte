<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from '@iconify/svelte'
  import { Button } from '$lib/components/ui/button'
  import { Label } from '$lib/components/ui/label'
  import Switch from '$lib/components/ui/switch/Switch.svelte'
  import ConfirmDialog from '$lib/components/ui/confirm-dialog/ConfirmDialog.svelte'
  import { addToast } from '$lib/stores/toast'
  import { _ } from '$lib/i18n'
  import { refreshImageAllowlist } from '$lib/stores/imageAllowlist.svelte'
  // @ts-ignore - wailsjs path
  import { settings } from '../../../../wailsjs/go/models'
  // @ts-ignore - wailsjs path
  import {
    GetImageAllowlist,
    RemoveImageAllowlist,
  } from '../../../../wailsjs/go/app/App'

  interface Props {
    alwaysLoadImages: boolean
    onAlwaysLoadImagesChange: (value: boolean) => void
  }

  let {
    alwaysLoadImages = $bindable(),
    onAlwaysLoadImagesChange,
  }: Props = $props()

  // State
  let entries = $state<settings.AllowlistEntry[]>([])
  let loading = $state(true)
  let hasLoaded = $state(false)
  let loadError = $state(false)
  let removeError = $state(false)
  let removingId = $state<number | null>(null)
  let addressesCollapsed = $state(false)
  let domainsCollapsed = $state(false)
  let showAlwaysLoadImagesConfirm = $state(false)

  // Derived
  let addresses = $derived(entries.filter(e => e.type === 'sender'))
  let domains = $derived(entries.filter(e => e.type === 'domain'))

  function handleAlwaysLoadImagesChange(value: boolean) {
    if (value) {
      showAlwaysLoadImagesConfirm = true
      return
    }
    alwaysLoadImages = false
    onAlwaysLoadImagesChange?.(false)
  }

  async function loadData() {
    loading = true
    loadError = false
    try {
      entries = await GetImageAllowlist() ?? []
      hasLoaded = true
      return true
    } catch (err) {
      console.error('Failed to load image allowlist:', err)
      loadError = true
      return false
    } finally {
      loading = false
    }
  }

  async function handleRemove(id: number) {
    if (removingId !== null) return
    removingId = id
    removeError = false
    try {
      await RemoveImageAllowlist(id)
      refreshImageAllowlist()
      if (await loadData()) {
        addToast({ type: 'success', message: $_('images.removed') })
      }
    } catch (err) {
      console.error('Failed to remove allowlist entry:', err)
      removeError = true
    } finally {
      removingId = null
    }
  }

  onMount(() => {
    loadData()
  })
</script>

{#if loadError}
  <div role="alert" class="mb-4 space-y-2 rounded-md border border-destructive p-3 text-sm text-foreground">
    <p>{$_('images.loadFailed')}</p>
    <Button size="sm" variant="outline" onclick={loadData} disabled={loading}>{$_('common.retry')}</Button>
  </div>
{/if}
{#if removeError}
  <div role="alert" class="mb-4 rounded-md border border-destructive p-3 text-sm text-foreground">
    {$_('images.removeFailed')}
  </div>
{/if}

{#if loading}
  <div class="flex items-center justify-center py-8">
    <Icon icon="mdi:loading" class="w-6 h-6 animate-spin text-muted-foreground" />
  </div>
{:else}
  <div class="space-y-6">
    <!-- Always Load Remote Images Toggle -->
    <div class="space-y-2">
      <div class="flex items-center justify-between gap-3">
        <div class="space-y-0.5 min-w-0">
          <Label for="always-load-images">{$_('settingsGeneral.alwaysLoadImages')}</Label>
          <p class="text-xs text-muted-foreground">
            {$_('settingsGeneral.alwaysLoadImagesHelp')}
          </p>
        </div>
        <Switch
          id="always-load-images"
          class="shrink-0"
          bind:checked={alwaysLoadImages}
          onCheckedChange={handleAlwaysLoadImagesChange}
        />
      </div>
    </div>

    {#if hasLoaded}
    <!-- Divider -->
    <div class="border-t border-border"></div>

    <!-- Addresses Section -->
    <div class="space-y-3">
      <button
        class="w-full flex items-center gap-2 text-sm font-semibold text-foreground hover:text-primary transition-colors text-left"
        onclick={() => addressesCollapsed = !addressesCollapsed}
      >
        <Icon icon={addressesCollapsed ? 'mdi:chevron-right' : 'mdi:chevron-down'} class="w-4 h-4 flex-shrink-0" />
        <Icon icon="mdi:email-outline" class="w-4 h-4" />
        {$_('images.addresses')}
        <span class="text-[10px] px-1.5 py-0.5 rounded bg-muted text-muted-foreground font-medium">{addresses.length}</span>
      </button>

      {#if !addressesCollapsed}
        {#if addresses.length === 0}
          <p class="text-sm text-muted-foreground ml-6">{$_('images.noAddresses')}</p>
        {:else}
          <div class="space-y-1.5 max-h-48 overflow-y-auto ml-6">
            {#each addresses as entry (entry.id)}
              <div class="flex items-center gap-3 p-2 rounded-md border border-border">
                <Icon icon="mdi:email-outline" class="w-4 h-4 text-muted-foreground flex-shrink-0" />
                <span class="text-sm flex-1 truncate">{entry.value}</span>
                <Button variant="ghost" size="sm" onclick={() => handleRemove(entry.id)} disabled={removingId !== null} title={$_('images.removeButton')}>
                  <Icon icon="mdi:close" class="w-3.5 h-3.5" />
                </Button>
              </div>
            {/each}
          </div>
        {/if}
      {/if}
    </div>

    <!-- Domains Section -->
    <div class="space-y-3">
      <button
        class="w-full flex items-center gap-2 text-sm font-semibold text-foreground hover:text-primary transition-colors text-left"
        onclick={() => domainsCollapsed = !domainsCollapsed}
      >
        <Icon icon={domainsCollapsed ? 'mdi:chevron-right' : 'mdi:chevron-down'} class="w-4 h-4 flex-shrink-0" />
        <Icon icon="mdi:web" class="w-4 h-4" />
        {$_('images.domains')}
        <span class="text-[10px] px-1.5 py-0.5 rounded bg-muted text-muted-foreground font-medium">{domains.length}</span>
      </button>

      {#if !domainsCollapsed}
        {#if domains.length === 0}
          <p class="text-sm text-muted-foreground ml-6">{$_('images.noDomains')}</p>
        {:else}
          <div class="space-y-1.5 max-h-48 overflow-y-auto ml-6">
            {#each domains as entry (entry.id)}
              <div class="flex items-center gap-3 p-2 rounded-md border border-border">
                <Icon icon="mdi:web" class="w-4 h-4 text-muted-foreground flex-shrink-0" />
                <span class="text-sm flex-1 truncate">{entry.value}</span>
                <Button variant="ghost" size="sm" onclick={() => handleRemove(entry.id)} disabled={removingId !== null} title={$_('images.removeButton')}>
                  <Icon icon="mdi:close" class="w-3.5 h-3.5" />
                </Button>
              </div>
            {/each}
          </div>
        {/if}
      {/if}
    </div>
    {/if}
  </div>
{/if}

<ConfirmDialog
  bind:open={showAlwaysLoadImagesConfirm}
  title={$_('settingsGeneral.alwaysLoadImagesWarningTitle')}
  description={$_('settingsGeneral.alwaysLoadImagesWarningDescription')}
  confirmLabel={$_('settingsGeneral.disable')}
  variant="destructive"
  onConfirm={() => { onAlwaysLoadImagesChange?.(true) }}
  onCancel={() => { alwaysLoadImages = false }}
/>
