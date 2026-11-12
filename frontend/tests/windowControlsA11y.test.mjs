import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const windowControls = await readFile(new URL('../src/lib/components/common/WindowControls.svelte', import.meta.url), 'utf8')

test('WindowControls container has role="group" and aria-label', () => {
  const containerStart = windowControls.indexOf('<div')
  const containerEnd = windowControls.indexOf('<button')
  const openingTag = windowControls.slice(containerStart, containerEnd)

  assert.match(openingTag, /role="group"/, 'container has role="group"')
  assert.match(openingTag, /aria-label=\{\$_?\('aria\.windowControls'\)\}/, 'container has aria.windowControls aria-label')
  assert.match(openingTag, /onfocusin=/, 'container tracks focus-in')
  assert.match(openingTag, /onfocusout=/, 'container tracks focus-out')
})

test('WindowControls buttons have high-contrast focus rings and outline reset', () => {
  const buttons = windowControls.match(/<button[\s\S]*?<\/button>/g)
  assert.ok(buttons && buttons.length === 3, 'Three traffic light buttons exist')

  for (const button of buttons) {
    assert.match(button, /focus-visible:outline-none/, 'button clears default focus outline')
    assert.match(button, /focus-visible:ring-2/, 'button has focus-visible:ring-2')
    assert.match(button, /focus-visible:ring-ring/, 'button uses ring-ring token')
    assert.match(button, /focus-visible:ring-offset-1/, 'button has ring-offset-1 for visibility')
  }
})

test('WindowControls reveals action glyphs on focus as well as hover', () => {
  assert.match(windowControls, /let isFocused = \$state\(false\)/, 'declares isFocused state')
  const glyphConditions = windowControls.match(/\{#if isHovering \|\| isFocused\}/g)
  assert.ok(glyphConditions && glyphConditions.length === 3, 'all 3 traffic lights reveal glyphs on hover or focus')
})
