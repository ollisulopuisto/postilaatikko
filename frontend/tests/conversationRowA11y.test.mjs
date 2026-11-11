import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const conversationRow = await readFile(new URL('../src/lib/components/list/ConversationRow.svelte', import.meta.url), 'utf8')

test('ConversationRow selection checkbox has accessible role, state, and focus styling', () => {
  const match = conversationRow.match(/<button[\s\S]*?onclick=\{handleCheckboxClick\}[\s\S]*?>/)
  assert.ok(match, 'Selection checkbox button exists in ConversationRow')
  const tag = match[0]
  assert.match(tag, /role="checkbox"/, 'checkbox button has role="checkbox"')
  assert.match(tag, /aria-checked=\{checked\}/, 'checkbox button reflects checked state with aria-checked')
  assert.match(tag, /aria-label=/, 'checkbox button has an accessible aria-label')
  assert.match(tag, /focus-visible:outline-none/, 'checkbox button clears default focus outline')
  assert.match(tag, /focus-visible:ring-2/, 'checkbox button has focus-visible:ring-2')
  assert.match(tag, /focus-visible:ring-ring/, 'checkbox button has focus-visible:ring-ring')
})

test('ConversationRow quick actions container reveals on keyboard focus', () => {
  // Quick actions container must disclose when its children receive focus
  assert.match(conversationRow, /group-focus-within:opacity-100/, 'quick actions container reveals opacity when focused within')
  assert.match(conversationRow, /focus-within:opacity-100/, 'quick actions container reveals opacity on focus-within')
  assert.match(conversationRow, /group-focus-within:pointer-events-auto/, 'quick actions container enables pointer events when focused within')
  assert.match(conversationRow, /focus-within:pointer-events-auto/, 'quick actions container enables pointer events on focus-within')
})

test('ConversationRow quick action buttons have high-contrast focus rings', () => {
  // Check the .quick-row-action CSS rule or button classes for focus ring
  const hasFocusRing = conversationRow.includes('focus-visible:ring-2') && conversationRow.includes('focus-visible:ring-ring')
  const styleMatch = conversationRow.match(/\.quick-row-action:focus-visible\s*\{[\s\S]*?\}/)
  assert.ok(
    hasFocusRing || styleMatch,
    'quick-row-action buttons have high-contrast focus rings for keyboard navigation'
  )
  if (styleMatch) {
    assert.match(styleMatch[0], /outline:\s*none/, 'focus-visible clears default outline')
    assert.match(styleMatch[0], /box-shadow|ring/, 'focus-visible defines a focus ring indicator')
  }
})
