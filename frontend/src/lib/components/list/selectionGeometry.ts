export interface SelectionRow { top: number; bottom: number; checked: boolean }
export interface SelectionRun { top: number; bottom: number }
// Geometry describes the checked object, never the hovered row or the gaps.
export function selectionRuns(rows: readonly SelectionRow[]): SelectionRun[] {
  const runs: SelectionRun[] = []
  let current: SelectionRun | undefined
  for (const row of rows) {
    if (!row.checked) { current = undefined; continue }
    if (current && row.top <= current.bottom + 0.5) current.bottom = row.bottom
    else { current = {top: row.top, bottom: row.bottom}; runs.push(current) }
  }
  return runs
}
