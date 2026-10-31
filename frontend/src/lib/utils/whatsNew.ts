// What's New changelog parsing and version selection.
//
// CHANGELOG.md uses "## Postilaatikko <calver>" headings (older entries keep
// a "v" prefix: "## Postilaatikko v0.3.21") plus a "## [Unreleased]" section
// that is never shown. All comparisons are numeric per version segment, so
// same-day patch releases (2026.10.29.1 > 2026.10.29) order correctly.

export interface ChangelogSection {
  version: string
  items: string[]
}

// Numeric per-segment compare; ignores a leading "v". Missing trailing
// segments count as zero, so 2026.10.29 < 2026.10.29.1.
export function compareVersions(a: string, b: string): number {
  const pa = a.replace(/^v/i, '').split('.').map(n => parseInt(n, 10) || 0)
  const pb = b.replace(/^v/i, '').split('.').map(n => parseInt(n, 10) || 0)
  const len = Math.max(pa.length, pb.length)
  for (let i = 0; i < len; i++) {
    const d = (pa[i] ?? 0) - (pb[i] ?? 0)
    if (d !== 0) return d < 0 ? -1 : 1
  }
  return 0
}

const HEADING = /^##\s+Postilaatikko\s+v?(\d+(?:\.\d+)*)\s*$/m

export function parseChangelog(markdown: string): ChangelogSection[] {
  const sections: ChangelogSection[] = []
  // Split into heading + body pairs; [Unreleased] and unknown headings drop out.
  for (const chunk of markdown.split(/^## /m)) {
    const match = chunk.match(/^Postilaatikko\s+v?(\d+(?:\.\d+)*)\s*\n([\s\S]*)$/)
    if (!match) continue
    const items = match[2]
      .split('\n')
      .filter(line => line.startsWith('- '))
      .map(line => line.slice(2).trim())
      .filter(Boolean)
    if (items.length > 0) sections.push({ version: match[1], items })
  }
  return sections.sort((a, b) => compareVersions(b.version, a.version))
}

// Sections to show after an upgrade: everything newer than the last version
// the user acknowledged, up to and including the running build. A first run
// (no recorded version) shows the current version only, never full history.
export function sectionsSince(sections: ChangelogSection[], current: string, lastSeen: string): ChangelogSection[] {
  if (!lastSeen) return sections.filter(s => compareVersions(s.version, current) === 0)
  return sections.filter(s => compareVersions(s.version, lastSeen) > 0 && compareVersions(s.version, current) <= 0)
}

// The after-upgrade dialog fires on the first run and on upgrades only.
// A downgrade or reinstall of an older build shows nothing and leaves the
// recorded version untouched.
export function shouldShowAfterLaunch(current: string, lastSeen: string): boolean {
  return lastSeen === '' || compareVersions(current, lastSeen) > 0
}

export { HEADING as _headingForTests }
