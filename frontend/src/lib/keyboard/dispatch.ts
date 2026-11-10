// Registry-driven key dispatch. The registry (registry.ts) is the only place a
// key is bound: this module turns a KeyboardEvent into the registry entry whose
// combo matches, and the host (App.svelte) runs the handler registered for
// that entry's id. There are no per-key switch cases in the host for the
// commands that go through here.

import { CORE_SHORTCUTS, type ShortcutDef } from './registry'
import { parseCombo } from './format'
import { LAYOUT_SYMBOL } from './shortcuts'

const LAYOUT_SYMBOLS = ['#', '/', '!', '?']

const NAMED: Record<string, string> = { Space: ' ' }

function isWordKey(key: string): boolean {
  return key.length > 1 || /[a-z0-9]/i.test(key)
}

/** True when the event is exactly this combo. Mod is Ctrl or Cmd. */
export function matchCombo(e: KeyboardEvent, combo: string): boolean {
  const { mods, key } = parseCombo(combo)
  // A bare layout symbol (# / ! ?) is matched by the character the layout produced, so
  // Option+3 on a Finnish Mac or AltGr combinations still work. Ordinary Ctrl+Alt chords
  // stay out (LAYOUT_SYMBOL), and a combo with explicit modifiers is matched exactly below.
  if (mods.length === 0 && LAYOUT_SYMBOLS.includes(key)) return e.key === key && LAYOUT_SYMBOL(e)
  const wantMod = mods.includes('Mod')
  const wantCtrl = mods.includes('Ctrl')
  const wantMeta = mods.includes('Meta')
  const wantAlt = mods.includes('Alt')
  const wantShift = mods.includes('Shift')
  if (wantMod) {
    if (!(e.ctrlKey || e.metaKey)) return false
  } else {
    if (e.ctrlKey !== wantCtrl || e.metaKey !== wantMeta) return false
  }
  if (e.altKey !== wantAlt) return false
  // Shift is implied by punctuation such as ! # ? and, on some layouts, /.
  if (isWordKey(key) && e.shiftKey !== wantShift) return false
  const expected = NAMED[key] ?? key
  if (expected.length !== 1 || !/[a-z]/i.test(expected)) return e.key === expected
  // Letters: a bare binding matches the produced case exactly, so Caps Lock (uppercase
  // without shiftKey) never fires a lowercase mail key, and Shift+Caps (lowercase) never
  // fires a Shift binding. Modifier chords keep the old handler's case-insensitive match.
  if (wantMod || wantCtrl || wantMeta || wantAlt) return e.key.toLowerCase() === expected.toLowerCase()
  return wantShift ? e.key === expected.toUpperCase() : e.key === expected
}

export type FocusPane = 'sidebar' | 'messageList' | 'viewer'

/**
 * The registry entry for this event, limited to the ids the host handles.
 * Entries in the viewer group apply only when the viewer has focus, unless they
 * are flagged anyFocus (focus mode and image loading work from any pane). When two
 * entries share a combo, the one in the focused pane's group wins.
 */
export function resolveShortcut(e: KeyboardEvent, focus: FocusPane | string, ids: Iterable<string>, defs: ShortcutDef[] = CORE_SHORTCUTS): ShortcutDef | undefined {
  const handled = new Set(ids)
  const hits = defs.filter(d =>
    handled.has(d.id) &&
    (!d.panes || d.panes.includes('mail')) &&
    (d.group !== 'viewer' || d.anyFocus || focus === 'viewer') &&
    d.combos.some(c => !d.legacy?.includes(c) && matchCombo(e, c)))
  return hits.find(d => d.group === focus) ?? hits[0]
}
