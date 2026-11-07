import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

test('sidebar AccountSection menu trigger button has accessible name, menu semantics, and focus-visible state', () => {
  const filePath = path.resolve(__dirname, '../src/lib/components/sidebar/AccountSection.svelte')
  const content = fs.readFileSync(filePath, 'utf-8')

  // Menu button has title and aria-label
  assert.ok(
    content.includes("aria-label={$_('common.moreActions')}"),
    'account menu button should have aria-label'
  )
  assert.ok(
    content.includes("title={$_('common.moreActions')}"),
    'account menu button should have title'
  )

  // Menu disclosure semantics
  assert.ok(
    content.includes('aria-haspopup="menu"'),
    'account menu button should have aria-haspopup="menu"'
  )
  assert.ok(
    content.includes('aria-expanded={showMenu}'),
    'account menu button should reflect aria-expanded state'
  )

  // Focus-visible ring and opacity on hover/focus-hidden button
  assert.ok(
    content.includes('focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'),
    'account menu button should have focus-visible styling'
  )
})
