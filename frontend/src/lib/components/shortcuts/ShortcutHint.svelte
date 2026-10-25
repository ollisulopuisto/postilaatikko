<script lang="ts">
  // Right-aligned shortcut hint for menu items, e.g. "Ctrl+K" next to
  // Archive. Text comes from the shortcut registry; renders nothing when the
  // id has no binding on this platform. Hidden from screen readers: the key
  // is announced through the shortcuts overlay instead of every menu item.
  import { formatShortcut } from '$lib/keyboard/format'

  interface Props {
    id: string
  }

  const { id }: Props = $props()
  const text = $derived(formatShortcut(id))
</script>

{#if text}
  <kbd class="ml-auto pl-6 font-sans text-xs tracking-wide text-muted-foreground" aria-hidden="true">{text}</kbd>
{/if}
