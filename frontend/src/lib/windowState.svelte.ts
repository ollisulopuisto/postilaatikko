// Shared zoom state for the frameless main window, so every control that
// zooms it (the green button, a double-click on the drag strip) keeps the
// button's label in step.
import { WindowIsMaximised, WindowToggleMaximise } from '../../wailsjs/runtime/runtime'

export const windowState = $state({ maximised: false })

export async function toggleMaximise(): Promise<void> {
  await WindowToggleMaximise()
  try {
    windowState.maximised = await WindowIsMaximised()
  } catch {
    windowState.maximised = !windowState.maximised
  }
}
