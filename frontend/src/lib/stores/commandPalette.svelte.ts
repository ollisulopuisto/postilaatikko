// Command palette store. Manages visibility and command dispatch for the
// Alfred-style quick launcher.
import { dialogGuardOpen, dialogGuardClose } from '$lib/stores/dialogGuard'

type CommandHandler = () => void | Promise<void>

const handlers = new Map<string, CommandHandler>()

let isOpen = $state(false)

export const commandPalette = {
  get open() {
    return isOpen
  },
  set open(value: boolean) {
    if (value === isOpen) return
    isOpen = value
    if (value) {
      dialogGuardOpen()
    } else {
      dialogGuardClose()
    }
  },
}

export function openCommandPalette() {
  commandPalette.open = true
}

export function closeCommandPalette() {
  commandPalette.open = false
}

export function toggleCommandPalette() {
  commandPalette.open = !commandPalette.open
}

export function registerCommandHandler(id: string, handler: CommandHandler) {
  handlers.set(id, handler)
}

export function unregisterCommandHandler(id: string) {
  handlers.delete(id)
}

export function hasCommandHandler(id: string): boolean {
  return handlers.has(id)
}

export async function executeCommand(id: string): Promise<boolean> {
  const handler = handlers.get(id)
  if (!handler) return false
  closeCommandPalette()
  try {
    await handler()
    return true
  } catch (err) {
    console.error(`[commandPalette] Error executing command '${id}':`, err)
    return false
  }
}
