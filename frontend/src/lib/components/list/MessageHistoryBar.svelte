<script lang="ts">
  import Icon from '@iconify/svelte'
  import { _ } from '../../i18n'
  import { withShortcut, ariaKeyshortcuts } from '../../keyboard/format'
  let { canBack, canForward, onBack, onForward }: { canBack: boolean; canForward: boolean; onBack: () => void; onForward: () => void } = $props()
</script>

<nav class="flex shrink-0 items-center gap-0.5" aria-label={$_('messageHistory.label')}>
  <button type="button" class="flex size-8 items-center justify-center rounded-md text-muted-foreground hover:bg-muted disabled:opacity-30 disabled:cursor-default focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" disabled={!canBack} onclick={onBack} title={withShortcut($_('messageHistory.back'), 'nav.historyBack')} aria-keyshortcuts={ariaKeyshortcuts('nav.historyBack')} aria-label={$_('messageHistory.back')}><Icon icon="mdi:arrow-left" width="18" /></button>
  <button type="button" class="flex size-8 items-center justify-center rounded-md text-muted-foreground hover:bg-muted disabled:opacity-30 disabled:cursor-default focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" disabled={!canForward} onclick={onForward} title={withShortcut($_('messageHistory.forward'), 'nav.historyForward')} aria-keyshortcuts={ariaKeyshortcuts('nav.historyForward')} aria-label={$_('messageHistory.forward')}><Icon icon="mdi:arrow-right" width="18" /></button>
</nav>
