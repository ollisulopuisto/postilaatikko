<script lang="ts">
  // Always-visible switch between the two ways to read a thread. A pair of
  // pressed-state buttons rather than a hidden menu: the way back to the whole
  // message must never take a search.
  import { _ } from '$lib/i18n'
  import { READER_MODES, type ReaderMode } from './readerMode'

  interface Props {
    mode: ReaderMode
    onChange: (mode: ReaderMode) => void
  }

  let { mode, onChange }: Props = $props()
</script>

<div
  class="inline-flex shrink-0 flex-wrap items-center rounded-md border border-border p-0.5 text-xs"
  role="group"
  aria-label={$_('viewer.readerMode')}
>
  {#each READER_MODES as option (option)}
    <button
      type="button"
      class="reader-mode-option whitespace-nowrap rounded px-2 py-1 transition-colors {mode === option ? 'bg-muted text-foreground font-medium' : 'text-muted-foreground hover:text-foreground'}"
      aria-pressed={mode === option}
      data-reader-mode={option}
      onclick={() => {
        if (mode !== option) onChange(option)
      }}
    >
      {option === 'conversation' ? $_('viewer.readerConversation') : $_('viewer.readerTraditional')}
    </button>
  {/each}
</div>

<style>
  .reader-mode-option { position: relative; min-width: max-content; flex: 0 0 auto; width: auto !important; padding: 0.25rem 0.5rem !important; }
</style>
