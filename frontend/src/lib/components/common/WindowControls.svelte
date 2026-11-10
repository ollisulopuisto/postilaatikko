<script lang="ts">
  import { _ } from '$lib/i18n'
  import { WindowMinimise, WindowToggleMaximise, Quit } from '../../../../wailsjs/runtime/runtime'

  interface Props {
    onClose?: () => void
    class?: string
  }

  let { onClose, class: extraClass = '' }: Props = $props()

  let isMaximized = $state(false)
  let isHovering = $state(false)

  async function minimize() {
    await WindowMinimise()
  }

  async function toggleMaximize() {
    await WindowToggleMaximise()
    isMaximized = !isMaximized
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
    onclick={toggleMaximize}
    title={isMaximized ? $_('window.restore') : $_('window.maximize')}
    aria-label={isMaximized ? $_('aria.restoreWindow') : $_('aria.maximizeWindow')}
  >
    {#if isHovering}
      <span class="text-[10px] font-bold text-black/60 leading-none">+</span>
    {/if}
  </button>
</div>
