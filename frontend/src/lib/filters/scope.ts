// Keep the explicit-list, deduplication and legacy fallback rules in sync with
// mailfilter.Rule.ScopeAccountIDs. An empty result means every account.
export function filterAccountIDs(rule: {accountIds?: string[]; accountId?: string}): string[] {
  const explicit = [...new Set((rule.accountIds || []).filter(id => id !== ''))]
  if (explicit.length) return explicit
  return rule.accountId ? [rule.accountId] : []
}
