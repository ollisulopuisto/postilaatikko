<script lang="ts">
  import RailButton from './RailButton.svelte'
  import { getRailTabs } from '$lib/stores/extensionRegistry.svelte'
  import { getActiveExtension, setActiveExtension } from '$lib/stores/uiState.svelte'
  import { navigationCards as nav } from '$lib/stores/navigationCards.svelte'
  import { shortcutIndex } from '$lib/sidebar/cards'
  import { _ } from '$lib/i18n'
  const mac = /Mac|iPhone|iPad/.test(navigator.platform)
  const systemIcons: Record<string, string> = {
    inbox: 'mdi:inbox-outline', starred: 'mdi:pin-outline', drafts: 'mdi:file-outline',
    sent: 'mdi:send-outline', trash: 'mdi:delete-outline', archive: 'mdi:archive-outline',
    replyLater: 'mdi:clock-outline', spam: 'mdi:alert-octagon-outline', snoozed: 'mdi:clock-outline', all: 'mdi:email-multiple-outline',
  }
  function quickIcon(entry: { kind: string; folderType?: string }): string {
    if (entry.kind === 'inboxes') return 'mdi:inbox-multiple-outline'
    if (entry.kind === 'virtual') return 'mdi:folder-multiple-outline'
    if (entry.kind === 'system') return systemIcons[entry.folderType || ''] || 'mdi:folder-outline'
    return 'mdi:folder-outline'
  }
  function keydown(event: KeyboardEvent) {
    if (event.defaultPrevented || document.querySelector('[role="dialog"], [role="alertdialog"]')) return
    const target = event.target as HTMLElement | null
    if (target?.closest('input, textarea, select, [contenteditable="true"]')) return
    const index = shortcutIndex(event, mac), entry = nav.quick[index]
    if (entry) { event.preventDefault(); event.stopPropagation(); nav.navigate(entry) }
  }
</script>
<svelte:window onkeydown={keydown} />
<nav class="flex flex-col items-stretch w-12 flex-shrink-0 bg-muted/30 border-r border-border pt-2" aria-label={$_('navigation.quickAccess')}>
  <RailButton icon="mdi:email" label="Mail" active={getActiveExtension() === 'mail'} onclick={() => setActiveExtension('mail')} />
  {#each nav.quick as entry, index (entry.id)}
    <RailButton icon={quickIcon(entry)} label={`${entry.name} (${mac ? '⌘' : 'Ctrl+'}${(index + 1) % 10})`} active={false} onclick={() => nav.navigate(entry)} />
  {/each}
  {#if nav.state.showApps}
    {#each getRailTabs() as tab (tab.extensionId)}
      <RailButton icon={tab.icon || 'mdi:puzzle'} label={tab.label} active={getActiveExtension() === tab.extensionId} onclick={() => setActiveExtension(tab.extensionId)} />
    {/each}
  {/if}
</nav>
