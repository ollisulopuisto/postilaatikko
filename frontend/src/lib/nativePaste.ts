/** Insert text from the system clipboard into the currently focused editor. */
export function insertClipboardTextAtFocus(text: string): void {
  if (!text || !document.hasFocus()) return

  const active = document.activeElement
  if (!(active instanceof HTMLElement) || active instanceof HTMLButtonElement) return

  const input = active instanceof HTMLInputElement || active instanceof HTMLTextAreaElement
  const editable = active.isContentEditable || active.closest('[contenteditable="true"]')
  if (!input && !editable) return
  if (input && (active as HTMLInputElement | HTMLTextAreaElement).disabled) return
  if (input && (active as HTMLInputElement | HTMLTextAreaElement).readOnly) return

  if (input) {
    const field = active as HTMLInputElement | HTMLTextAreaElement
    const start = field.selectionStart ?? field.value.length
    const end = field.selectionEnd ?? start
    if (field instanceof HTMLInputElement && field.type === 'email') {
      field.value = `${field.value.slice(0, start)}${text}${field.value.slice(end)}`
    } else {
      field.setRangeText(text, start, end, 'end')
    }
    field.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertFromPaste', data: text }))
    return
  }

  // execCommand insertText preserves the editor's input transaction and
  // history integration. execCommand('paste') is blocked by WebKit because
  // this callback is not a trusted paste gesture.
  document.execCommand('insertText', false, text)
}
