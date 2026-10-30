<script lang="ts">
  import { _ } from '$lib/i18n'
  let { onInsert, compact = false }: { onInsert: (value: string) => void; compact?: boolean } = $props()
  const operators = [
    { label: 'search.opUnread', value: 'is:unread' },
    { label: 'search.opRead', value: 'is:read' },
    { label: 'search.opStarred', value: 'is:starred' },
    { label: 'search.opFrom', value: 'from:' },
    { label: 'search.opTo', value: 'to:' },
    { label: 'search.opAttachment', value: 'has:attachment' },
    { label: 'search.opFolder', value: 'label:' },
    { label: 'search.opExclude', value: '-' },
    { label: 'search.opBefore', value: 'before:' },
    { label: 'search.opAfter', value: 'after:' },
  ]
</script>

<div
  role="group"
  aria-label={$_('search.operators')}
  class={compact ? 'flex min-w-0 flex-wrap items-center gap-1.5 px-3 py-2' : 'flex flex-wrap gap-2'}
>
  {#each operators as operator (operator.value)}
    <button
      type="button"
      class="shrink-0 rounded-full border border-border px-2.5 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
      title={operator.value}
      onclick={() => onInsert(operator.value)}
    >
      {$_(operator.label)}
    </button>
  {/each}
</div>
