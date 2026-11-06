// Bolt optimization regression: domain-suffix matching used by
// MessageList.svelte `categoryForConversation`.
//
// OLD (current source): `new RegExp(`(^|[.-])${name}([.-]|$)`)` is compiled
// for every name, on every call — ~264 regex compilations per conversation
// classification. Runs for every conversation on every MessageList render
// (default inbox mode is `categories`).
//
// NEW: each domain suffix is compiled once and cached in a module-level Map.
//
// This test verifies:
// 1. old and new implementations agree across fuzzed inputs (correctness)
// 2. new is materially faster than old (performance regression guard)

import { test } from 'node:test'
import assert from 'node:assert/strict'

// ---- OLD implementation (current source semantics) ----
function domainHasOld(domain, names) {
  return names.some((name) => new RegExp(`(^|[.-])${name}([.-]|$)`).test(domain))
}

// ---- NEW implementation (optimized, compiled-once cache) ----
// `cache` defaults to a shared module-level Map for production use; callers
// (e.g. benchmarks) may pass a fresh Map to stay hermetic.
const sharedRegexCache = new Map()
function domainHasNew(domain, names, cache = sharedRegexCache) {
  for (const name of names) {
    let re = cache.get(name)
    if (!re) {
      re = new RegExp(`(^|[.-])${name}([.-]|$)`)
      cache.set(name, re)
    }
    if (re.test(domain)) return true
  }
  return false
}

const automatedMailboxNames = [
  'noreply', 'donotreply', 'notreply', 'autoreply', 'automaticreply', 'auto',
  'automatic', 'automated', 'automacao', 'automatico', 'automatica', 'system',
  'sistema', 'mailer', 'mailbot', 'bot', 'robot', 'daemon', 'postmaster',
  'bounce', 'bounces', 'returnpath', 'notification', 'notifications',
  'notificacao', 'notificacoes', 'alert', 'alerts', 'aviso', 'avisos',
  'update', 'updates', 'newsletter', 'news', 'digest', 'mailinglist',
  'broadcast', 'bulk', 'marketing', 'campaign', 'campaigns', 'promo',
  'promotions', 'offers', 'deals', 'sales', 'vendas', 'commercial', 'comercial',
  'service', 'services', 'support', 'suporte', 'help', 'ajuda', 'atendimento',
  'sac', 'contact', 'contato', 'faleconosco', 'info', 'information',
  'comunicacao', 'relacionamento', 'financeiro', 'billing', 'invoice',
  'faturamento', 'cobranca', 'payment', 'payments', 'pagamento', 'orders',
  'order', 'pedido', 'pedidos', 'shipping', 'delivery', 'entrega', 'account',
  'accounts', 'conta', 'security', 'seguranca', 'verify', 'verification',
  'verificacao', 'transaction', 'transactions', 'transacional',
]
const alertNames = ['alert', 'alerts', 'notify', 'notification', 'notifications', 'security', 'secure', 'account', 'accounts', 'auth', 'login', 'status', 'support']
const bankNames = ['bank', 'banco', 'finance', 'financial', 'financas', 'pay', 'payment', 'payments', 'billing', 'invoice', 'cobranca', 'insurance', 'seguro', 'health', 'saude', 'travel', 'viagens', 'hotel', 'delivery', 'entrega', 'food', 'market', 'marketplace', 'commerce', 'ecommerce', 'business', 'empresa', 'corp', 'corporate', 'service', 'services']
const peopleNames = ['gmail', 'googlemail', 'outlook', 'hotmail', 'live', 'yahoo', 'icloud', 'me', 'mac', 'protonmail', 'proton', 'zoho', 'uol', 'bol', 'terra']

const nameSets = [automatedMailboxNames, alertNames, bankNames, peopleNames]

