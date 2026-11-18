import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
const read = p => readFile(new URL(p, import.meta.url), 'utf8')

test('move-history suggestion opens the existing filter dialog and never creates a rule by itself', async () => {
  const app = await read('../src/App.svelte')
  const dialog = await read('../src/lib/components/common/FilterDialog.svelte')
  assert.match(app, /EventsOn\('filters:suggestionAvailable'/)
  assert.match(app, /suggestion=\{filterSuggestion\}/)
  assert.match(app, /MarkFilterSuggestionCreated\(filterSuggestion\.key\)/)
  assert.match(app, /DismissFilterSuggestion\(suggestion\.key\)/)
  assert.doesNotMatch(app.slice(app.indexOf("EventsOn('filters:suggestionAvailable'"), app.indexOf("EventsOn('pending-send:warning'")), /CreateFilterRule/)
  assert.match(dialog, /prefillFromSuggestion/)
  assert.match(dialog, /suggestion\?\.sourceFolderId \? \[suggestion\.sourceFolderId\]/)
})

test('suggestion strings exist in English and Finnish', async () => {
  for (const l of ['en', 'fi']) {
    const j = JSON.parse(await read(`../src/lib/i18n/locales/${l}.json`))
    for (const k of ['suggestionAvailable', 'suggestionReview', 'suggestionDismiss']) assert.ok(j.filters[k], `${l}.${k}`)
  }
})
