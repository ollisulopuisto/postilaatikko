import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const source = fs.readFileSync(path.resolve(__dirname, '../src/lib/components/settings/account/AccountIdentityTab.svelte'), 'utf-8')

function listBlock(marker) {
  const start = source.indexOf(marker)
  assert.notEqual(start, -1, `${marker} must exist`)
  return source.slice(start, source.indexOf('{/each}', start))
}

function assertActionDisclosure(block, label) {
  const wrapper = block.match(/<div class="([^"]*opacity-0[^"]*)">/)
  assert.ok(wrapper, `${label} must have its own action wrapper`)
  for (const token of ['group-hover:opacity-100', 'group-focus-within:opacity-100', 'focus-within:opacity-100']) {
    assert.ok(wrapper[1].split(/\s+/).includes(token), `${label} wrapper needs ${token}`)
  }
  assert.ok(block.includes("aria-label={$_('common.edit')}"), `${label} Edit needs a name`)
}

test('default identity uses native account-qualified radio grouping and browser keyboard behavior', () => {
  const block = listBlock('{#each identities as identity (identity.id)}')
  const input = block.match(/<input\s[\s\S]*?\/>/)
  assert.ok(input, 'default identity must be a native radio input')
  assert.match(input[0], /type="radio"/)
  assert.match(input[0], /name=\{`default-identity-\$\{accountId\}`\}/)
  assert.match(input[0], /bind:group=\{selectedDefaultId\}/)
  assert.match(input[0], /onchange=\{\(\) => handleSetDefault\(identity\)\}/)
  assert.match(input[0], /aria-label=\{`\$\{identity.email\}/)
  assert.match(input[0], /focus-visible:ring-2/)
  assert.ok(!block.includes('role="radio"'), 'do not fake native radio behavior on buttons')
  assert.ok(!input[0].includes('tabindex='), 'let the browser manage the radio group tab stop')
})

test('identity actions independently disclose on keyboard focus and have names', () => {
  const block = listBlock('{#each identities as identity (identity.id)}')
  assertActionDisclosure(block, 'identity')
  assert.ok(block.includes("aria-label={identity.isDefault ? $_('identity.cannotDeleteDefaultTitle') : $_('common.delete')}"))
})

test('shared-mailbox actions independently disclose on keyboard focus and have names', () => {
  const block = listBlock('{#each sharedMailboxes as mailbox (mailbox.id)}')
  assertActionDisclosure(block, 'shared mailbox')
  assert.ok(block.includes("aria-label={$_('common.delete')}"))
})
