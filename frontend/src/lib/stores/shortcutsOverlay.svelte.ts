// Open state of the keyboard shortcuts overlay (ShortcutsDialog). Lives in a
// store so the `?` key in App.svelte, the mail sidebar entry and the
// extension sidebar footers can all open the single dialog mounted in
// App.svelte.

let open = $state(false)

export const shortcutsOverlay = {
  get open(): boolean {
    return open
  },
  set open(value: boolean) {
    open = value
  },
}

export function openShortcutsOverlay(): void {
  open = true
}
