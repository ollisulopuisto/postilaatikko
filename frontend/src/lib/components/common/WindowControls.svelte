<script lang="ts">
  import { _ } from '$lib/i18n'
  import { WindowMinimise, Quit } from '../../../../wailsjs/runtime/runtime'
  import { windowState, toggleMaximise, syncMaximised } from '$lib/windowState.svelte'

  interface Props {
    onClose?: () => void
    class?: string
  }

  let { onClose, class: extraClass = '' }: Props = $props()

  let isHovering = $state(false)
  let isFocused = $state(false)

  // Resizes the app did not start (OS zoom, snapping) also change the label.
  $effect(() => {
    void syncMaximised()
    const onResize = () => void syncMaximised()
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  })

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
  role="group"
  aria-label={$_('aria.windowControls')}
  class="flex items-center gap-2 {extraClass}"
  style="--wails-draggable: no-drag"
  data-window-controls
  onmouseenter={() => isHovering = true}
  onmouseleave={() => isHovering = false}
  onfocusin={() => isFocused = true}
  onfocusout={(e) => {
    if (!e.currentTarget.contains(e.relatedTarget as Node | null)) {
      isFocused = false
    }
  }}
>
  <button
    class="w-3 h-3 rounded-full flex items-center justify-center transition-all bg-[#FF5F57] hover:brightness-90 active:brightness-75 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1"
    onclick={close}
    title={$_('window.close')}
    aria-label={$_('aria.closeWindow')}
  >
    {#if isHovering || isFocused}
      <span class="text-[10px] font-bold text-black/60 leading-none">×</span>
    {/if}
  </button>
  <button
    class="w-3 h-3 rounded-full flex items-center justify-center transition-all bg-[#FEBC2E] hover:brightness-90 active:brightness-75 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1"
    onclick={minimize}
    title={$_('window.minimize')}
    aria-label={$_('aria.minimizeWindow')}
  >
    {#if isHovering || isFocused}
      <span class="text-[10px] font-bold text-black/60 leading-none">−</span>
    {/if}
  </button>
  <button
    class="w-3 h-3 rounded-full flex items-center justify-center transition-all bg-[#28C840] hover:brightness-90 active:brightness-75 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1"
    onclick={() => void toggleMaximise()}
    title={windowState.maximised ? $_('window.restore') : $_('window.maximize')}
    aria-label={windowState.maximised ? $_('aria.restoreWindow') : $_('aria.maximizeWindow')}
  >
    {#if isHovering || isFocused}
      <span class="text-[10px] font-bold text-black/60 leading-none">+</span>
    {/if}
  </button>
</div>
