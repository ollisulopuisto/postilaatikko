// Tracks open dialogs that should prevent background reloads from destroying
// the component tree (e.g., folder picker dialog dismissed by sync reload).
let count = 0
const listeners = new Set<(active: boolean) => void>()

function notify(active: boolean) {
  for (const listener of listeners) listener(active)
}

export function dialogGuardOpen() {
  count++
  if (count === 1) notify(true)
}

export function dialogGuardClose() {
  if (count === 0) return
  count--
  if (count === 0) notify(false)
}

export function isDialogGuardActive(): boolean {
  return count > 0
}

/** Subscribe to guard state transitions without polling. */
export function onDialogGuardChange(listener: (active: boolean) => void): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}
