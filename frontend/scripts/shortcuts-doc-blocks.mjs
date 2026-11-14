// The pure part of gen-shortcuts-doc.mjs: replace the GENERATED blocks in the doc text.
// Kept apart so it can be tested without loading the app's modules.

/** Returns `doc` with each generated block replaced; text outside the markers is untouched.
 *  Line endings are normalised to LF first, so a Windows checkout (core.autocrlf) of an
 *  up-to-date doc is not reported as stale. */
export function replaceGeneratedBlocks(docText, blocks) {
  let doc = docText.replace(/\r\n/g, '\n')
  for (const [name, body] of Object.entries(blocks)) {
    const begin = `<!-- BEGIN GENERATED: ${name} (run \`npm run docs:shortcuts\` in frontend/; do not edit by hand) -->`
    const end = `<!-- END GENERATED: ${name} -->`
    // Plain string search, not a RegExp built from `name`: nothing here is interpreted as a pattern.
    const startAt = doc.indexOf(`<!-- BEGIN GENERATED: ${name}`)
    const endAt = startAt === -1 ? -1 : doc.indexOf(end, startAt)
    if (startAt === -1 || endAt === -1) {
      throw new Error(`Markers for the "${name}" block are missing from docs/KEYBOARD_SHORTCUTS.md`)
    }
    doc = doc.slice(0, startAt) + `${begin}\n\n${body}\n\n${end}` + doc.slice(endAt + end.length)
  }
  return doc
}

/** Whether the committed doc already matches (ignoring line-ending style). */
export function isUpToDate(docText, blocks) {
  return docText.replace(/\r\n/g, '\n') === replaceGeneratedBlocks(docText, blocks)
}
