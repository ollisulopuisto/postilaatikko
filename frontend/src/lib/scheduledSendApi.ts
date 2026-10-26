import type { smtp } from '../../wailsjs/go/models'

export interface SendLaterConfig {
  accountId: string
  endpoint: string
  helperKey: string
  helperAccountId: string
}

export interface SendLaterSetup {
  config: SendLaterConfig
  clientKey: string
  keyAvailable: boolean
}

export interface ScheduledJob {
  id: string
  accountId: string
  draftId: string
  revision: number
  state: string
  dueAt: string
  code: string
  updatedAt: string
  subject?: string
  from?: string
  to?: string
  toCount?: number
  previewAvailable: boolean
  draftCleanup: string
}

export interface ScheduledPage {
  jobs: ScheduledJob[]
  total: number
  keyAvailable: boolean
}

export interface ScheduleResult {
  job: ScheduledJob
  error?: string
}

interface Backend {
  GetSendLaterSetup(accountId: string): Promise<SendLaterSetup>
  InitializeSendLaterClient(): Promise<string>
  SaveSendLaterConfig(config: SendLaterConfig): Promise<void>
  ScheduleMessage(accountId: string, message: smtp.ComposeMessage, draftId: string, due: string): Promise<ScheduleResult>
  ListScheduledMessages(accountId: string): Promise<ScheduledJob[]>
  UpdateScheduledMessage(id: string, action: string): Promise<ScheduleResult>
  RescheduleMessage(id: string, due: string): Promise<ScheduleResult>
  GetScheduledPage(accountId: string, history: boolean, limit: number, offset: number): Promise<ScheduledPage>
  SendMessageWithDraftGuard(accountId: string, message: smtp.ComposeMessage, draftId: string): Promise<void>
}

// Both Wails entry points export the same methods. Keep this narrow adapter
// independent of generated files so the integrator can regenerate bindings once.
export function sendLaterBackend(detached = false): Backend {
  const bindings = window as unknown as { go: { app: { App: Backend; ComposerApp: Backend } } }
  return detached ? bindings.go.app.ComposerApp : bindings.go.app.App
}
