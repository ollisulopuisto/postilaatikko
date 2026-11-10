<script lang="ts">
  import { _ } from '$lib/i18n'
  import { WindowMinimise, Quit } from '../../../../wailsjs/runtime/runtime'
  import { windowState, toggleMaximise } from '$lib/windowState.svelte'

  interface Props {
    onClose?: () => void
    class?: string
  }

  let { onClose, class: extraClass = '' }: Props = $props()

  let isHovering = $state(false)

  async function minimize() {
    await WindowMinimise()
  }

  function close() {
    if (onClose) {
      onClose()
    } else {
      // Windows without a custom close handler (e.g. the composer) just quit.
      Quit()
    }
  }
</script>

<!-- Mac-style traffic lights in the standard order: close, minimise, zoom.
     The group opts out of the window drag region its parent bar provides. -->
<div
  class="flex items-center gap-2 {extraClass}"
  style="--wails-draggable: no-drag"
  data-window-controls
  onmouseenter={() => isHovering = true}
  onmouseleave={() => isHovering = false}
>
  <button
    class="w-3 h-3 rounded-full flex items-center justify-center transition-all bg-[#FF5F57] hover:brightness-90 active:brightness-75"
    onclick={close}
    title={$_('window.close')}
    aria-label={$_('aria.closeWindow')}
  >
    {#if isHovering}
      <span class="text-[10px] font-bold text-black/60 leading-none">×</span>
    {/if}
  </button>
  <button
    class="w-3 h-3 rounded-full flex items-center justify-center transition-all bg-[#FEBC2E] hover:brightness-90 active:brightness-75"
    onclick={minimize}
    title={$_('window.minimize')}
    aria-label={$_('aria.minimizeWindow')}
  >
    {#if isHovering}
      <span class="text-[10px] font-bold text-black/60 leading-none">−</span>
    {/if}
  </button>
  <button
    class="w-3 h-3 rounded-full flex items-center justify-center transition-all bg-[#28C840] hover:brightness-90 active:brightness-75"
    onclick={() => void toggleMaximise()}
    title={windowState.maximised ? $_('window.restore') : $_('window.maximize')}
    aria-label={windowState.maximised ? $_('aria.restoreWindow') : $_('aria.maximizeWindow')}
  >
    {#if isHovering}
      <span class="text-[10px] font-bold text-black/60 leading-none">+</span>
    {/if}
  </button>
</div>
