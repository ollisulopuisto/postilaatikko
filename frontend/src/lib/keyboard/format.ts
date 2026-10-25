// Platform-aware rendering of registry combos for the shortcuts overlay,
// tooltips, menu hints and aria-keyshortcuts. Reads ./registry.ts only; it
// never decides what a key does.

import { getShortcut, type ShortcutDef } from './registry'

/** True on macOS (WKWebView reports "Macintosh" in the user agent). */
export function isMacPlatform(): boolean {
  if (typeof navigator === 'undefined') return false
  const platform = (navigator as Navigator & { userAgentData?: { platform?: string } }).userAgentData?.platform
    ?? navigator.platform
    ?? ''
  return /mac/i.test(platform) || /Macintosh|Mac OS X/.test(navigator.userAgent ?? '')
}

const MAC_MODIFIERS: Record<string, string> = {
  Mod: '⌘',
  Meta: '⌘',
  Ctrl: '⌃',
  Alt: '⌥',
  Shift: '⇧',
}

const OTHER_MODIFIERS: Record<string, string> = {
  Mod: 'Ctrl',
  Meta: 'Meta',
  Ctrl: 'Ctrl',
  Alt: 'Alt',
  Shift: 'Shift',
}

const KEY_NAMES: Record<string, { mac: string; other: string }> = {
  ArrowUp: { mac: '↑', other: '↑' },
  ArrowDown: { mac: '↓', other: '↓' },
  ArrowLeft: { mac: '←', other: '←' },
  ArrowRight: { mac: '→', other: '→' },
  Enter: { mac: '↩', other: 'Enter' },
  Space: { mac: 'Space', other: 'Space' },
  Escape: { mac: 'Esc', other: 'Esc' },
  Tab: { mac: '⇥', other: 'Tab' },
  Backspace: { mac: '⌫', other: 'Backspace' },
  Delete: { mac: '⌦', other: 'Delete' },
  ContextMenu: { mac: 'Menu', other: 'Menu' },
  RightAlt: { mac: 'Right ⌥', other: 'Right Alt' },
  LeftAlt: { mac: 'Left ⌥', other: 'Left Alt' },
}

/** ARIA key names (https://www.w3.org/TR/wai-aria-1.2/#aria-keyshortcuts). */
const ARIA_KEY_NAMES: Record<string, string> = {
  Space: 'Space',
  Escape: 'Escape',
  RightAlt: 'Alt',
  LeftAlt: 'Alt',
}

/** Split a combo like 'Mod+Shift+r' into modifiers and key. The key may itself be '+'-free punctuation. */
export function parseCombo(combo: string): { mods: string[]; key: string } {
  const parts = combo.split('+')
  const key = parts.pop() ?? ''
  return { mods: parts, key }
}

/** Display caps for one combo, e.g. ['Ctrl', 'Shift', 'R'] or ['⌘', '⇧', 'R']. */
export function comboCaps(combo: string, mac = isMacPlatform()): string[] {
  const { mods, key } = parseCombo(combo)
  const modNames = mac ? MAC_MODIFIERS : OTHER_MODIFIERS
  const caps = mods.map(m => modNames[m] ?? m)
  const named = KEY_NAMES[key]
  if (named) {
    caps.push(mac ? named.mac : named.other)
    return caps
  }
  // Single-key shortcuts are shown lowercase (as typed, Gmail-style) so `g`
  // and `Shift+G` are not confused; chords use the conventional capital.
  caps.push(mods.length === 0 ? key : key.toUpperCase())
  return caps
}

/** One combo as text: 'Ctrl+Shift+R' elsewhere, '⌘⇧R' on macOS. */
export function formatCombo(combo: string, mac = isMacPlatform()): string {
  return comboCaps(combo, mac).join(mac ? '' : '+')
}

const SPOKEN_MAC: Record<string, string> = { '⌘': 'Command', '⌃': 'Control', '⌥': 'Option', '⇧': 'Shift', '↩': 'Return', '⇥': 'Tab', '⌫': 'Delete', '⌦': 'Forward Delete' }
const SPOKEN_ARROWS: Record<string, string> = { '↑': 'Up', '↓': 'Down', '←': 'Left', '→': 'Right' }

/** Screen-reader text for a combo: 'Command+Shift+R' rather than '⌘⇧R'. */
export function spokenCombo(combo: string, mac = isMacPlatform()): string {
  return comboCaps(combo, mac)
    .map(cap => SPOKEN_ARROWS[cap] ?? (mac ? SPOKEN_MAC[cap] ?? cap.replace('⌥', 'Option') : cap))
    .join('+')
}

/** True when the shortcut exists on this platform. */
export function isAvailable(def: ShortcutDef, mac = isMacPlatform()): boolean {
  return !def.macOnly || mac
}

/** Primary combo of a registry entry as text, or '' if unknown/unavailable. */
export function formatShortcut(id: string, mac = isMacPlatform()): string {
  const def = getShortcut(id)
  if (!def || !isAvailable(def, mac)) return ''
  return formatCombo(def.combos[0], mac)
}

/**
 * Append the shortcut to a visible label for tooltips: "Reply (Ctrl+R)".
 * Returns the label unchanged when the id has no binding on this platform.
 */
export function withShortcut(label: string, id: string, mac = isMacPlatform()): string {
  const keys = formatShortcut(id, mac)
  return keys ? `${label} (${keys})` : label
}

/** Value for the aria-keyshortcuts attribute, or undefined to omit it. */
export function ariaKeyshortcuts(id: string, mac = isMacPlatform()): string | undefined {
  const def = getShortcut(id)
  if (!def || !isAvailable(def, mac)) return undefined
  const value = def.combos.map(combo => {
    const { mods, key } = parseCombo(combo)
    const names = mods.map(m => {
      if (m === 'Mod') return mac ? 'Meta' : 'Control'
      if (m === 'Ctrl') return 'Control'
      return m
    })
    names.push(ARIA_KEY_NAMES[key] ?? (key.length === 1 && mods.includes('Shift') ? key.toUpperCase() : key))
    return names.join('+')
  }).join(' ')
  return value || undefined
}
