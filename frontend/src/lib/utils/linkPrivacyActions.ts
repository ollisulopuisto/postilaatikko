// Shared action boundary: choose the outgoing URL immediately before use.
export function linkForAction(url: string, original: boolean, clean: (value: string) => string): string {
  return original ? url : clean(url)
}

export function openResolvedLink(url: string, original: boolean, clean: (value: string) => string, open: (value: string) => void): void {
  open(linkForAction(url, original, clean))
}

export async function copyResolvedLink(url: string, clean: (value: string) => string, copy: (value: string) => Promise<void>): Promise<void> {
  await copy(linkForAction(url, false, clean))
}
