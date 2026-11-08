import type { message } from '../../../../wailsjs/go/models'

const rfcID = (value = '') => value.trim().replace(/^</, '').replace(/>$/, '')

/**
 * Deduplicate message copies sharing the same RFC Message-ID.
 * Prioritizes:
 * 1. Messages belonging to the active folder (folderId) or marked matched !== false
 * 2. Messages with body fetched (bodyFetched)
 * 3. First encountered message copy (maintaining chronological order)
 */
export function deduplicateMessages<T extends Partial<message.Message> & { id: string }>(
  messages: T[],
  currentFolderId?: string
): T[] {
  if (!messages || messages.length <= 1) return messages || []

  const dedupMap = new Map<string, { index: number; msg: T }>()
  const orderedKeys: string[] = []

  const isBetterCopy = (candidate: T, existing: T): boolean => {
    const candidateInFolder = !!currentFolderId && candidate.folderId === currentFolderId
    const existingInFolder = !!currentFolderId && existing.folderId === currentFolderId
    if (candidateInFolder !== existingInFolder) {
      return candidateInFolder
    }

    const candidateMatched = (candidate as any).matched !== false
    const existingMatched = (existing as any).matched !== false
    if (candidateMatched !== existingMatched) {
      return candidateMatched
    }

    const candidateFetched = !!candidate.bodyFetched
    const existingFetched = !!existing.bodyFetched
    if (candidateFetched !== existingFetched) {
      return candidateFetched
    }

    return false
  }

  for (const m of messages) {
    const rawMid = (m as any).messageId || (m as any).rfcMessageId || ''
    const key = rfcID(rawMid) || m.id
    const prev = dedupMap.get(key)
    if (prev) {
      if (isBetterCopy(m, prev.msg)) {
        dedupMap.set(key, { index: prev.index, msg: m })
      }
    } else {
      dedupMap.set(key, { index: orderedKeys.length, msg: m })
      orderedKeys.push(key)
    }
  }

  return orderedKeys.map(key => dedupMap.get(key)!.msg)
}
