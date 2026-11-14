// Renders the shortcut tables in docs/KEYBOARD_SHORTCUTS.md from the keyboard
// registry (src/lib/keyboard/registry.ts and extensions/*/frontend/keyboard/registry.ts),
// so the doc cannot drift from the app.
//
//   npm run docs:shortcuts          rewrite the generated blocks
//   npm run docs:shortcuts -- --check   exit 1 if the committed doc is stale
//
// Only text between the GENERATED markers is replaced; the prose around it is hand-written.
import { readFile, writeFile, readdir } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import path from 'node:path'
import { createServer } from 'vite'

const frontendDir = fileURLToPath(new URL('..', import.meta.url))
const repoDir = path.resolve(frontendDir, '..')
const docPath = path.join(repoDir, 'docs', 'KEYBOARD_SHORTCUTS.md')
const check = process.argv.includes('--check')

const server = await createServer({
  root: frontendDir,
  configFile: path.join(frontendDir, 'vite.config.ts'),
  logLevel: 'error',
  server: { middlewareMode: true, hmr: false, watch: null },
  appType: 'custom',
  optimizeDeps: { noDiscovery: true, include: [] },
})

let registry, format
try {
  registry = await server.ssrLoadModule('/src/lib/keyboard/registry.ts')
  format = await server.ssrLoadModule('/src/lib/keyboard/format.ts')
} finally {
  await server.close()
}
const { CORE_GROUPS, CORE_SHORTCUTS, EXTENSION_REGISTRIES } = registry
const { formatCombo } = format

const en = JSON.parse(await readFile(path.join(frontendDir, 'src/lib/i18n/locales/en.json'), 'utf8'))
// Extensions ship their own English strings, namespaced by extension id (e.g. calendar.shortcuts.group).
for (const name of (await readdir(path.join(repoDir, 'extensions'), { withFileTypes: true })).filter(d => d.isDirectory()).map(d => d.name).sort()) {
  const file = path.join(repoDir, 'extensions', name, 'frontend/i18n/locales/en.json')
  try {
    Object.assign(en, JSON.parse(await readFile(file, 'utf8')))
  } catch (error) {
    if (error.code !== 'ENOENT') throw error
  }
}
function t(key) {
  const value = key.split('.').reduce((node, part) => (node == null ? node : node[part]), en)
  if (typeof value !== 'string') throw new Error(`Missing English text for i18n key ${key}`)
  return value
}

const cell = text => text.replace(/\|/g, '\\|')
const code = text => (text.includes('`') ? '`` ' + text + ' ``' : '`' + text + '`')

function comboText(combo, macOnly) {
  if (combo === 'LeftAlt+RightAlt') return '`Left Alt` + `Right Alt` pressed together'
  if (macOnly) return code(formatCombo(combo, true))
  const other = formatCombo(combo, false)
  const mac = formatCombo(combo, true)
  return other === mac ? code(other) : `${code(other)} (macOS ${code(mac)})`
}

function keysText(def) {
  const text = def.combos.map(c => comboText(c, def.macOnly)).join(' / ')
  return def.macOnly ? `${text} (macOS only)` : text
}

function paneNote(def, host) {
  if (!def.panes || (host && def.panes.length === 1 && def.panes[0] === host)) return ''
  return ` _(${def.panes.map(p => p[0].toUpperCase() + p.slice(1)).join(', ')} only)_`
}

function table(defs, host) {
  const rows = defs.map(def => `| ${cell(keysText(def))} | ${cell(t(def.label))}${paneNote(def, host)} |`)
  return ['| Shortcut | Action |', '|----------|--------|', ...rows].join('\n')
}

function section(heading, groups, defs, host) {
  const out = []
  for (const group of groups) {
    const inGroup = defs.filter(def => def.group === group.id)
    if (inGroup.length === 0) continue
    out.push(`${heading} ${t(group.label)}`, '', table(inGroup, host), '')
  }
  return out.join('\n').trimEnd()
}

const core = section('###', CORE_GROUPS, CORE_SHORTCUTS, 'mail')
const extensions = EXTENSION_REGISTRIES.map(reg => {
  const title = reg.extensionId[0].toUpperCase() + reg.extensionId.slice(1)
  return `### ${title}\n\n` + section('####', reg.groups, reg.shortcuts, reg.extensionId)
}).join('\n\n')

const blocks = { core, extensions }
let doc = await readFile(docPath, 'utf8')
for (const [name, body] of Object.entries(blocks)) {
  const begin = `<!-- BEGIN GENERATED: ${name} (run \`npm run docs:shortcuts\` in frontend/; do not edit by hand) -->`
  const end = `<!-- END GENERATED: ${name} -->`
  const re = new RegExp(`<!-- BEGIN GENERATED: ${name}[^\\n]*-->[\\s\\S]*?${end}`)
  if (!re.test(doc)) throw new Error(`Markers for the "${name}" block are missing from docs/KEYBOARD_SHORTCUTS.md`)
  doc = doc.replace(re, () => `${begin}\n\n${body}\n\n${end}`)
}

if (check) {
  const current = await readFile(docPath, 'utf8')
  if (current !== doc) {
    console.error('docs/KEYBOARD_SHORTCUTS.md is out of date with the shortcut registry. Run `npm run docs:shortcuts` in frontend/ and commit the result.')
    process.exit(1)
  }
  console.log('docs/KEYBOARD_SHORTCUTS.md matches the shortcut registry.')
} else {
  await writeFile(docPath, doc)
  console.log('Wrote docs/KEYBOARD_SHORTCUTS.md')
}
