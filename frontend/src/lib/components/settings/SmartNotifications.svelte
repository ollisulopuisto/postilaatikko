<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from '@iconify/svelte'
  import Switch from '$lib/components/ui/switch/Switch.svelte'
  import { _ } from '$lib/i18n'
  import { addToast } from '$lib/stores/toast'
  import { GetSmartNotificationSettings, SaveSmartNotificationSettings } from '../../../../wailsjs/go/app/App.js'

  type Rule = { id: string; scope: 'sender' | 'domain'; match: string; action: 'notify' | 'mute' }
  type Preferences = { enabled: boolean; rules: Rule[] }

  let preferences = $state<Preferences>({ enabled: true, rules: [] })
  let loaded = $state(false)
  let saving = $state(false)
  let scope = $state<'sender' | 'domain'>('sender')
  let match = $state('')
  let action = $state<'notify' | 'mute'>('mute')

  onMount(async () => {
    try {
      const saved = await GetSmartNotificationSettings()
      preferences = { enabled: saved.enabled, rules: saved.rules ?? [] }
    } catch (error) {
      console.error('Failed to load smart notification settings:', error)
      addToast({ type: 'error', message: $_('settingsGeneral.notificationsLoadError') })
    } finally {
      loaded = true
    }
  })

  async function save(next: Preferences): Promise<boolean> {
    const previous = preferences
    preferences = next
    saving = true
    try {
      await SaveSmartNotificationSettings(next)
      return true
    } catch (error) {
      console.error('Failed to save smart notification settings:', error)
      preferences = previous
      addToast({ type: 'error', message: $_('settingsGeneral.notificationsSaveError') })
      return false
    } finally {
      saving = false
    }
  }

  function toggleEnabled(enabled: boolean) {
    void save({ ...preferences, enabled })
  }

  async function addRule(event: SubmitEvent) {
    event.preventDefault()
    const value = match.trim()
    if (!value || saving) return
    const saved = await save({
      ...preferences,
      rules: [...preferences.rules, { id: crypto.randomUUID(), scope, match: value, action }],
    })
    if (saved) match = ''
  }

  function updateRule(id: string, changes: Partial<Rule>) {
    void save({ ...preferences, rules: preferences.rules.map(rule => rule.id === id ? { ...rule, ...changes } : rule) })
  }

  function removeRule(id: string) {
    void save({ ...preferences, rules: preferences.rules.filter(rule => rule.id !== id) })
  }
</script>

<section class="mt-7 border-t border-border pt-5" aria-labelledby="smart-notifications-title">
  <div class="mb-4 flex items-start gap-3">
    <span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary"><Icon icon="mdi:bell-outline" class="h-5 w-5" /></span>
    <div class="min-w-0 flex-1">
      <h3 id="smart-notifications-title" class="text-sm font-semibold text-foreground">{$_('settingsGeneral.smartNotifications')}</h3>
      <p class="mt-1 text-xs text-muted-foreground">{$_('settingsGeneral.smartNotificationsHelp')}</p>
    </div>
    <Switch checked={preferences.enabled} onCheckedChange={toggleEnabled} disabled={!loaded || saving} aria-label={$_('settingsGeneral.smartNotifications')} />
  </div>

  {#if preferences.enabled}
    <div class="space-y-2">
      {#each preferences.rules as rule (rule.id)}
        <div class="flex flex-wrap items-center gap-2 rounded-lg border border-border p-2">
          <select class="min-w-24 rounded-md border border-input bg-background px-2 py-1.5 text-sm" value={rule.action} onchange={(event) => updateRule(rule.id, { action: (event.currentTarget as HTMLSelectElement).value as Rule['action'] })} disabled={saving} aria-label={$_('settingsGeneral.notificationAction')}>
            <option value="mute">{$_('settingsGeneral.notificationMute')}</option>
            <option value="notify">{$_('settingsGeneral.notificationAllow')}</option>
          </select>
          <span class="text-xs text-muted-foreground">{$_('settingsGeneral.notificationFrom')}</span>
          <select class="rounded-md border border-input bg-background px-2 py-1.5 text-sm" value={rule.scope} onchange={(event) => updateRule(rule.id, { scope: (event.currentTarget as HTMLSelectElement).value as Rule['scope'] })} disabled={saving} aria-label={$_('settingsGeneral.notificationMatchType')}>
            <option value="sender">{$_('settingsGeneral.notificationSender')}</option>
            <option value="domain">{$_('settingsGeneral.notificationDomain')}</option>
          </select>
          <span class="min-w-0 flex-1 truncate text-sm" title={rule.match}>{rule.match}</span>
          <button type="button" class="rounded-md p-1 text-muted-foreground hover:bg-muted hover:text-foreground" onclick={() => removeRule(rule.id)} disabled={saving} aria-label={$_('settingsGeneral.notificationRemoveRule')}>
            <Icon icon="mdi:close" class="h-4 w-4" />
          </button>
        </div>
      {/each}
    </div>

    <form class="mt-3 flex flex-wrap gap-2" onsubmit={addRule}>
      <select class="rounded-md border border-input bg-background px-2 py-1.5 text-sm" bind:value={action} aria-label={$_('settingsGeneral.notificationAction')} disabled={!loaded || saving}>
        <option value="mute">{$_('settingsGeneral.notificationMute')}</option>
        <option value="notify">{$_('settingsGeneral.notificationAllow')}</option>
      </select>
      <select class="rounded-md border border-input bg-background px-2 py-1.5 text-sm" bind:value={scope} aria-label={$_('settingsGeneral.notificationMatchType')} disabled={!loaded || saving}>
        <option value="sender">{$_('settingsGeneral.notificationSender')}</option>
        <option value="domain">{$_('settingsGeneral.notificationDomain')}</option>
      </select>
      <input class="min-w-40 flex-1 rounded-md border border-input bg-background px-3 py-1.5 text-sm" bind:value={match} aria-label={$_('settingsGeneral.notificationMatchValue')} placeholder={scope === 'sender' ? $_('settingsGeneral.notificationEmailPlaceholder') : $_('settingsGeneral.notificationDomainPlaceholder')} disabled={!loaded || saving} />
      <button type="submit" class="inline-flex items-center gap-1 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground disabled:opacity-50" disabled={!loaded || saving || !match.trim()}>
        <Icon icon="mdi:plus" class="h-4 w-4" />{$_('settingsGeneral.notificationAddRule')}
      </button>
    </form>
    <p class="mt-2 text-xs text-muted-foreground">{$_('settingsGeneral.notificationRuleHelp')}</p>
  {/if}
</section>
