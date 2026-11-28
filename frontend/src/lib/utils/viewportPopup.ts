/** Fixed-position popups use window coordinates, never screen or iframe bounds. */
export function popupPosition(x: number, y: number, width: number, height: number,
  bounds: {left: number; top: number; width: number; height: number}, gap = 8) {
  const left = bounds.left + gap, top = bounds.top + gap
  const right = bounds.left + bounds.width - gap, bottom = bounds.top + bounds.height - gap
  return {
    x: Math.max(left, Math.min(x + width > right ? x - width : x, right - width)),
    y: Math.max(top, Math.min(y + height > bottom ? y - height : y, bottom - height)),
  }
}

export function viewportPopup(node: HTMLElement, anchor: {x: number; y: number}) {
  let current = anchor
  function place() {
    const viewport = window.visualViewport
    const bounds = {left: viewport?.offsetLeft ?? 0, top: viewport?.offsetTop ?? 0,
      width: viewport?.width ?? window.innerWidth, height: viewport?.height ?? window.innerHeight}
    node.style.boxSizing = 'border-box'
    node.style.minWidth = '0'
    node.style.width = 'max-content'
    node.style.maxWidth = `${Math.max(0, bounds.width - 16)}px`
    node.style.maxHeight = `${Math.max(0, bounds.height - 16)}px`
    node.style.overflowY = 'auto'
    node.style.overflowWrap = 'anywhere'
    const rect = node.getBoundingClientRect()
    const position = popupPosition(current.x, current.y, rect.width, rect.height, bounds)
    node.style.left = `${position.x}px`
    node.style.top = `${position.y}px`
  }
  const observer = new ResizeObserver(place)
  observer.observe(node)
  window.addEventListener('resize', place)
  window.visualViewport?.addEventListener('resize', place)
  window.visualViewport?.addEventListener('scroll', place)
  place()
  return {
    update(anchor: {x: number; y: number}) { current = anchor; place() },
    destroy() {
      observer.disconnect()
      window.removeEventListener('resize', place)
      window.visualViewport?.removeEventListener('resize', place)
      window.visualViewport?.removeEventListener('scroll', place)
    },
  }
}
