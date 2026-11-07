# Desktop Conversation List Routing

**Goal:** Make the authenticated mailbox service the read path for physical-folder and All Inboxes conversation pages when the local helper is connected.

**Architecture:** Reuse the conversation API and its typed client. Translate the desktop arguments to a validated folder or all-inboxes scope and numeric page cursor, then return the API's existing `message.Conversation` summaries directly. Keep virtual-folder list paths on their existing local operations until those scopes have API contracts.

## Behavior

- Query service status before routing. Fall back to the current local query only when the helper is unreachable; return authorization, capability, and API errors to the caller without silently bypassing the service.
- Route the frontend's actual All Inboxes adapter (`GetUnifiedFolderConversations("inbox", ...)`) through the all-inboxes API scope.
- Preserve the Wails method signatures, sort order, filters, offset, and limit. Split requested windows above the API's 100-summary page cap into bounded requests, combining results only when all pages succeed.
- Preserve the local store's supported `readLater`, `replyLater`, and combined triage filters at the authenticated API boundary.
- Keep asynchronous visible-page body prefetch. Use the existing body-fetch path so connected desktops fetch through the service and cancelled prefetch contexts still cancel work.
- Keep on-demand message-body reads unchanged.

## Verification

- Format changed Go files and run `git diff --check`.
- Build the app and affected automation/helper packages.
- Do not run tests unless requested separately.

## Dependency

Requires HOSTED-SERVER-013's authenticated `POST /v1/conversations` route and typed client.
