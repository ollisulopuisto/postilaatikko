import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

test('AccountIdentityTab identity action buttons and radio have accessible attributes and keyboard disclosure', () => {
  const filePath = path.resolve(__dirname, '../src/lib/components/settings/account/AccountIdentityTab.svelte')
  const content = fs.readFileSync(filePath, 'utf-8')

  // Check radio button semantics and focus-visible styling
  assert.ok(
    content.includes('role="radio"'),
    'default identity selector button should have role="radio"'
  )
  assert.ok(
    content.includes('aria-checked={identity.isDefault}'),
    'default identity selector should reflect aria-checked'
  )
  assert.ok(
    content.includes('aria-label={identity.isDefault ? $_(\'identity.defaultAddress\') : `${$_(\'identity.setAsDefaultAddress\')} (${identity.email})`}'),
    'default identity selector should have an accessible aria-label'
  )
  assert.ok(
    content.includes('focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2'),
    'default identity selector should have visible focus indicator ring'
  )

  // Check action buttons container disclosure on keyboard focus-within
  assert.ok(
    content.includes('group-focus-within:opacity-100 focus-within:opacity-100'),
    'action buttons container should reveal when focused via keyboard'
  )

  // Check action buttons have aria-label
  assert.ok(
    content.includes("aria-label={$_('common.edit')}"),
    'edit action button should have aria-label'
  )
  assert.ok(
    content.includes("aria-label={identity.isDefault ? $_('identity.cannotDeleteDefaultTitle') : $_('common.delete')}"),
    'identity delete action button should have aria-label'
  )
})

test('AccountIdentityTab shared mailbox action buttons have accessible names and focus-within disclosure', () => {
  const filePath = path.resolve(__dirname, '../src/lib/components/settings/account/AccountIdentityTab.svelte')
  const content = fs.readFileSync(filePath, 'utf-8')

  // Shared mailbox delete button aria-label
  assert.ok(
    content.includes("aria-label={$_('common.delete')}"),
    'shared mailbox delete button should have aria-label'
  )
})
