/** A view-local header snapshot. Passive updates refresh row data, never membership/order. */
export interface FrozenOrder<T> { baseline: T[]; latest: T[]; live: T[]; changed: string[] }
export function freezeOrder<T>(rows: T[], clone: (row: T) => T): FrozenOrder<T> {
  return { baseline: rows.map(clone), latest: rows, live: rows, changed: [] }
}
export function updateFrozenOrder<T>(state: FrozenOrder<T>, incoming: T[], key: (row: T) => string, signature: (row: T) => string): FrozenOrder<T> {
  const before = new Map(state.baseline.map(row => [key(row), signature(row)]))
  const after = new Map(incoming.map(row => [key(row), signature(row)]))
  const changed = new Set<string>()
  for (const [id, value] of before) if (after.get(id) !== value) changed.add(id)
  for (const [id, value] of after) if (before.get(id) !== value) changed.add(id)
  // A date/sort-key change can reorder equal-looking rows too.
  const baselineKeys = state.baseline.map(key)
  const known = incoming.map(key).filter(id => before.has(id))
  if (known.length === baselineKeys.length) known.forEach((id, index) => { if (id !== baselineKeys[index]) changed.add(id) })
  const live = new Map(state.live.map(row => [key(row), row]))
  for (const row of incoming) live.set(key(row), row)
  return { baseline: state.baseline, latest: incoming, live: [...live.values()], changed: [...changed] }
}
export function frozenRows<T>(state: FrozenOrder<T>, key: (row: T) => string): T[] {
  const latest = new Map(state.live.map(row => [key(row), row]))
  return state.baseline.map(row => latest.get(key(row)) ?? row)
}
/** Explicit pagination appends rows without releasing existing positions. */
export function appendFrozenRows<T>(state: FrozenOrder<T>, incoming: T[], key: (row: T) => string, clone: (row: T) => T): FrozenOrder<T> {
  const known = new Set(state.baseline.map(key))
  const added = incoming.filter(row => !known.has(key(row)))
  const next = updateFrozenOrder(state, incoming, key, () => '')
  return { baseline: [...state.baseline, ...added.map(clone)], latest: incoming, live: next.live, changed: state.changed.filter(id => !added.some(row => key(row) === id)) }
}
