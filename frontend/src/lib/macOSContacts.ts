/** Read-only native Contacts bridge. Permission requests belong to user actions. */
export interface MacOSPerson {
  id: string
  name: string
  emails: string[]
}

export interface MacOSContactsResult {
  people: MacOSPerson[]
  truncated: boolean
}

interface Backend {
  GetMacOSContactsStatus(): Promise<string>
  RequestMacOSContactsAccess(): Promise<string>
  SearchMacOSContacts(query: string, limit: number): Promise<MacOSContactsResult>
  OpenMacOSContact(id: string): Promise<void>
}

// Keep this adapter independent of generated files, as with scheduledSendApi.
export function macOSContactsBackend(): Backend {
  return (window as unknown as { go: { app: { App: Backend } } }).go.app.App
}
