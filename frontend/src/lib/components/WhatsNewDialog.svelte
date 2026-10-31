<script lang="ts">
  // Release notes dialog. After an upgrade it shows the changelog sections
  // newer than the last acknowledged version (current version only on the
  // first run); from Help > What's New it shows the full released history.
  // The acknowledged version is recorded only on the explicit OK click of the
  // after-upgrade dialog (see App.svelte); ESC/outside-click leaves it
  // unrecorded so the dialog fires again next launch.
  import { Dialog as DialogPrimitive } from 'bits-ui'
  import { cn } from '$lib/utils'
  import { Button } from '$lib/components/ui/button'
  // @ts-ignore - wailsjs path
  import { OpenURL } from '../../../wailsjs/go/app/App.js'
  import { _ } from '$lib/i18n'
  import { parseChangelog, sectionsSince, type ChangelogSection } from '$lib/utils/whatsNew'
  import changelog from '../../../../CHANGELOG.md?raw'

  interface Props {
    open: boolean
    version: string
    lastSeen: string
    history: boolean
    onAcknowledge: () => void
    onClose: () => void
  }

  let { open = $bindable(false), version, lastSeen, history, onAcknowledge, onClose }: Props = $props()

  const allSections = $derived(parseChangelog(changelog))
  const shownSections: ChangelogSection[] = $derived(
    history ? allSections : sectionsSince(allSections, version, lastSeen)
  )

  const CHANGELOG_URL = 'https://github.com/ollisulopuisto/postilaatikko/blob/main/CHANGELOG.md'

  // Open external links via the backend OpenURL: on Linux it tries the OpenURI
  // portal first, so links work inside the Flatpak sandbox (where xdg-open —
  // and thus Wails' BrowserOpenURL — can't reach the host browser).
  function openExternal(url: string) {
    OpenURL(url).catch((err: unknown) => console.error('Failed to open URL:', err))
  }
</script>

<DialogPrimitive.Root bind:open>
  <DialogPrimitive.Portal>
    <DialogPrimitive.Overlay
      class="fixed inset-0 z-50 bg-black/80 data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
    />
    <DialogPrimitive.Content
      class={cn(
        'fixed left-[50%] top-[50%] z-[51] grid w-full max-w-lg translate-x-[-50%] translate-y-[-50%] gap-6 border bg-background p-8 shadow-lg duration-200 data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[state=closed]:slide-out-to-left-1/2 data-[state=closed]:slide-out-to-top-[48%] data-[state=open]:slide-in-from-left-1/2 data-[state=open]:slide-in-from-top-[48%] sm:rounded-lg'
      )}
    >
      <div class="flex flex-col space-y-1.5 text-center sm:text-left">
        <h2 class="text-lg font-semibold leading-none tracking-tight">
          {history ? $_('whatsNew.historyTitle') : $_('whatsNew.title')}
        </h2>
      </div>

      <div class="space-y-4 max-h-[60vh] overflow-y-auto text-sm">
        {#if !history}
          <p>{$_('whatsNew.welcome', { values: { version } })}</p>
        {/if}

        {#if shownSections.length > 0}
          {#each shownSections as section (section.version)}
            <section>
              {#if history || shownSections.length > 1}
                <h3 class="font-semibold mb-1">v{section.version}</h3>
              {/if}
              <ul class="list-disc space-y-2 pl-5">
                {#each section.items as note (note)}
                  <li>{note}</li>
                {/each}
              </ul>
            </section>
          {/each}
        {:else}
          <p>{$_('whatsNew.fallback')}</p>
        {/if}

      </div>

      <p>
        <button
          type="button"
          class="text-primary hover:underline break-all focus:outline-none focus-visible:outline-none focus:ring-0"
          onclick={() => openExternal(CHANGELOG_URL)}
        >
          {$_('whatsNew.fullChangelog')}
        </button>
      </p>

      <div class="flex flex-col-reverse sm:flex-row sm:justify-end sm:space-x-2">
        <Button onclick={history ? onClose : onAcknowledge}>{$_('common.ok')}</Button>
      </div>
    </DialogPrimitive.Content>
  </DialogPrimitive.Portal>
</DialogPrimitive.Root>
