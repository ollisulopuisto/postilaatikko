/** Presentation copy only: authored email colors must not defeat reader contrast.
 * A template is inert, so inspecting it never attaches remote email resources.
 * Stored bodies and Traditional/original rendering are left untouched.
 */
export function normalizeConversationHtml(html: string): string {
  const template = document.createElement('template')
  template.innerHTML = html
  for (const style of template.content.querySelectorAll('style, link')) style.remove()
  for (const element of template.content.querySelectorAll<HTMLElement>('*')) {
    for (const attribute of ['color', 'bgcolor', 'background']) element.removeAttribute(attribute)
    for (const property of [
      'color', 'background', 'background-color', 'background-image',
      '-webkit-text-fill-color', 'text-shadow', 'opacity', 'filter', 'mix-blend-mode',
      'font-family', 'font-size', 'line-height',
    ]) element.style.removeProperty(property)
  }
  return template.innerHTML
}
