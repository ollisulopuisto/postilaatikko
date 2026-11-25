/** D1: sender dark CSS first, otherwise adapt painted colors, never pixels. */
export function adaptEmailDarkMode(doc: Document): 'authored' | 'adapted' {
  const css = Array.from(doc.querySelectorAll('style')).map(node => node.textContent || '').join('\n')
  if (/@media[^{}]*\(\s*prefers-color-scheme\s*:\s*dark\s*\)/i.test(css)) {
    // The app can be dark while the OS is light. Honor the app's selection
    // for sender media rules without changing any email image content.
    for (const sheet of Array.from(doc.styleSheets)) {
      for (const rule of Array.from(sheet.cssRules)) {
        if ('conditionText' in rule && 'media' in rule) {
          const media = rule as CSSMediaRule
          media.media.mediaText = media.conditionText
            .replace(/\(\s*prefers-color-scheme\s*:\s*dark\s*\)/gi, '(min-width: 0px)')
            .replace(/\(\s*prefers-color-scheme\s*:\s*light\s*\)/gi, 'not all')
        }
      }
    }
    doc.documentElement.style.colorScheme = 'dark'
    return 'authored'
  }
  const win = doc.defaultView
  if (!win) return 'adapted'
  const lightness = (value: string): number | null => {
    const parts = value.match(/^rgba?\(\s*([\d.]+)[, ]+([\d.]+)[, ]+([\d.]+)(?:\s*[,/]\s*([\d.]+))?\s*\)$/)
    if (!parts || (parts[4] !== undefined && Number(parts[4]) < 0.1)) return null
    return (0.2126 * Number(parts[1]) + 0.7152 * Number(parts[2]) + 0.0722 * Number(parts[3])) / 255
  }
  // Snapshot before writing: changing an ancestor must not change how a child
  // is classified. CSS background images, photos, SVGs and their subtrees stay
  // untouched. No global filter or image counter-filter is used.
  const rows = Array.from(doc.querySelectorAll<HTMLElement>('html, body, body *'))
    .filter(node => !node.closest('img, picture, video, svg, canvas, iframe, [data-no-invert]'))
    .map(node => {
      const style = win.getComputedStyle(node)
      return { node, background: lightness(style.backgroundColor), color: lightness(style.color), image: style.backgroundImage }
    })
  for (const { node, background, color, image } of rows) {
    if (image !== 'none') continue
    if (background !== null && background > 0.55) node.style.setProperty('background-color', '#20242b', 'important')
    if (color !== null && color < 0.55) {
      node.style.setProperty('color', node.tagName === 'A' ? '#90bcff' : '#e4e7ec', 'important')
    }
  }
  doc.documentElement.style.colorScheme = 'dark'
  doc.documentElement.style.setProperty('background-color', '#20242b', 'important')
  doc.body.style.setProperty('background-color', '#20242b', 'important')
  return 'adapted'
}
