// Command palette store. Manages visibility and command dispatch for the
// Alfred-style quick launcher.
import { dialogGuardOpen, dialogGuardClose } from '$lib/stores/dialogGuard'
import { getCommand, type CommandId } from '$lib/keyboard/registry'

type CommandHandler = () => void | Promise<void>
interface CommandRegistration { run: CommandHandler; isAvailable?: () => boolean }

const RECENT_KEY = 'postilaatikko.palette.recent'
const RECENT_MAX = 8

/** The last commands run from the palette, newest first. */
export function getRecentCommands(): string[] {
  try {
    const raw = JSON.parse(globalThis.localStorage?.getItem(RECENT_KEY) ?? '[]')
    return Array.isArray(raw) ? raw.filter((id): id is string => typeof id === 'string').slice(0, RECENT_MAX) : []
  } catch {
    return []
  }
}

export function recordRecentCommand(id: string) {
  try {
    const next = [id, ...getRecentCommands().filter(entry => entry !== id)].slice(0, RECENT_MAX)
    globalThis.localStorage?.setItem(RECENT_KEY, JSON.stringify(next))
  } catch {
    // Storage can be unavailable; recents are a convenience only.
  }
}

const registrations = new Map<string, CommandRegistration>()

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

export function registerCommand(id: CommandId, run: CommandHandler, isAvailable?: () => boolean) {
  if (!getCommand(id) && !id.startsWith('extension.')) {
    throw new Error(`Command '${id}' is missing from the command registry`)
  }
  registrations.set(id, { run, isAvailable })
}

export function unregisterCommand(id: CommandId) {
  registrations.delete(id)
}

export function isCommandAvailable(id: string): boolean {
  return registrations.get(id)?.isAvailable?.() ?? true
}

export function hasCommandHandler(id: CommandId): boolean {
  return registrations.has(id)
}

export async function executeCommand(id: CommandId): Promise<boolean> {
  const registration = registrations.get(id)
  if (!registration || registration.isAvailable?.() === false) return false
  if (!getCommand(id)?.keepOpen) closeCommandPalette()
  recordRecentCommand(id)
  try {
    await registration.run()
    return true
  } catch (err) {
    console.error(`[commandPalette] Error executing command '${id}':`, err)
    return false
  }
}
