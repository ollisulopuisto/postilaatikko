import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
const read = p => readFile(new URL(p, import.meta.url), 'utf8')
const toast = await read('../src/lib/components/common/PendingSendToast.svelte')
const app = await read('../src/App.svelte')
test('quiet toast has no numeric countdown and its edge progress is silent', () => {
  assert.doesNotMatch(toast, /countdown|seconds|role="progressbar"|setInterval|setTimeout/)
  assert.match(toast, /aria-hidden="true"/)
  assert.match(toast, /h-0\.5 bg-muted/)
  assert.match(toast, /motion-reduce:transition-none/)
  assert.match(toast, /role="status" aria-live="polite" aria-atomic="true"/)
  assert.doesNotMatch(app, /pendingRemaining|pendingSend\.countdown/)
})
test('Undo and Send now keep the existing dispatch/cancel paths and lock during dispatch', () => {
  assert.match(toast, /onclick=\{onSendNow\}/)
  assert.match(toast, /onclick=\{onUndo\}/)
  assert.equal((toast.match(/disabled=\{dispatching\}/g) || []).length, 2)
  assert.match(app, /onSendNow=\{\(\) => void pendingDispatch\(pendingSends\[0\]\)\}/)
  assert.match(app, /onUndo=\{\(\) => void pendingCancel\(pendingSends\[0\]\)\}/)
  assert.match(app, /await CancelPendingSend\(ps.id\)/)
  assert.match(app, /if \(ps.draftId\) await handleEditDraft\(ps.draftId\)/)
})
test('queued notification no longer instructs the user to click a countdown', async () => {
  const en = JSON.parse(await read('../src/lib/i18n/locales/en.json'))
  const fi = JSON.parse(await read('../src/lib/i18n/locales/fi.json'))
  for (const locale of [en, fi]) {
    assert.doesNotMatch(locale.composer.sendQueued, /30|countdown|laskuri/)
    assert.ok(locale.pendingSend.queued)
    assert.ok(locale.pendingSend.sending)
    assert.ok(locale.pendingSend.undo)
  }
})
test('remote pending dispatch uses durable queue ID and leaves helper-owned cleanup alone', () => {
  assert.match(app, /remoteHandled = await DispatchRemotePendingSend\(ps.id\)/)
  assert.match(app, /if \(!remoteHandled\) \{/)
  assert.match(app, /if \(ps.draftId && !remoteHandled\) \{/)
  assert.match(app, /EventsOn\('pending-send:warning'/)
})
