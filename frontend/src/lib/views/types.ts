// Shared view, thread and message-reference contract (D-002, UX-002).
//
// These types mirror internal/views/model.go field for field, and both are
// checked against the fixtures in internal/views/testdata. Type-only: nothing
// here may import runtime code, so the pure modules next to it stay loadable
// from node tests.
//
// Four choices stay independent of each other:
//   Scope    which messages are eligible          (in the Go contract too)
//   Grouping how eligible items are organized     (in the Go contract too)
//   Reader   how an opened thread is rendered     (UI only)
//   Layout   which panels are visible             (UI only)

export type ScopeKind = 'all_inboxes' | 'folder' | 'virtual_folder' | 'search'

/**
 * The messages a view contains. Fields outside the kind must be absent.
 * `within` limits a search; absent means all inboxes, and it is never a search.
 */
export interface Scope {
  kind: ScopeKind
  accountId?: string
  folderId?: string
  virtualFolderId?: string
  query?: string
  within?: Scope
}

/** 'people' is reserved for the later People view. */
export type Grouping = 'threads' | 'people'

/** What a client asks for. */
export interface ViewSpec {
  scope: Scope
  grouping: Grouping
}

/** How an opened thread is rendered. UI only; never changes thread membership or flags. */
export type ReaderMode = 'conversation' | 'traditional'

/** Folder drawer state. UI only. */
export type DrawerState = 'closed' | 'open' | 'docked'

/** Which panels are visible. UI only; temporary per window unless a task persists it. */
export interface Layout {
  drawer: DrawerState
  /** Thread Focus: only the open thread is shown. */
  focus: boolean
}

/** The UI-only half of a view. Agents never see it. */
export interface Presentation {
  reader: ReaderMode
  layout: Layout
}

/** One local message. The account is part of the identity. */
export interface MessageRef {
  accountId: string
  messageId: string
}

/** One thread within one account. */
export interface ThreadRef {
  accountId: string
  threadId: string
}

export interface Address {
  name: string
  email: string
}

/** Whether a body is available locally. */
export type ContentState = 'cached' | 'not_fetched' | 'fetch_failed'

export interface ThreadMessage {
  ref: MessageRef
  folderId: string
  /** The RFC 5322 Message-ID header. Copies of one email can share it. */
  rfcMessageId?: string
  from: Address
  /** RFC 3339 timestamp. */
  date: string
  /**
   * True when the message is in the view's scope. Others are conversation
   * context. An action on the view must not reach them unless its target says so.
   */
  matched: boolean
  isRead: boolean
  isStarred: boolean
  isAnswered: boolean
  hasAttachments: boolean
  contentState: ContentState
}

export interface BodyCounts {
  total: number
  cached: number
  notFetched: number
  fetchFailed: number
}

/** Whether the client has seen the whole thread. Complete only when every body is cached. */
export interface Completeness {
  complete: boolean
  bodies: BodyCounts
}

/** An opened thread: every known message in chronological order. */
export interface ThreadView {
  ref: ThreadRef
  subject: string
  latestDate: string
  participants: Address[]
  messageCount: number
  matchedCount: number
  messages: ThreadMessage[]
  completeness: Completeness
}

/** One row of a list. `matched` names the messages that put the thread in the view. */
export interface ThreadSummary {
  ref: ThreadRef
  subject: string
  snippet: string
  latestDate: string
  participants: Address[]
  messageCount: number
  unreadCount: number
  hasAttachments: boolean
  isStarred: boolean
  matched: MessageRef[]
}

/** One account could not answer; the page is returned but incomplete. */
export interface PartialError {
  accountId: string
  code: string
  message: string
}

/** One page of a list. `nextCursor` is opaque and bound to the scope it was issued for. */
export interface ThreadPage {
  items: ThreadSummary[]
  nextCursor?: string
  /** False when any account failed or the list was cut short. */
  complete: boolean
  errors?: PartialError[]
}