// Deterministic fuzz seed covering real domains, edge cases and noise.
const domains = [
  'gmail.com', 'googlemail.com', 'outlook.com', 'hotmail.com', 'live.com',
  'yahoo.com', 'icloud.com', 'me.com', 'mac.com', 'protonmail.com', 'zoho.com',
  'news.mail.example.com', 'noreply@service.bank.example.com'.split('@')[1],
  'donotreply@shops.store.example.com'.split('@')[1],
  'alerts@security.account.example.com'.split('@')[1],
  'deals@promo.marketing.example.com'.split('@')[1],
  'info@empresa.business.example.com'.split('@')[1],
  'verify@security.seguranca.example.com'.split('@')[1],
  'update@news.newsletter.example.com'.split('@')[1],
  'noreply.bounce@mail.bounce.example.com'.split('@')[1],
  'example.com', 'sub.example.com', 'a.b.c.example.com', 'example',
  'noreply', 'donotreply', 'alerts.news', 'bank.finance',
  'gmail.co.uk', 'outlook.it', 'Yahoo.COM', 'Gmail.Com',
  '', 'noreply@', '@example.com', 'example..com', '-example.com',
  'example-.com', 'example.com.', 'a', 'ab', 'shop', 'shops', 'store',
  'news', 'newsletter', 'marketing', 'promo', 'deals', 'offers',
  'contact', 'support', 'service', 'services', 'billing', 'invoice',
  'payment', 'payments', 'order', 'orders', 'shipping', 'delivery',
  'account', 'accounts', 'security', 'verify', 'verification',
  'transaction', 'transactions', 'campaign', 'campaigns', 'digest',
  'broadcast', 'bulk', 'mailinglist', 'update', 'updates', 'alert',
  'alerts', 'notification', 'notifications', 'system', 'sistema',
  'mailer', 'mailbot', 'bot', 'robot', 'daemon', 'postmaster', 'bounce',
  'bounces', 'returnpath', 'auto', 'automatic', 'automated',
  'automacao', 'automatico', 'automatica', 'sales', 'vendas', 'commercial',
  'comercial', 'suporte', 'help', 'ajuda', 'atendimento', 'sac',
  'faleconosco', 'contato', 'info', 'information', 'comunicacao',
  'relacionamento', 'financeiro', 'billing', 'invoice', 'faturamento',
  'cobranca', 'health', 'saude', 'travel', 'viagens', 'hotel', 'delivery',
  'entrega', 'food', 'market', 'marketplace', 'commerce', 'ecommerce',
  'business', 'empresa', 'corp', 'corporate',
]

// ---- Correctness: old and new must agree on every input ----
for (const domain of domains) {
  for (const names of nameSets) {
    test(`domainHas old==new for domain="${domain}" set=${names.length}`, () => {
      assert.equal(domainHasOld(domain, names), domainHasNew(domain, names))
    })
  }
}

test('domainHasNew caches compiled regexes and reuses them', () => {
  sharedRegexCache.clear()
  assert.equal(sharedRegexCache.size, 0)
  // Use names that cannot match, so every name is compiled and cached.
  domainHasNew('zzz.nonexistent.example.com', ['zzznews', 'zzzmail'])
  assert.equal(sharedRegexCache.size, 2)
  // Second call with same names must not grow the cache.
  domainHasNew('zzz.nonexistent.example.com', ['zzznews', 'zzzmail'])
  assert.equal(sharedRegexCache.size, 2)
  // A new name adds exactly one entry.
  domainHasNew('zzz.bank.example.com', ['zzzbank', 'zzzfinance'])
  assert.equal(sharedRegexCache.size, 4)
})

// ---- Performance regression guard: new must beat old by a clear margin ----
function bench(fn, iterations) {
  for (let i = 0; i < 500; i++) fn() // warmup
  const start = process.hrtime.bigint()
  for (let i = 0; i < iterations; i++) fn()
  const end = process.hrtime.bigint()
  return Number(end - start) / 1_000_000 // ms
}

const ITER = 200
function runOld() {
  for (const domain of domains) {
    for (const names of nameSets) domainHasOld(domain, names)
  }
}
function runNew() {
  const localCache = new Map()
  for (const domain of domains) {
    for (const names of nameSets) domainHasNew(domain, names, localCache)
  }
}

test('domainHasNew is faster than domainHasOld (performance regression)', () => {
  const oldMs = bench(runOld, ITER)
  const newMs = bench(runNew, ITER)
  console.log(`domainHas old=${oldMs.toFixed(2)} ms new=${newMs.toFixed(2)} ms over ${ITER} iterations`)
  assert.ok(newMs < oldMs, `expected new (${newMs.toFixed(2)} ms) < old (${oldMs.toFixed(2)} ms)`)
  // Expect a clear, non-noise margin (~5x+): far fewer regex compilations.
  assert.ok(oldMs / newMs > 3, `expected >3x speedup, got ${(oldMs / newMs).toFixed(2)}x`)
})
