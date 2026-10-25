// Maps the Gmail API state reported by the backend (GetGmailAPIStatus) to what
// account settings show. Kept pure so the mapping is testable without a DOM.

export type GmailApiTone = 'good' | 'warning' | 'neutral'

export interface GmailApiStatusInput {
  applicable: boolean
  state: string
  since?: string
}

export interface GmailApiStatusView {
  /** i18n key of the explanation shown under the title. */
  key: string
  tone: GmailApiTone
  since?: string
}

const VIEWS: Record<string, { key: string; tone: GmailApiTone }> = {
  unknown: { key: 'account.gmailApi.unknown', tone: 'neutral' },
  active: { key: 'account.gmailApi.active', tone: 'good' },
  unavailable: { key: 'account.gmailApi.unavailable', tone: 'warning' },
  reauthRequired: { key: 'account.gmailApi.reauthRequired', tone: 'warning' },
  notUsed: { key: 'account.gmailApi.notUsed', tone: 'neutral' },
}

/** Returns null when the row should not be shown at all. */
export function gmailApiStatusView(status: GmailApiStatusInput | undefined): GmailApiStatusView | null {
  if (!status || !status.applicable) return null
  const view = VIEWS[status.state] ?? VIEWS.unknown
  return { key: view.key, tone: view.tone, since: status.since }
}
