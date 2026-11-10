<script lang="ts">
  import Icon from '@iconify/svelte'
  import { DropdownMenu } from 'bits-ui'
  import { _ } from '$lib/i18n'
  import { ariaKeyshortcuts, formatShortcut } from '$lib/keyboard/format'

  interface Action {
    id: string
    label: string
    icon: string
    shortcut?: string
    disabled?: boolean
  }

  interface Props {
    count: number
    actions: Action[]
    onAction: (id: string) => void
    onMove: () => void
    onMore?: () => void
    onClear: () => void
    disabled?: boolean
  }

  let { count, actions, onAction, onMove, onMore, onClear, disabled = false }: Props = $props()
  const primaryActions = $derived(actions.slice(0, 3))
  const overflowActions = $derived(actions.slice(3))
  let activeIndex = $state(0)
  let toolbar: HTMLDivElement | null = $state(null)

  function handleKeydown(event: KeyboardEvent) {
    if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return
    const buttons = toolbar?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')
    if (!buttons?.length) return
    event.preventDefault()
    const current = [...buttons].indexOf(document.activeElement as HTMLButtonElement)
    activeIndex = (current + (event.key === 'ArrowRight' ? 1 : -1) + buttons.length) % buttons.length
    buttons[activeIndex]?.focus()
  }
</script>

<div
  bind:this={toolbar}
  class="flex min-w-0 items-center gap-1 border-b border-border bg-muted/40 px-2 py-1.5 sm:gap-1.5 sm:px-3"
  role="toolbar"
  aria-label={$_('messageList.selectedActions')}
  tabindex="-1"
  onkeydown={handleKeydown}
>
  <span class="mr-auto min-w-0 truncate px-1 text-xs font-medium text-foreground sm:text-sm">
    {$_('messageList.selectedCount', { values: { count } })}
  </span>
  {#each primaryActions as action, index (action.id)}
    <button
      type="button"
      class="inline-flex h-8 shrink-0 items-center justify-center gap-1.5 rounded-md px-2 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:text-sm"
      title={action.shortcut ? `${action.label} (${formatShortcut(action.shortcut)})` : action.label}
      aria-label={action.label}
      aria-keyshortcuts={action.shortcut ? ariaKeyshortcuts(action.shortcut) : undefined}
      tabindex={index === activeIndex ? 0 : -1}
      disabled={disabled || action.disabled}
      onclick={() => action.id === 'mail.moveTo' ? onMove() : onAction(action.id)}
    >
      <Icon icon={action.icon} class="h-4 w-4" />
      <span class="sr-only">{action.label}</span>
    </button>
  {/each}
  <DropdownMenu.Root>
  <DropdownMenu.Trigger
    class="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    title={$_('messageList.selectedActions')}
    aria-label={$_('messageList.selectedActions')}
    tabindex={primaryActions.length === activeIndex ? 0 : -1}
  >
    <Icon icon="mdi:dots-horizontal" class="h-4 w-4" />
  </DropdownMenu.Trigger>
  <DropdownMenu.Portal>
    <DropdownMenu.Content align="end" side="bottom" sideOffset={4} class="z-50 min-w-[200px] rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-md">
      {#each overflowActions as action (action.id)}
        <DropdownMenu.Item
          onSelect={() => action.id === 'mail.moveTo' ? onMove() : onAction(action.id)}
          class="flex cursor-default select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none focus:bg-accent focus:text-accent-foreground"
        >
          <Icon icon={action.icon} class="mr-2 h-4 w-4" />
          <span>{action.label}</span>
          {#if action.shortcut}<span class="ml-auto pl-4 text-xs text-muted-foreground">{formatShortcut(action.shortcut)}</span>{/if}
        </DropdownMenu.Item>
      {/each}
      {#if onMore}
        {#if overflowActions.length}<DropdownMenu.Separator class="-mx-1 my-1 h-px bg-border" />{/if}
        <DropdownMenu.Item
          onSelect={onMore}
          class="flex cursor-default select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none focus:bg-accent focus:text-accent-foreground"
        >
          <Icon icon="mdi:console-line" class="mr-2 h-4 w-4" />
          {$_('commandPalette.title')}
        </DropdownMenu.Item>
      {/if}
    </DropdownMenu.Content>
  </DropdownMenu.Portal>
  </DropdownMenu.Root>
  <button
    type="button"
    class="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    title={$_('common.close')}
    aria-label={$_('common.close')}
    tabindex={primaryActions.length + 1 === activeIndex ? 0 : -1}
    onclick={onClear}
  >
    <Icon icon="mdi:close" class="h-4 w-4" />
  </button>
</div>
