// Shared zoom state for the frameless main window, so every control that
// zooms it (the green button, a double-click on the drag strip) keeps the
// button's label in step.
import { WindowIsMaximised, WindowToggleMaximise } from '../../wailsjs/runtime/runtime'

export const windowState = $state({ maximised: false })

/**
 * WindowToggleMaximise returns before the native transition finishes, so a
 * single read can still see the old state. Read until the state differs from
 * `before` or the attempts run out, and return the last confirmed value.
 */
export async function settleMaximised(
  read: () => Promise<boolean>,
  before: boolean,
  wait: (ms: number) => Promise<void> = ms => new Promise(resolve => setTimeout(resolve, ms)),
  attempts = 10,
  intervalMs = 100,
): Promise<boolean> {
  let current = before
  for (let i = 0; i < attempts; i++) {
    current = await read()
    if (current !== before) return current
    await wait(intervalMs)
  }
  return current
}

export async function toggleMaximise(): Promise<void> {
  const before = windowState.maximised
  await WindowToggleMaximise()
  try {
    windowState.maximised = await settleMaximised(WindowIsMaximised, before)
  } catch {
    windowState.maximised = !before
  }
}

/** Re-read the native state, e.g. after a resize the app did not start itself. */
export async function syncMaximised(): Promise<void> {
  try {
    windowState.maximised = await WindowIsMaximised()
  } catch {
    // keep the last known state
  }
}
