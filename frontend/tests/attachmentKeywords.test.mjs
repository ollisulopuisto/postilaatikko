// Bolt optimization regression: attachment keyword mention detection in composerUtils.ts
//
// OLD: `new RegExp(\`\\b${escaped}\\b\`)` was dynamically constructed and compiled
// for every keyword on every call to `textMentionsAttachment`.
//
// NEW: pre-compiled regex instances at module scope.
//
// This test verifies:
// 1. old and new implementations agree across all test cases (correctness)
// 2. new is materially faster than old (performance regression guard)

import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  ATTACHMENT_KEYWORDS,
  textMentionsAttachment
} from '../src/lib/components/composer/composerUtils.ts'

// ---- OLD implementation ----
function textMentionsAttachmentOld(text) {
  const lowerText = text.toLowerCase()
  return ATTACHMENT_KEYWORDS.some(keyword => {
    const escaped = keyword.replace(/'/g, "['\u2019]")
    const pattern = new RegExp(`\\b${escaped}\\b`)
    return pattern.test(lowerText)
  })
}

const testCases = [
  '',
  '   ',
  'Hello world! No attachments here.',
  'Please find attached the updated quarterly budget spreadsheet.',
  'Please find attached the file.',
  'See attached report for details.',
  'I have attached the document you requested.',
  "I've attached the invoice.",
  "I’ve attached the invoice with curly quote.",
  'The file is enclosed in this message.',
  'Enclosed please find the signed agreement.',
  'We are enclosing the relevant receipts.',
  'Please enclose your contact information.',
  'Check out the attachment.',
  'Multiple attachments are included.',
  'I am attaching the photos from yesterday.',
  'Can you attach that document?',
  'Attachment of strings: unattached, reattach, nonattachment should not match as whole words except boundaries.',
  'unattached should not match',
  'reattaching is a substring',
  'ATTENTION: do not attach anything here.',
  'PLEASE FIND ATTACHED in uppercase.',
  'Random text with numbers 123456 and symbols #$%^&*.',
  'Multiple lines\nof text\nwith please find attached\nin the middle.'
]

// ---- Correctness tests ----
for (const [idx, text] of testCases.entries()) {
  test(`textMentionsAttachment old == new for case #${idx}`, () => {
    assert.equal(textMentionsAttachment(text), textMentionsAttachmentOld(text))
  })
}

// ---- Benchmark & regression guard ----
function bench(fn, iterations) {
  for (let i = 0; i < 200; i++) fn() // warmup
  const start = process.hrtime.bigint()
  for (let i = 0; i < iterations; i++) fn()
  const end = process.hrtime.bigint()
  return Number(end - start) / 1_000_000 // ms
}

const ITER = 5000
function runOld() {
  for (const text of testCases) {
    textMentionsAttachmentOld(text)
  }
}

function runNew() {
  for (const text of testCases) {
    textMentionsAttachment(text)
  }
}

test('pre-compiled attachment keyword regexes are faster than old implementation', () => {
  const oldMs = bench(runOld, ITER)
  const newMs = bench(runNew, ITER)
  console.log(`textMentionsAttachment old=${oldMs.toFixed(2)} ms, new=${newMs.toFixed(2)} ms over ${ITER} iterations (${(oldMs / newMs).toFixed(2)}x faster)`)
  assert.ok(newMs < oldMs, `expected new (${newMs.toFixed(2)} ms) < old (${oldMs.toFixed(2)} ms)`)
  // Guard against regression: expect at least 1.5x speedup even under noisy CI runners
  assert.ok(oldMs / newMs > 1.5, `expected >1.5x speedup, got ${(oldMs / newMs).toFixed(2)}x`)
})
