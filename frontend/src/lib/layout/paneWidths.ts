// Default column widths for the three-pane layout (P-039).
//
// The sidebar and message list were wide enough to crowd the message. The new
// defaults give the message pane the room; the user can still drag the dividers.

export interface PaneWidths {
  sidebarWidth: number
  listWidth: number
}

export const PANE_WIDTH_DEFAULTS: PaneWidths = {
  sidebarWidth: 260,
  listWidth: 320,
}

// Returns the widths to use after the one-time move to the narrower defaults.
// `alreadyApplied` is true once the migration has run, after which the saved
// widths are kept as they are. The input is never modified.
export function applyNarrowerDefaults(
  widths: PaneWidths,
  alreadyApplied: boolean,
): { widths: PaneWidths; applied: boolean } {
  if (alreadyApplied) return { widths: { ...widths }, applied: true }
  return { widths: { ...PANE_WIDTH_DEFAULTS }, applied: true }
}
