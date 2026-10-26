<script lang="ts">
  import Icon from '@iconify/svelte'
  import Switch from '$lib/components/ui/switch/Switch.svelte'
  import { _ } from '$lib/i18n'
  import { getShowContactsCalendarRail, setShowContactsCalendarRail } from '$lib/stores/uiState.svelte'
  import { sidebarGroups } from '$lib/stores/sidebarGroups.svelte'

  let groups = $derived(sidebarGroups.list)
  let showContactsCalendar = $state(getShowContactsCalendarRail())

  function handleExtensionRailChange(value: boolean): void {
    showContactsCalendar = value
    setShowContactsCalendarRail(value)
  }
</script>

<div class="space-y-5">
  <section class="space-y-2">
    <div class="flex items-center justify-between gap-4">
      <div>
        <p class="text-sm font-medium">{$_('sidebarLayout.extensionRail')}</p>
        <p class="text-xs text-muted-foreground">{$_('sidebarLayout.extensionRailHelp')}</p>
      </div>
      <Switch
        id="show-contacts-calendar-rail"
        checked={showContactsCalendar}
        onCheckedChange={handleExtensionRailChange}
      />
    </div>
  </section>

  <section class="space-y-2">
    <div>
      <p class="text-sm font-medium">{$_('sidebarLayout.groupsTitle')}</p>
      <p class="text-xs text-muted-foreground">{$_('sidebarLayout.groupsHelp')}</p>
    </div>

    <ul class="space-y-1">
      {#each groups as group, index (group.id)}
        {@const label = $_(group.titleKey)}
        <li class="flex items-center gap-2 rounded-md border border-border px-2 py-1.5">
          <input
            id={`sidebar-group-${group.id}`}
            type="checkbox"
            class="h-4 w-4 accent-primary"
            checked={!group.hidden}
            onchange={(event) => sidebarGroups.setHidden(group.id, !event.currentTarget.checked)}
          />
          <label for={`sidebar-group-${group.id}`} class="min-w-0 flex-1 truncate text-sm">{label}</label>
          <button
            type="button"
            class="flex h-7 w-7 shrink-0 items-center justify-center rounded hover:bg-muted disabled:opacity-40"
            disabled={index === 0}
            onclick={() => sidebarGroups.move(group.id, -1)}
            aria-label={`${$_('sidebarLayout.moveUp')}: ${label}`}
            title={$_('sidebarLayout.moveUp')}
          >
            <Icon icon="mdi:chevron-up" class="h-4 w-4" />
          </button>
          <button
            type="button"
            class="flex h-7 w-7 shrink-0 items-center justify-center rounded hover:bg-muted disabled:opacity-40"
            disabled={index === groups.length - 1}
            onclick={() => sidebarGroups.move(group.id, 1)}
            aria-label={`${$_('sidebarLayout.moveDown')}: ${label}`}
            title={$_('sidebarLayout.moveDown')}
          >
            <Icon icon="mdi:chevron-down" class="h-4 w-4" />
          </button>
        </li>
      {/each}
    </ul>
    <button
      type="button"
      class="rounded-md border border-border px-3 py-1.5 text-sm hover:bg-muted"
      onclick={() => sidebarGroups.reset()}
    >
      {$_('sidebarLayout.reset')}
    </button>
  </section>
</div>
