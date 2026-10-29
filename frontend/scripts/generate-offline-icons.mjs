// Bundle only the Iconify glyphs referenced by the host app and bundled
// extensions. Importing whole @iconify-json collections made every window
// download and parse several megabytes of unused icon paths.
import { readdir, readFile, mkdir, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { dirname, join, relative } from 'node:path'

const scriptDir = dirname(fileURLToPath(import.meta.url))
const frontendDir = dirname(scriptDir)
const repoDir = dirname(frontendDir)
const outputPath = join(frontendDir, 'src/lib/generated/offline-icons.json')
const collections = new Map([
  ['mdi', '@iconify-json/mdi'],
  ['lucide', '@iconify-json/lucide'],
  ['heroicons', '@iconify-json/heroicons'],
  ['logos', '@iconify-json/logos'],
  ['simple-icons', '@iconify-json/simple-icons'],
])
const iconPattern = /\b(mdi|lucide|heroicons|logos|simple-icons):([A-Za-z0-9_-]+)/g
const sourceExtensions = new Set(['.svelte', '.ts', '.js', '.mjs', '.json', '.html'])
const namesByPrefix = new Map([...collections.keys()].map(prefix => [prefix, new Set()]))

async function scanDirectory(directory) {
  let entries
  try {
    entries = await readdir(directory, { withFileTypes: true })
  } catch (error) {
    if (error?.code === 'ENOENT') return
    throw error
  }
  await Promise.all(entries.map(async entry => {
    if (entry.name === 'node_modules' || entry.name === 'dist' || entry.name === 'build') return
    const path = join(directory, entry.name)
    if (entry.isDirectory()) {
      await scanDirectory(path)
      return
    }
    const extension = entry.name.slice(entry.name.lastIndexOf('.'))
    if (!sourceExtensions.has(extension)) return
    const source = await readFile(path, 'utf8')
    for (const match of source.matchAll(iconPattern)) namesByPrefix.get(match[1]).add(match[2])
  }))
}

await scanDirectory(join(frontendDir, 'src'))
await scanDirectory(join(repoDir, 'extensions'))

function resolveIconNames(collection, names) {
  const icons = {}
  const aliases = {}
  const pending = [...names]
  while (pending.length) {
    const name = pending.pop()
    if (icons[name] || aliases[name]) continue
    if (collection.icons[name]) {
      icons[name] = collection.icons[name]
      continue
    }
    const alias = collection.aliases?.[name]
    if (!alias) throw new Error(`Unknown Iconify icon: ${collection.prefix}:${name}`)
    aliases[name] = alias
    if (alias.parent) pending.push(alias.parent)
  }
  return { icons, ...(Object.keys(aliases).length ? { aliases } : {}) }
}

const output = {}
let referencedCount = 0
for (const [prefix, packageName] of collections) {
  const jsonPath = join(frontendDir, 'node_modules', packageName, 'icons.json')
  const collection = JSON.parse(await readFile(jsonPath, 'utf8'))
  const names = namesByPrefix.get(prefix)
  const subset = resolveIconNames(collection, names)
  referencedCount += names.size
  output[prefix] = {
    prefix,
    width: collection.width,
    height: collection.height,
    ...subset,
  }
}

await mkdir(dirname(outputPath), { recursive: true })
await writeFile(outputPath, `${JSON.stringify(output)}\n`)
const { size } = await (await import('node:fs/promises')).stat(outputPath)
console.log(`Generated ${referencedCount} offline icons (${size.toLocaleString()} bytes) at ${relative(repoDir, outputPath)}`)
