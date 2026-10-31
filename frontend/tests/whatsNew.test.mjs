// WN-001: What's New changelog parsing and upgrade semantics.
//
// The dialog must show the sections newer than the last acknowledged version
// (current version only on first run), never [Unreleased], and nothing on a
// downgrade or reinstall of an older build.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { readFile } from 'node:fs/promises'

async function load(relative) {
  const source = await readFile(new URL(relative, import.meta.url), 'utf8')
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  })
  return import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)
}

const { compareVersions, parseChangelog, sectionsSince, shouldShowAfterLaunch } = await load('../src/lib/utils/whatsNew.ts')

const realChangelog = await readFile(new URL('../../CHANGELOG.md', import.meta.url), 'utf8')

test('compareVersions orders calver and same-day patch releases numerically', () => {
  assert.equal(compareVersions('2026.10.29', '2026.10.29.1'), -1)
  assert.equal(compareVersions('2026.10.29.1', '2026.10.29'), 1)
  assert.equal(compareVersions('2026.10.30', '2026.10.30'), 0)
  assert.equal(compareVersions('v0.3.21', '2026.10.29'), -1)
  assert.equal(compareVersions('2026.10.30', 'v2026.10.30'), 0)
  assert.equal(compareVersions('2026.9.30', '2026.10.1'), -1)
})

test('the shipped CHANGELOG parses into the real released sections, skipping [Unreleased]', () => {
  const sections = parseChangelog(realChangelog)
  assert.ok(sections.length >= 5, `expected several released sections, got ${sections.length}`)
  assert.equal(sections[0].version, '2026.10.30')
  assert.ok(sections.every(s => s.items.length > 0))
  assert.ok(sections.some(s => s.version === '2026.10.29.1'))
  assert.ok(sections.some(s => s.version === '0.3.21'))
  assert.ok(!sections.some(s => /unreleased/i.test(s.version)))
})

test('an upgrade shows every section newer than the acknowledged version, newest first', () => {
  const sections = parseChangelog(realChangelog)
  const shown = sectionsSince(sections, '2026.10.30', '2026.10.29')
  assert.deepEqual(shown.map(s => s.version), ['2026.10.30', '2026.10.29.1'])
})

test('the first run of the feature shows the current version only, never full history', () => {
  const sections = parseChangelog(realChangelog)
  const shown = sectionsSince(sections, '2026.10.30', '')
  assert.deepEqual(shown.map(s => s.version), ['2026.10.30'])
})

test('an upgrade never shows sections from a future build', () => {
  const sections = parseChangelog(realChangelog)
  const shown = sectionsSince(sections, '2026.10.29', '2026.10.28')
  assert.ok(!shown.some(s => s.version === '2026.10.30'))
})

test('the launch check fires on first run and upgrades, not on downgrades or same version', () => {
  assert.equal(shouldShowAfterLaunch('2026.10.30', ''), true)
  assert.equal(shouldShowAfterLaunch('2026.10.30', '2026.10.29.1'), true)
  assert.equal(shouldShowAfterLaunch('2026.10.29', '2026.10.30'), false)
  assert.equal(shouldShowAfterLaunch('2026.10.30', '2026.10.30'), false)
})
