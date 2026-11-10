import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'

const source = await readFile(new URL('../src/lib/snooze/presets.ts', import.meta.url), 'utf8')
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { snoozePresets, parseCustomSnooze, toDatetimeLocal } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const L = (y, mo, d, h = 0, mi = 0) => new Date(y, mo - 1, d, h, mi)
const ids = list => list.map(p => p.id)
const at = (list, id) => list.find(p => p.id === id)?.wakeAt

// 2026-11-10 is a Tuesday.
test('weekday morning offers every preset at the documented local times', () => {
  const list = snoozePresets(L(2026, 11, 10, 9, 5))
  assert.deepEqual(ids(list), ['laterToday', 'thisEvening', 'tomorrow', 'weekend', 'nextWeek'])
  assert.deepEqual(at(list, 'laterToday'), L(2026, 11, 10, 12, 15))
  assert.deepEqual(at(list, 'thisEvening'), L(2026, 11, 10, 18))
  assert.deepEqual(at(list, 'tomorrow'), L(2026, 11, 11, 8))
  assert.deepEqual(at(list, 'weekend'), L(2026, 11, 14, 8))
  assert.deepEqual(at(list, 'nextWeek'), L(2026, 11, 16, 8))
})

test('late evening drops same-day presets', () => {
  const list = snoozePresets(L(2026, 11, 10, 22, 30))
  assert.deepEqual(ids(list), ['tomorrow', 'weekend', 'nextWeek'])
})

test('afternoon keeps Later today but not an evening less than two hours away', () => {
  const list = snoozePresets(L(2026, 11, 10, 16, 30))
  assert.deepEqual(ids(list), ['laterToday', 'tomorrow', 'weekend', 'nextWeek'])
})

test('Saturday and Sunday roll to the following weekend and Monday', () => {
  const sat = snoozePresets(L(2026, 11, 14, 10))
  assert.deepEqual(at(sat, 'weekend'), L(2026, 11, 21, 8))
  assert.deepEqual(at(sat, 'nextWeek'), L(2026, 11, 16, 8))
  // Sunday: Monday is tomorrow, so Next week must not repeat it.
  const sun = snoozePresets(L(2026, 11, 15, 10))
  assert.deepEqual(ids(sun), ['laterToday', 'thisEvening', 'tomorrow', 'weekend'])
  assert.deepEqual(at(sun, 'weekend'), L(2026, 11, 21, 8))
})

test('Friday: tomorrow is Saturday so Weekend does not repeat it', () => {
  const list = snoozePresets(L(2026, 11, 13, 10))
  assert.deepEqual(ids(list), ['laterToday', 'thisEvening', 'tomorrow', 'nextWeek'])
})

test('month boundary', () => {
  const list = snoozePresets(L(2026, 12, 31, 23, 0))
  assert.deepEqual(at(list, 'tomorrow'), L(2027, 1, 1, 8))
})

test('custom snooze validates and parses local wall-clock values', () => {
  const now = L(2026, 11, 10, 9)
  assert.deepEqual(parseCustomSnooze('2026-11-12T07:30', now), { ok: true, wakeAt: L(2026, 11, 12, 7, 30) })
  assert.equal(parseCustomSnooze('', now).reason, 'invalid')
  assert.equal(parseCustomSnooze('2026-02-31T10:00', now).reason, 'invalid')
  assert.equal(parseCustomSnooze('2026-11-10T09:00', now).reason, 'past')
  assert.equal(parseCustomSnooze('2026-11-10T08:00', now).reason, 'past')
  assert.equal(parseCustomSnooze('2040-01-01T08:00', now).reason, 'toofar')
  assert.equal(toDatetimeLocal(L(2026, 3, 5, 7, 4)), '2026-03-05T07:04')
})

test('snooze is wired through the central registry, palette, menu, selection bar and viewer toolbar', async () => {
  const read = p => readFile(new URL(p, import.meta.url), 'utf8')
  const registry = await read('../src/lib/keyboard/registry.ts')
  assert.match(registry, /id: 'mail.snooze', icon: [^}]*combos: \['h'\][^}]*dispatch: true, menu: 'Message'/)
  assert.match(registry, /\{ id: 'mail.snooze', category: 'commandPalette.categories.mail', shortcutId: 'mail.snooze' \}/)
  const app = await read('../src/App.svelte')
  assert.match(app, /registerCommand\('mail.snooze'/)
  assert.match(app, /'mail.snooze': \(\) => void executeCommand\('mail.snooze'\)/)
  assert.match(app, /EventsOn\('snooze:expired'/)
  const list = await read('../src/lib/components/list/MessageList.svelte')
  assert.match(list, /id: 'mail\.snooze'/)
  assert.match(list, /export function snoozeSelection/)
  assert.match(await read('../src/lib/components/viewer/ConversationViewer.svelte'), /<SnoozeButton /)
  assert.match(await read('../src/lib/components/common/MessageContextMenu.svelte'), /<SnoozeDialog /)
  for (const loc of ['en', 'fi']) {
    const j = JSON.parse(await read(`../src/lib/i18n/locales/${loc}.json`))
    for (const k of ['laterToday', 'thisEvening', 'tomorrow', 'weekend', 'nextWeek']) assert.ok(j.snooze.preset[k], `${loc} preset ${k}`)
    for (const k of ['invalid', 'past', 'toofar']) assert.ok(j.snooze.error[k], `${loc} error ${k}`)
    for (const k of ['doneUntil', 'doneUntilMany', 'returned', 'returnedMany', 'title', 'custom']) assert.ok(j.snooze[k], `${loc} ${k}`)
    assert.ok(j.shortcuts.action.snooze)
  }
})
