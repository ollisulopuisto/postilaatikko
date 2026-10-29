# Conversation, Navigation and Agent API Implementation Plan (D-002)

> **For implementing agents:** Read `AGENTS.md`, `ROADMAP.md` and `TASKS.md` first. Claim exactly one eligible `UX-*` task in a separate branch/worktree. Follow the steps and acceptance criteria here; no external skill package is required to use this handoff.

**Goal:** Make personal mail easy to read as conversations, navigate across accounts, act on close to the message, and operate through an authenticated API usable by external LLM tools.

**Architecture:** Keep view scope, grouping, reader format and layout independent. Reuse existing query, threading, draft, action, sanitizer and focus behavior, adding shared email-content extraction and versioned agent operations. GUI, CLI and MCP use the same backend behavior; routine sync stays lightweight.

**Tech stack:** Go 1.26, Wails v2, Svelte 5/TypeScript, SQLite, existing Go HTML/sanitization libraries, existing IMAP/Gmail integrations, existing draft/identity services. Choose any additional MCP dependency only in its adapter task after checking maintenance and compatibility.

**Status:** User-approved plan consolidated on 2026-10-26. UX-002/004/005/006 are integrated in PRs #29/#30/#33/#34. UX-007 is reserved for Claude; current claims and integration state are in TASKS.md and docs/coordination/DISPATCH.md.
**Surveyed base:** Freshly fetched `origin/main` at `8a7a2a8`. This plan does not assert that task-branch work is merged, installed or released.

**Synchronization checkpoint:** PR #29 merged as `d3490b1` (source `d11dfaf`). Use [the accepted view contract](../api/view-contract.md), `internal/views/model.go` and `frontend/src/lib/views/` as the shared foundation. PR #30 (`664f630`) integrates UX-004's accepted `internal/email/replycontent/model.go` schema and 22 fixtures, unblocking UX-005/006 after claim checks. PR #31 (`29da972`) adds the coordination protocol. PR #32 (`2191cb8`) integrates date normalization/NULL-subject fixes; UX-001 remains in Review for live-device and remaining header-refresh/exclude-mode work. UX-016 and UX-019 are available after the integrated UX-002 contract.

**Parser integration checkpoint:** PR #33 merged first as `74ed7ff`, followed by stacked PR #34 as `7436026`. The accepted APIs are `replycontent.ExtractPlain(body) Result` and `replycontent.ExtractHTML(body) Result`; use their merged schema, warnings and fixtures. UX-006 has a real shared-helper dependency on UX-005. UX-007's service/cache dependencies are now UX-002/005/006: it processes existing bodies and explicitly fetches through the current body service, without depending on unresolved virtual-folder refresh UI or live mailbox checks. Those UX-001 gates remain in force for their own work and release verification.

---

## 1. Accepted product decisions

- Personal multi-account mail is the product. AI writing, shared inboxes, team chat, meetings and collaborative documents are outside this plan.
- The main reader offers **Conversation / Traditional**. Conversation means extracting the new contribution from each email, rather than redisplaying nested history. Traditional shows full emails. Original content is always recoverable per message.
- All Inboxes belongs **inside the Virtual folders card**, alongside saved cross-account views. It is a built-in view of enabled accounts' Inbox folders, resolved dynamically.
- Folder navigation is available on demand. A visible Folders control opens a drawer with a Keep open option; the normal layout need not permanently allocate a folder column.
- Cards can be reordered, expanded/collapsed, hidden/restored and have entries moved between them. Organizing cards never moves messages between mailboxes.
- The quick-access rail holds chosen folders/views with Cmd+1…0 on macOS and Ctrl+1…0 elsewhere. Contacts/Calendar shortcuts default off for new layouts and can be restored in Preferences.
- Focus mode shows **only the open thread**, retaining its reply/actions. Exiting restores the previous view state.
- Actions are available beside messages and on list rows, with a bulk-selection bar. Mouse actions and shortcuts are equally supported.
- Gmail keyboard defaults, native platform shortcuts, Finnish keyboard layouts, command-panel discovery and shortcut tooltips all matter.
- Provide inline replies, reusable snippets/templates, visible sender/recipient controls, expandable full compose and durable drafts.
- Preserve contextual signatures, critical E2EE Send Later, the always-on helper, cross-account moves/filters, multiple windows and ICS handoff as existing product goals.
- External agents must be able to search/read structured mail and invoke authorized application operations. Summarization can be performed by the user's chosen external tool.
- Routine sync fetches headers/flags. Bodies and attachment bytes remain on demand, except explicitly opted-in folder body-download preferences. A body-download opt-in does not authorize attachment downloads or unlimited historical backfill.

### Working defaults for implementation

These are reversible defaults selected to make the plan executable; they are not claims that every detail was separately approved.

- Conversation is the initial reader preference; remember a user's Traditional choice. Start with compact full-width cards suited to long mail, tables and code.
- Keep thread grouping as the initial list organization. People grouping is a later, independent option.
- Focus is temporary per window. Card membership, rail pins and Keep open are persisted user layout preferences; account-tree expansion is separate.
- Reply Later follows the core reader/navigation/API work. It must not delay shipping the virtual-folder repair.
- Optional virtual-folder tabs, hiding empty views, sender bundles and scheduled presentation of incoming mail are later decisions. Empty views remain discoverable, particularly while diagnosing population failures.
- Existing default button sets can be tuned after real use. Avoid putting every action permanently on every row.
- Create/rename user cards is included; the four suggested starting cards are Everyday, Later, Virtual folders and Starred folders.
- No paid or hosted agent gateway is required for the first API. The Mac Studio Send Later helper remains a separate endpoint/trust concern.

## 2. Four independent choices

| Choice | Values | Meaning |
| --- | --- | --- |
| Scope | All Inboxes, real folder, saved virtual folder, search | Which messages are eligible |
| Grouping | Threads; People later | How eligible items are organized |
| Reader | Conversation or Traditional | How an opened thread is rendered |
| Layout | List + reader, folder drawer/docked panel, thread Focus | Which panels are visible |

Changing reader format must not change thread membership, flags, account identity or scope. People grouping must preserve topics/thread boundaries. Multiple accounts may contain identical Message-IDs or different copies of one email: keep account-qualified records and mutation targets even when a presentation deduplicates known copies.

Scope membership and conversation context are related but different: an Inbox result can open related Sent messages to make a thread understandable. Return both the messages matching the view and any additional thread context. Default actions must clearly identify their message targets; an expanded context message must not silently broaden an archive/delete operation.

### Reader and interaction behavior

1. Opening a thread shows known messages in chronological order and selects a useful unread/latest anchor.
2. Already fetched content is usable offline. Unfetched bodies show explicit placeholders; choosing to read/fetch them uses bounded existing fetch operations.
3. Each message has attribution, timestamp, source account where relevant, attachments, nearby actions and Show original.
4. Conversation mode collapses quotations and signatures only when supported by extraction evidence. Inline replies, code, tables and meaning-bearing context survive. Unknown/quote-only cases fall back to complete content.
5. Traditional mode uses the current full-email renderer. Neither mode changes the stored original or encryption/authentication banners.
6. Switching reader mode, opening/closing the drawer and entering/exiting Focus preserve draft, thread selection and scroll/focus context.
7. Marking a selected message read does not make its row vanish under the pointer in an unread view. Preserve the current interaction anchor until the user leaves/explicitly refreshes; reconcile counts and next navigation consistently.
8. Menus/dialogs take Escape first; then drawer/focus recovery follows existing context. Hidden controls have a keyboard-accessible recovery path.
9. Inline reply expands to the full composer using the same draft reference. Sender identity and recipients stay visible; avoid creating a competing draft/SMTP implementation.

## 3. Reuse and integration evidence

The following describes inspected code and candidate branches, not release status.

| Existing work | Reuse / action |
| --- | --- |
| `frontend/src/lib/actions/{types,target,listActions}.ts` and the shortcut registry exist on surveyed main | Extend explicit targeting and existing executors. Do not assign a second implementation of P-029 because an old board says Review. |
| Thread/message Focus modes already exist in `frontend/src/App.svelte` | Extend restoration/coverage rather than replace the focus architecture. |
| `internal/virtualfolder`, `internal/message/virtual_folder_queries.go`, `app/virtual_folder.go` | Existing cross-account criteria are the starting point; unify/extend query behavior carefully. |
| Pins/order/hide settings exist from P-007/P-008 | Migrate persisted layouts and preserve account-tree state. |
| Empty-folder patch `57e9c17`; coordinator copy `e1b1c90` on `integration/p037-review` | Provenance and live-device verification are UX-001. This patch was not on surveyed main; do not report it shipped. |
| Local navigation candidate `agent/P-038-sidebar-groups-rail`, implementation `b410f22`, migration `3d25d0e`, last ledger `d0004e5` | Reuse suitable pieces after review. It predates the final drawer/All Inboxes/typed-entry decisions and must be adapted. |
| Main P-038 deletion fix was merged in PR #27 at `8a7a2a8` | Preserve app ConfirmDialog and keyboard reachability. The local navigation P-038 is a different task; blind branch integration can regress this fix. |
| P-039 follow-up `agent/P-039-virtual-folder-followup`, handoff `953f4d7` | Records absent released patch and lack of live DB here. Fresh diagnostics are required on the actual mail device. |
| `internal/core/api/v1` / `internal/extensions/mail/api.go` | In-process extension interfaces, not proof of an authenticated external API. Adapt underlying operations; do not relabel them as completed external endpoints. |
| Draft, identity, snippets, compose and confirmed cross-account move paths already exist | Reuse them for inline reply and agent operations. Existing P-015/P-020 cover contextual signatures/full templates. |
| P-011 scheduled-send work is preserved remotely on `codex/p011-local-main-backup-20261026` through `cba3457`, as well as earlier integration branches | All 10 divergent local-main commits are retained. Its security/live-delivery release gates remain; it is not part of the reviewed origin/main. Advertise scheduling API capabilities only when the implementation is integrated and enabled. |

Historical P-026/P-036/P-037/P-038 names collided across independently edited boards. Identify old work by descriptive title **and branch/commit**. The local `agent/P-036-folder-body-prefetch` worktree carries a planning checkpoint rather than an integrated content-policy feature; use UX-040/041 for its backend/UI follow-through. New implementation tasks use the dedicated `UX-001…UX-041` namespace. Their live owner/status is in `TASKS.md`; this plan carries specifications, not a second claim ledger.

## 4. Shared extraction and content model

Implement extraction in Go so UI and external tools receive the same result. Generic article readability alone cannot define the new portion of an email. Use conservative email-specific segmentation and preserve the original decoded body/source.

Illustrative content-service envelope using UX-004's integrated extraction field names; UX-007 defines the enclosing response. UX-002 remains authoritative for shared references, scope and completeness, and `internal/email/replycontent/model.go` for the extraction result:

```json
{
  "message": {"accountId": "account-a", "messageId": "local-message-id"},
  "contentState": "cached",
  "extraction": {
    "version": "reply-v1",
    "status": "extracted",
    "blocks": [
      {"id": "b1", "kind": "authored", "format": "text", "content": "Tuesday works for me."},
      {"id": "b2", "kind": "quoted", "format": "text", "content": "Could we meet Tuesday?"}
    ],
    "warnings": []
  },
  "originalAvailable": true,
  "attachmentsState": "metadata-only"
}
```

- Block kinds: authored, quoted, signature, unknown. Extraction status: extracted, partial, fallback, unavailable.
- Keep source-to-block attribution and original access; a false-positive removal is a correctness defect, even if the result looks clean.
- Parse HTML structurally using existing libraries; carry CID/image relationships and sanitized formatting. No remote requests during extraction.
- Sanitization/iframe isolation and blocked remote images apply to derived content as well as originals.
- Cache only derived content, keyed by message reference/body hash/extractor version, with eviction. Default new caches to bounded memory; do not duplicate decrypted mail on disk.
- Bound input, DOM depth, processing work and concurrent jobs. Cancel work when the selected thread changes.
- Load long conversations incrementally and virtualize/mount content as appropriate. A thread summary request must report unavailable messages rather than claim completeness.

## 5. Application operations and agent API

Every feature needs an operation contract that can be used from the GUI or an authorized external client. Read access must be possible without opening a particular GUI panel.

### Initial transport and access

- Opt-in authenticated HTTP on a loopback address; CLI and a stdio MCP adapter share its service/client contracts.
- Existing OS credential storage protects credentials. Authorize client capabilities once, support revocation, and enforce them on every operation.
- Separate read metadata, read bodies, fetch remote content, attachment access, draft write, mail mutation, send and schedule capabilities.
- Read defaults to cached data and has no flag/notification side effects. Explicit fetch is separately authorized and bounded.
- A native app API token must not make the service accessible to arbitrary browser origins or LAN clients. Validate origin/host as applicable; no wildcard CORS.
- Preserve account/source attribution, stable cursors, revisions, cancellation, response-size bounds and machine-readable errors.
- Mail content is untrusted source material. Return it as content with attribution, never as system/tool instructions. No shell execution based on an email body.
- All writes reuse existing mail/draft/scheduler services. Provide precise targets and operation receipts; use idempotency and revision checks where retries could duplicate sends or lose draft edits.
- Return capability unavailable/unsupported honestly. A scheduled-send stub or ambiguous SMTP result must not produce a false success.
- The user may choose an external LLM provider. This architecture adds no mandatory AI processing or automatic mail upload.

### Endpoint scope (proposed v1)

| Endpoint family | Examples | Required behavior |
| --- | --- | --- |
| Discovery | `GET /v1/capabilities`, `GET /v1/accounts` | Version, enabled features, authorized capabilities and account metadata |
| Scope/navigation | `GET /v1/folders`, `GET /v1/views`, view CRUD/layout operations | System/saved scopes, supported criteria, presentation metadata; no server moves for layout edits |
| Search | `POST /v1/search` | Scope/query/cursor/limit, local versus server mode, completeness and partial errors |
| Threads | `GET /v1/threads/{ref}` | Message/source IDs, participants/dates, matching versus context messages, authored blocks and missing-body markers |
| Content | `GET /v1/messages/{ref}/original`, `POST /v1/messages/{ref}/fetch` | Explicit original/fetch policy, bounded data and cancellation |
| Attachments | list metadata, explicit fetch/open/save operations | Known/unknown metadata, source-message links and no eager byte downloads |
| Draft/reply | draft create/read/update, reply preparation | Exact sender/recipient/message references, draft revisions, same compose service |
| Mail actions | `POST /v1/actions` | Validated action ID, explicit targets, request/operation ID, result/undo metadata |
| Send Later | create/list/cancel/reschedule/status | Gate on integrated P-011; preserve existing E2EE and uncertain-delivery rules |
| Reply Later | list/add/remove queue items | Persistent personal queue separate from mail flags/delivery scheduling |
| Optional GUI context | current scope/selection, open thread/view | Separate UI-control capability, available only with GUI context; normal reads work independently |

An example agent workflow is search → retrieve structured thread → explicitly fetch missing bodies if authorized → summarize with per-message references. No built-in AI writing UI is required for that workflow. CLI `--json` and MCP adapters must not invent different IDs, query semantics or authorization rules.

## 6. Reply Later and optional later workflows

Reply Later is an explicit queue of conversations the user intends to answer. It does not change mail folders, read flags or delivery time. Focus & Reply takes a stable session snapshot, preserves drafts/skips, and offers next/previous/skip/remove.

A draft or Send Later enqueue alone does not prove a reply was sent. Complete a queue item only on the linked send's confirmed accepted status; SMTP acceptance is not recipient delivery/read confirmation. Failed, cancelled or ambiguous sends keep the queue recoverable. New incoming mail must not unexpectedly replace/reorder the current session.

People view groups correspondence within a selected scope and retains subjects/topics. An attachment browser can work within a thread/person/view using known metadata; incomplete local indexing is visible. Sender bundles are optional bulk-triage presentation, separate from both actual email threads and navigation cards.

Per-sender notifications, contextual signatures, full templates, multiple windows and calendar recognition remain linked to their existing tasks. Do not create competing implementations here. Incoming-mail delivery schedules, AI filters and a new task-manager product are not implicitly approved.

## 7. Agent capability and token-conscious dispatch

These are task-risk recommendations, not measured model benchmarks or price claims. Use Standard speed when the runner offers it; speed is a runner setting, not a repository guarantee. No implementation agents are launched by this documentation task.

| Tier | Default assignment | Suitable work | Escalation |
| --- | --- | --- | --- |
| Bounded | Luna High | Fixed-schema UI wiring, copy/localization, fixtures, thin adapters with accepted contracts | Escalate to Sol 6.1 Medium if the contract changes, cross-component state becomes unclear, or one focused attempt fails to resolve the issue |
| Integration | GPT-6.1 Sol Medium | Query/service wiring, Svelte state integration, migrations with known invariants, draft reuse, normal adapter work | Escalate to Sol 6.1 High for ambiguous data semantics, races, security or repeated unexplained failures |
| Sensitive | GPT-6.1 Sol High, followed by independent review | Extraction fidelity, HTML safety, API authorization, action target correctness, send/draft concurrency | Use a stronger available reviewer such as GPT-6 Astra High only if concrete unresolved architecture/security evidence justifies it; broad delegation needs coordinator authorization |

There are 9 bounded tasks, 19 integration tasks and 13 sensitive tasks. Task effort is separate from model reasoning effort: a small security patch can require a stronger model than a large amount of straightforward UI copy.

### Working protocol

1. Read only `AGENTS.md`, the relevant roadmap section, the selected task section, its explicit predecessor handoffs and named source files.
2. Verify predecessors are **integrated on the chosen base**, not merely implemented elsewhere. Claim the task in `TASKS.md` with owner/model/branch/worktree.
3. Write a short target/acceptance checklist. Add focused semantic regression coverage for risky behavior; avoid tests that only mirror source markup.
4. Implement the smallest accepted slice. Keep commits reviewable; proposed new modules are explicitly marked below.
5. Run focused checks, then the relevant build/type checks. Report exact commands/results and what requires the actual mail device.
6. Commit only your files and ledger update. Handoff: base SHA, commit SHA, changed files, accepted behavior, verification, limitations, next eligible tasks.
7. Do not merge/push other agents' branches or touch shared `main`. Integrator owns branch reconciliation and release. `Done` requires integration; compilation alone is not live verification.

Never put an entire mailbox dump, huge build logs or every historical branch in each agent prompt. For escalation, provide the failing case, expected behavior, observed result and the smallest relevant diff. Keep branch/task identity explicit because of past ID collisions.

### Verification command reference

Use the task's named packages/tests; proposed packages become runnable only after their task creates them. Examples from the repository root:

```sh
go test ./internal/virtualfolder ./internal/message
go build ./app
git diff --check
```

After the extraction/API tasks create their packages:

```sh
go test ./internal/email/replycontent
go test -race ./internal/automation/...
```

From `frontend/`, use installed worktree dependencies and focused existing/new Node test files. The normal type/build commands are:

```sh
npx svelte-check --tsconfig ./tsconfig.json
node --test tests/sidebarPins.test.mjs tests/sidebarLayout.test.mjs
npm run build
```

Run lint on changed sources using the repository configuration. `npm run check` also runs the shortcut-registry checks when needed. Expected: zero new type errors, relevant assertions pass, production build completes, and no whitespace errors. Record pre-existing failures explicitly; never hide them or call a GUI workflow verified from compilation alone. Meaningful behavioral tests should exercise state/contracts or user interaction, rather than assert only that source strings exist. Documentation-only work needs document/link/dependency validation, not application tests.

## 8. Delivery sequence and safe parallelism

- **First checkpoint:** UX-001 identifies and verifies the actual virtual-folder build. UX-002 can specify contracts independently.
- **Backend before presentation:** UX-003 query behavior and UX-004→UX-005→UX-006→UX-007 extraction precede their dependent UI. The parser order reflects the now-merged shared helpers.
- **Navigation lane:** UX-010→UX-011→UX-012→UX-013→UX-014. UX-015 then completes existing Focus behavior.
- **Actions/compose lane:** UX-016→UX-017 and UX-018; reuse P-029/P-030 and current draft behavior.
- **API lane:** UX-019 design follows UX-002, then UX-020 transport, UX-021 reads, UX-022 mail actions, UX-037 draft/send operations, UX-038 view/layout operations, UX-023 CLI and UX-024 MCP. UX-039 scheduling waits on integrated P-011. API contract design is early even though read implementation awaits extraction.
- **Core release:** UX-035 measurements and UX-036 integrated-device verification. Critical P-011 delivery/security work continues under its own owner and priority.
- **Folder content policy:** UX-040 backend policy follows UX-001/002; UX-041 adds its UI after the drawer is integrated. This is a separate requested feature; it does not hold the virtual-folder repair or core reader release hostage.
- **Later:** UX-025→UX-026→UX-027 Reply Later sessions, UX-028 related searches, UX-029 keyboard customization, UX-030→UX-031 People, UX-032 attachments, UX-033 columns and UX-034 bundles. UX-029 remains a core release dependency; its implementation can run as soon as its UI dependencies are ready.

Current implementation pickups: UX-002/004/005/006 are integrated; UX-007 service/cache is reserved for Claude. UX-016 action targets and UX-019 API design can proceed on separate files/worktrees after the dispatch/claim check. UX-001's date repair is integrated but its remaining live/refresh gate is not waived. Do not recreate accepted schemas from earlier illustrative examples or launch additional agents without authorization.

**File ownership boundaries:** One integrator owns `frontend/src/App.svelte` and `Sidebar.svelte` sequencing. Navigation tasks sharing those files run sequentially. Viewer timeline, toggle, action buttons, focus and inline reply also need sequential integration when they touch `ConversationViewer.svelte`. UX-018 and UX-037 share draft/compose services and need coordinated sequential integration. API transport and read/write adapters share auth/types; integrate contract first. P-030 owns the bulk-selection toolbar; UX-017 consumes it after integration and owns nearby row/message buttons. Migrations get coordinator-assigned numbers immediately before integration; never pick a number independently across worktrees.

## 9. Small implementation tasks

Each section is one reviewable task, not an instruction to implement the whole plan. For each task: reproduce/specify its expected behavior first, implement only that slice, run the listed focused checks, update changelog/docs if behavior changes, then commit and mark Review with remaining live checks. Where a test command names a proposed package, run it after creating that task's package. Do not run speculative commands against nonexistent paths and report them as verified.

<a id="ux-001"></a>

### UX-001: Virtual-folder and branch integration checkpoint

**Effort:** Medium. **Suggested agent:** Sol 6.1 Medium. **Dependencies:** none.

**Files:** `app/virtual_folder.go`; `internal/message/virtual_folder_queries.go`; `app/sync.go`; `docs/diagnostics/2026-10-26-ui-base.md (create)`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Inventory current main and candidate patches by commit; integrate/rework only the proven empty-folder fixes, preserving main's ConfirmDialog deletion fix and keyboard reachability. Record installed build SHA, known-cache query counts, real-label header refresh and empty/error distinctions. Release integration is coordinator-owned; unavailable live-device evidence stays explicitly pending.

**Verification:** go test ./internal/virtualfolder ./internal/message; go build ./app; frontend production build; actual-device smoke check.

<a id="ux-002"></a>

### UX-002: Shared view, thread and message-reference contract

**Effort:** Medium. **Suggested agent:** Sol 6.1 High. **Dependencies:** none.

**Files:** `internal/views/model.go (create)`; `frontend/src/lib/views/types.ts (create)`; `docs/api/view-contract.md (create)`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Specify independent scope, grouping, reader mode and layout. Use account-qualified thread/message references, matched message IDs versus expanded conversation context, stable cursors and body completeness. Cover All Inboxes, saved queries and single-account folders without changing threading algorithms.

**Verification:** Contract fixtures covering duplicate Message-IDs across accounts, scope membership and partial-body responses; review Go/TypeScript JSON parity.

<a id="ux-003"></a>

### UX-003: One saved-view query path

**Effort:** Medium. **Suggested agent:** Sol 6.1 Medium. **Dependencies:** UX-001, UX-002.

**Files:** `internal/views/query.go (create)`; `internal/message/virtual_folder_queries.go`; `internal/virtualfolder/model.go`; `internal/virtualfolder/store.go`; `internal/database/migrations.go`; `app/search.go`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Extend existing criteria with saved search expressions using the common query contract. List/search/count/unread badges agree, dates use latest activity, and exclusions work across accounts with archives eligible. Separate cached from server-backed search and report completeness; never fetch bodies merely to evaluate a view. A new general Boolean filter language/editor is deferred.

**Verification:** `go test ./internal/message ./internal/virtualfolder ./internal/views`; migration round-trip cases; explain query plan on synthetic large header sets.

<a id="ux-004"></a>

### UX-004: Extraction schema and representative fixture corpus

**Effort:** Easy. **Suggested agent:** Luna High. **Dependencies:** UX-002.

**Files:** `internal/email/replycontent/model.go (create)`; `internal/email/replycontent/testdata/ (create)`; `internal/email/replycontent/fixtures_test.go (create)`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Define authored/quoted/signature/unknown blocks, source references, extractor version and explicit fallback status. Add synthetic Gmail, Outlook, Apple Mail, plain text, Finnish/English reply markers, bottom/inline replies, tables, code, quote-only and malformed messages. Include expected text that must survive; use no private mail.

**Verification:** go test ./internal/email/replycontent; fixture/schema review by Sol before parser implementation.

<a id="ux-005"></a>

### UX-005: Conservative plain-text reply extraction

**Effort:** Medium. **Suggested agent:** Sol 6.1 High. **Dependencies:** UX-004.

**Files:** `internal/email/replycontent/plain.go (create)`; `internal/email/replycontent/plain_test.go (create)`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Identify new text and repeated quotations using email-aware boundaries. Preserve interleaved replies, authored lines after quotes, code and ambiguous signature-like text. Return full-body fallback when uncertain or no reliable authored portion exists; never rewrite the stored original.

**Verification:** go test ./internal/email/replycontent -run Plain; semantic fixture assertions and bounded-input cases.

<a id="ux-006"></a>

### UX-006: HTML reply extraction preserving meaningful content

**Effort:** Medium. **Suggested agent:** Sol 6.1 High. **Dependencies:** UX-004, UX-005.

**Files:** `internal/email/replycontent/html.go (create)`; `internal/email/replycontent/html_test.go (create)`; `internal/email/sanitizer.go`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Parse DOM with the existing golang.org/x/net/html dependency; handle client quote containers, inline replies, lists, code, tables and CID references. Treat generic article Readability as insufficient for reply extraction. Keep source originals, explicit fallback, script isolation and remote-image blocking. Bound bytes/depth/work.

**Verification:** go test ./internal/email/replycontent ./internal/email; HTML attack and preservation fixtures; stronger review before integration.

<a id="ux-007"></a>

### UX-007: On-demand extraction service and bounded cache

**Effort:** Medium. **Suggested agent:** Sol 6.1 Medium. **Dependencies:** UX-002, UX-005, UX-006.

**Files:** `internal/email/replycontent/service.go (create)`; `internal/email/replycontent/cache.go (create)`; `app/message.go`; `internal/message/store.go`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Return extraction for already fetched bodies; explicit content fetch uses the current body-fetch path. Cache by account/message reference, body hash and extractor version with bounded eviction. Cancellation prevents stale results replacing a newly selected thread. Decrypted derived text follows existing encryption policy and stays memory-only unless durable storage is explicitly supported.

**Verification:** go test ./internal/email/replycontent ./internal/message; cancellation/hash-change/eviction checks; go build ./app.

<a id="ux-008"></a>

### UX-008: Conversation timeline component

**Effort:** Medium. **Suggested agent:** Sol 6.1 Medium. **Dependencies:** UX-007.

**Files:** `frontend/src/lib/components/viewer/ConversationTimeline.svelte (create)`; `frontend/src/lib/components/viewer/ConversationMessage.svelte (create)`; `frontend/src/lib/components/viewer/ConversationViewer.svelte`; `frontend/src/lib/components/viewer/EmailBody.svelte`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Render compact chronological message cards with sender, timestamp, origin identity, authored content and attachments beside their message. Keep quoted/signature blocks expandable, authentication/encryption banners visible, existing safe HTML handling intact, and unloaded/error states honest. Initial viewport targets first unread/newest relevant content without losing timeline context.

**Verification:** frontend type check and production build; component/behavior tests for quoted-only fallback, partial loading, unsafe HTML and long content; manual keyboard reading.

<a id="ux-009"></a>

### UX-009: Conversation/Traditional toggle and per-message original

**Effort:** Easy. **Suggested agent:** Luna High. **Dependencies:** UX-008.

**Files:** `frontend/src/lib/components/viewer/ReaderModeToggle.svelte (create)`; `frontend/src/lib/components/viewer/ConversationViewer.svelte`; `frontend/src/lib/stores/uiState.svelte.ts`; `internal/appstate/model.go`; `frontend/src/lib/i18n/locales/en.json`; `frontend/src/lib/i18n/locales/fi.json`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Expose an always discoverable reader-mode toggle, remember the preference, and add Show original on each card. Keep selected thread/message and scroll anchor when switching. Traditional uses the current full-email renderer; extraction uncertainty always has an immediate recovery path.

**Verification:** UI-state round trip; reader-switch behavior test; frontend build; manual repeated toggle and focus check.

<a id="ux-010"></a>

### UX-010: Typed navigation entries and card-layout migration

**Effort:** Medium. **Suggested agent:** Sol 6.1 High. **Dependencies:** UX-001, UX-002.

**Files:** `frontend/src/lib/sidebar/entries.ts (create)`; `frontend/src/lib/sidebar/groups.ts (create or adapt candidate)`; `frontend/src/lib/stores/uiState.svelte.ts`; `internal/appstate/model.go`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Represent individual system views, real folders and saved views as first-class entries, each with stable identity. Persist order/card membership/hidden/collapsed state; migrate P-008 pins/layout and any pre-release sidebar state. Preserve orphaned entries for recovery. Adapt useful code from agent/P-038-sidebar-groups-rail; do not import its obsolete layout or deletion code wholesale.

**Verification:** go test ./internal/appstate; frontend navigation migration tests including malformed layouts and old preference files.

<a id="ux-011"></a>

### UX-011: Folder drawer with Keep open

**Effort:** Medium. **Suggested agent:** Sol 6.1 Medium. **Dependencies:** UX-010.

**Files:** `frontend/src/lib/components/sidebar/FolderDrawer.svelte (create)`; `frontend/src/lib/components/sidebar/Sidebar.svelte`; `frontend/src/App.svelte`; `frontend/src/lib/stores/uiState.svelte.ts`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Default to an on-demand folder panel opened by a visible Folders control. Closing it restores focus and selection; Keep open docks it when space permits. Preserve All folders access, account-tree expansion and narrow-window operation. Escape closes the topmost transient surface before the drawer.

**Verification:** frontend type check/build; drawer open/close/dock state tests; keyboard-only and narrow-window walkthrough.

<a id="ux-012"></a>

### UX-012: Card organization and entry moves

**Effort:** Medium. **Suggested agent:** Sol 6.1 Medium. **Dependencies:** UX-011.

**Files:** `frontend/src/lib/components/sidebar/SidebarCard.svelte (create)`; `frontend/src/lib/components/sidebar/Sidebar.svelte`; `frontend/src/lib/sidebar/groups.ts`; `frontend/src/lib/components/settings/SidebarLayoutSettings.svelte`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Create/rename/reorder/collapse/hide/restore cards and move individual special folders, saved views and real-folder shortcuts between them. Provide keyboard alternatives to drag/drop. Default cards: Everyday, Later, Virtual folders, Starred folders. All operations change presentation, never mailbox contents.

**Verification:** navigation transformation tests; frontend build; manual keyboard reorder, hide/restore and move-entry checks; verify no mail mutation calls.

<a id="ux-013"></a>

### UX-013: All Inboxes inside the Virtual folders card

**Effort:** Easy. **Suggested agent:** Sol 6.1 Medium. **Dependencies:** UX-003, UX-012.

**Files:** `app/unified.go`; `app/virtual_folder.go`; `frontend/src/lib/components/sidebar/Sidebar.svelte`; `frontend/src/lib/stores/virtualFolders.svelte.ts`; `frontend/src/lib/views/types.ts`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Add a stable built-in All Inboxes entry in the Virtual folders card, resolving enabled accounts' Inbox folders dynamically. Show it once, use shared query/status semantics, support pinning and card moves, and prevent deleting its system definition. User-created virtual folders remain editable/deletable.

**Verification:** Go shared-query tests; navigation tests for account add/disable and no duplicate All Inboxes; frontend build.

<a id="ux-014"></a>

### UX-014: Quick-access rail and Cmd/Ctrl+1…0

**Effort:** Easy. **Suggested agent:** Luna High. **Dependencies:** UX-010, UX-013.

**Files:** `frontend/src/lib/components/rail/ExtensionRail.svelte`; `frontend/src/lib/sidebar/pins.ts`; `frontend/src/lib/keyboard/registry.ts`; `frontend/src/lib/components/settings/SidebarLayoutSettings.svelte`; `docs/KEYBOARD_SHORTCUTS.md`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Pin/reorder real folders, saved views and All Inboxes in the rail; first ten positions map to 1…9,0. Contacts/Calendar rail preference defaults off for new layouts and preserves deliberate migrated choices. Hints show view names and platform keys. Do not intercept text editing, compose input or IME composition.

**Verification:** frontend shortcut/target tests; build; actual Finnish Mac and non-Mac key-event checks.

<a id="ux-015"></a>

### UX-015: Complete thread Focus mode and restore behavior

**Effort:** Easy. **Suggested agent:** Sol 6.1 Medium. **Dependencies:** UX-008, UX-011.

**Files:** `frontend/src/App.svelte`; `frontend/src/lib/components/viewer/ConversationViewer.svelte`; `frontend/src/lib/stores/keyboard.svelte.ts`; `frontend/src/lib/keyboard/registry.ts`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Extend the existing focus implementation to show only the open thread with its actions/reply box, hiding rail/drawer/list. Preserve previous layout, selected row, scroll and focused pane; nested menus/dialogs consume Escape first. Keep mode temporary per window. Reuse existing Shift+F behavior after auditing conflicts.

**Verification:** focused state-transition tests; frontend build; real iframe focus/Escape, draft and layout restoration walkthrough.

<a id="ux-016"></a>

### UX-016: Shared explicit action targets for every surface

**Effort:** Medium. **Suggested agent:** Sol 6.1 High. **Dependencies:** UX-002.

**Files:** `frontend/src/lib/actions/types.ts`; `frontend/src/lib/actions/target.ts`; `frontend/src/lib/actions/listActions.ts`; `app/actions.go`; `app/undo.go`; `internal/operations/mail.go (create)`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Audit and extend the existing P-029 action layer rather than replace it. Distinguish message, displayed matched-message set and explicit whole-thread targets. Carry account-qualified references to backend operations with validation, event emission and existing undo/sync. A thread ID must never be passed as a message ID; preserve copy-before-delete moves.

**Verification:** existing action/target tests; Go operation target and cross-account failure tests; review destructive-action semantics.

<a id="ux-017"></a>

### UX-017: Nearby list/message buttons and selection toolbar

**Effort:** Easy. **Suggested agent:** Luna High. **Dependencies:** UX-008, UX-016, P-030.

**Files:** `frontend/src/lib/components/list/ConversationRow.svelte`; `frontend/src/lib/components/viewer/ConversationMessage.svelte`; `frontend/src/lib/components/kit/ActionBar.svelte (inspect after P-030)`; `frontend/src/lib/components/list/MessageList.svelte`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Add list buttons on hover and keyboard focus, message-header Reply/Reply all/Forward plus context-appropriate mail actions, and More overflow. Consume P-030's integrated stable selection bar rather than reimplement it. Shortcut hints share the registry. Buttons do not open the row, move it under the pointer, or steal focus on appearance; archive never implicitly marks read.

**Verification:** action target behavior tests; frontend build; pointer/keyboard/touch and unread-filter selection-retention checks.

<a id="ux-018"></a>

### UX-018: Inline reply backed by durable drafts

**Effort:** Medium. **Suggested agent:** Sol 6.1 Medium. **Dependencies:** UX-008, UX-016.

**Files:** `frontend/src/lib/components/viewer/InlineReply.svelte (create)`; `frontend/src/lib/components/viewer/ConversationViewer.svelte`; `frontend/src/lib/components/composer/Composer.svelte`; `frontend/src/lib/composerApi.ts`; `app/draft.go`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Reuse current compose/draft services for an inline Reply/Reply all box with visible sender identity and recipients, snippet insertion, attachments and full-composer expansion. Preserve the same draft across navigation and expansion; route send/send-later through the existing service when available. Consume P-020 full templates when integrated, without implementing its manager here. Do not build a second SMTP or signature path.

**Verification:** Go draft/identity regressions; frontend build; manual thread-switch/expand/reopen and multi-identity reply walkthrough.

<a id="ux-019"></a>

### UX-019: Versioned agent API and authorization design

**Effort:** Medium. **Suggested agent:** Sol 6.1 High. **Dependencies:** UX-002.

**Files:** `docs/api/agent-v1.md (create)`; `internal/automation/v1/types.go (create)`; `internal/automation/v1/errors.go (create)`; `internal/core/api/v1/mail.go (inspect)`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Specify versioned read/fetch/draft/mutate/send/schedule capabilities, stable scoped IDs, cursors, explicit completeness, bounded responses, idempotency and revision conflicts. Distinguish existing in-process extension interfaces from external endpoints. Define local authenticated HTTP plus thin CLI/MCP adapters, disabled until enabled, with credentials protected by existing platform storage. Document mail content as untrusted data.

**Verification:** contract examples for cached/fetched/partial/error and permission-denied responses; stronger architecture/security review; no network listener in this design task.

<a id="ux-020"></a>

### UX-020: Local API listener and capability enforcement

**Effort:** Medium. **Suggested agent:** Sol 6.1 High. **Dependencies:** UX-019.

**Files:** `internal/automation/server.go (create)`; `internal/automation/auth.go (create)`; `internal/automation/server_test.go (create)`; `app/background.go`; `app/app.go`; `app/shutdown_test.go`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Implement opt-in local-only listener, explicit client authorization, independent capability checks, credential revocation, input/body limits and clean shutdown. Prevent browser-origin/host confusion and accidental LAN exposure; log metadata without credentials/mail bodies. No wildcard CORS or credentials in query strings.

**Verification:** go test -race ./internal/automation/...; unauthorized/forbidden-origin/wrong-host/oversized-input/revoked-token/shutdown cases; independent security review.

<a id="ux-021"></a>

### UX-021: Read/search/thread/body/attachment API endpoints

**Effort:** Medium. **Suggested agent:** Sol 6.1 Medium. **Dependencies:** UX-003, UX-007, UX-020.

**Files:** `internal/automation/v1/read.go (create)`; `internal/automation/v1/read_test.go (create)`; `app/message.go`; `app/search.go`; `app/attachment.go`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Expose accounts/folders/views, paged search, scoped thread/message reads and bounded attachment discovery. Reads default to cached content; explicit fetch requires capability and reports partial/provider errors. Return authored blocks plus original access, account/source attribution and missing-content markers. Reading never changes flags or sends remote-image requests.

**Verification:** Go read-contract tests with fake providers; prove default no-fetch/no-read-flag side effects and cursor scope isolation.

<a id="ux-022"></a>

### UX-022: Mail-mutation API endpoints

**Effort:** Medium. **Suggested agent:** Sol 6.1 High. **Dependencies:** UX-016, UX-020, UX-021.

**Files:** `internal/automation/v1/actions.go (create)`; `internal/automation/v1/actions_test.go (create)`; `internal/operations/mail.go`; `app/actions.go`; `app/undo.go`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Expose archive/read/unread/star/move/trash through validated shared operations and capabilities. Require account-qualified explicit targets and request/operation IDs; preserve events, existing undo and honest partial failures. Keep draft/send, view/layout CRUD and scheduling in UX-037/038/039 so this adapter stays bounded.

**Verification:** Go fake-provider action/capability/target tests; cross-account copy-failure preservation and partial-error/undo checks.

<a id="ux-023"></a>

### UX-023: Documented JSON CLI for agents and humans

**Effort:** Easy. **Suggested agent:** Luna High. **Dependencies:** UX-021.

**Files:** `cmd/postilaatikko-cli/main.go (create)`; `internal/automation/client/client.go (create)`; `docs/API.md (create)`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Thin CLI discovers the local endpoint, uses protected credentials, supports --json with stable stdout/exit codes and help examples. Initial commands search/thread/read/fetch; add write commands only after UX-022. Content is never interpolated into shell commands. Include a complete summarize-thread handoff example for an external LLM.

**Verification:** CLI tests against fake transport; JSON parsing/error/UTF-8 cases; go build ./cmd/postilaatikko-cli.

<a id="ux-024"></a>

### UX-024: MCP adapter using the same API contract

**Effort:** Medium. **Suggested agent:** Sol 6.1 Medium. **Dependencies:** UX-021, UX-023.

**Files:** `cmd/postilaatikko-mcp/main.go (create)`; `internal/automation/mcp/ (create)`; `docs/MCP.md (create)`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Provide stdio MCP tools/resources for search, threads, fetch and later authorized writes. Reuse HTTP service handlers/client and capabilities; no duplicate provider logic. Add pagination and truncation markers, cancellation and tool schemas. Return email as source content, never agent instructions. Select and document a maintained protocol implementation during this task.

**Verification:** MCP schema/protocol smoke checks with a local test client; transport cancellation and error mapping tests; document no unsupported protocol assumptions.

<a id="ux-025"></a>

### UX-025: Durable Reply Later queue operations

**Effort:** Medium. **Suggested agent:** Sol 6.1 Medium. **Dependencies:** UX-002, UX-016.

**Files:** `internal/replylater/store.go (create)`; `internal/undo/reply_later.go (create)`; `internal/undo/serialize.go`; `internal/database/migrations.go`; `app/reply_later.go (create)`; `app/app.go`.

**Deliverable / acceptance:** Persist stable conversation references and queue membership/order across accounts. Add/remove/list are idempotent and queue membership changes participate in the normal durable undo flow. Marking Reply Later never changes Inbox membership/read status or schedules delivery. Queue membership is explicit and is not inferred from provider replied/unreplied flags. Preserve missing/stale references for the UI to report and migrate safely. UX-027 owns completion semantics after sending; this task never removes queue entries implicitly.

**Verification:** go test ./internal/replyqueue; migration/restart/account-disable and duplicate enqueue cases.

<a id="ux-026"></a>

### UX-026: Reply Later entry, buttons and API tools

**Effort:** Easy. **Suggested agent:** Luna High. **Dependencies:** UX-012, UX-017, UX-021, UX-025.

**Files:** `frontend/src/lib/components/sidebar/Sidebar.svelte`; `frontend/src/lib/components/viewer/ConversationMessage.svelte`; `frontend/src/lib/stores/replyQueue.svelte.ts (create)`; `internal/automation/v1/reply_queue.go (create)`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Expose queue add/remove beside messages, a moveable system view and count, plus the same operations to authorized agents. Add quick filters for unread messages and explicit Reply Later queue membership within the current folder. Distinguish Reply Later, Snooze and Send Later in names and effects. Show empty/loading/error states and recover hidden navigation. The queue is a normal mail view that can be opened in a tab/window; it does not need a modal or separate window of its own.

**Verification:** queue/API adapter tests; frontend build; restart and keyboard queue management walkthrough.

<a id="ux-027"></a>

### UX-027: Focus & Reply session

**Effort:** Medium. **Suggested agent:** Sol 6.1 High. **Dependencies:** UX-015, UX-018, UX-026.

**Files:** `frontend/src/lib/components/viewer/ReplySession.svelte (create)`; `frontend/src/lib/stores/replySession.svelte.ts (create)`; `app/reply_queue.go`; `internal/replyqueue/store.go`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Open a stable snapshot of queued conversations with Reply/Skip/Remove and next/previous navigation. New mail cannot interrupt/reorder the session; preserve skipped items and drafts. A successful immediate send completes the queue item; a draft or scheduled-send enqueue alone does not. Delivery ambiguity/failure must not silently remove work.

**Verification:** queue/session transition tests including new mail, duplicate send events, send failure and scheduled send pending; manual interruption/restart review.

<a id="ux-028"></a>

### UX-028: Related searches and Save as virtual folder

**Effort:** Easy. **Suggested agent:** Luna High. **Dependencies:** UX-003, UX-013.

**Files:** `frontend/src/lib/components/viewer/ConversationViewer.svelte`; `frontend/src/lib/components/list/MessageList.svelte`; `frontend/src/lib/components/sidebar/VirtualFolderDialog.svelte`; `frontend/src/lib/stores/virtualFolders.svelte.ts`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Offer Search from/to for address chips and a Save view action for active queries. Preserve account/folder scope and status/date criteria. Display whether search is local/partial/server-backed; saved views do not silently shrink to Inbox. Optional tab surfacing remains a separate later decision.

**Verification:** query round-trip tests; frontend build; combined unread/folder/date saved-search walkthrough.

<a id="ux-029"></a>

### UX-029: Keyboard preferences and Finnish layout support

**Effort:** Medium. **Suggested agent:** Sol 6.1 Medium. **Dependencies:** UX-014, UX-015, UX-017.

**Files:** `frontend/src/lib/keyboard/registry.ts`; `frontend/src/lib/keyboard/shortcuts.ts`; `frontend/src/lib/keyboard/format.ts`; `frontend/src/lib/components/settings/KeyboardSettings.svelte (create)`; `docs/KEYBOARD_SHORTCUTS.md`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Keep Gmail defaults, platform-native shortcuts and existing command-panel binding; add overrides with conflict detection and reset. Registry drives buttons/tooltips/command panel/reference/API action metadata. Respect actual Finnish character keys, modifiers, composition and editable fields; verify reader iframe forwarding.

**Verification:** shortcut registry/override tests; frontend type check/build; actual Finnish macOS keyboard and another platform walkthrough.

<a id="ux-030"></a>

### UX-030: People grouping query and identity rules (later)

**Effort:** Medium. **Suggested agent:** Sol 6.1 High. **Dependencies:** UX-003, UX-021.

**Files:** `internal/views/people.go (create)`; `internal/views/people_test.go (create)`; `internal/message/store.go`; `internal/automation/v1/read.go`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Add optional per-scope grouping by correspondent with sent+received history and latest activity. Preserve topic/thread boundaries and account provenance; do not merge people solely by display name. Handle self identities, mailing lists, multi-recipient threads and aliases explicitly; surface unresolved identity instead of guessing.

**Verification:** Go correspondence fixtures for shared names, mailing lists, aliases and multiple sender identities; review grouping semantics.

<a id="ux-031"></a>

### UX-031: People view and related-thread navigation (later)

**Effort:** Medium. **Suggested agent:** Sol 6.1 Medium. **Dependencies:** UX-009, UX-030.

**Files:** `frontend/src/lib/components/list/PeopleList.svelte (create)`; `frontend/src/lib/components/viewer/CorrespondenceView.svelte (create)`; `frontend/src/lib/components/list/MessageList.svelte`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Add Threads/People grouping separately from Conversation/Traditional reader choice. Person selection shows paged topic-preserving correspondence and retains selected scope. Switching grouping keeps reader preference, mail flags and draft identity unchanged.

**Verification:** frontend behavior tests/build; manual thread/people/scope switch and long correspondence paging.

<a id="ux-032"></a>

### UX-032: Scoped attachment browser (later)

**Effort:** Medium. **Suggested agent:** Sol 6.1 Medium. **Dependencies:** UX-021, UX-030.

**Files:** `internal/message/attachment_store.go`; `internal/automation/v1/read.go`; `frontend/src/lib/components/viewer/AttachmentBrowser.svelte (create)`; `app/attachment.go`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Browse known attachments for a thread/person/view as list or grid with type/name/date/source-message links. Report incomplete metadata where bodies are uncached. Fetch bytes/previews on demand with cancellation, size caps and bounded thumbnail memory; reuse existing save/open actions.

**Verification:** attachment scope/pagination/no-auto-download tests; frontend build; large-file and incomplete-cache walkthrough.

<a id="ux-033"></a>

### UX-033: Column order, density and layout preferences

**Effort:** Easy. **Suggested agent:** Luna High. **Dependencies:** UX-009, UX-012.

**Files:** `frontend/src/lib/components/list/MessageList.svelte`; `frontend/src/lib/components/list/ConversationRow.svelte`; `frontend/src/lib/components/settings/SettingsDialog.svelte`; `frontend/src/lib/stores/uiState.svelte.ts`; `internal/appstate/model.go`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Add supported column show/hide/reorder and density preferences to the traditional list layout, plus compact/expanded headers where applicable. Do not create pretend columns in card layouts. Persist defaults and restore hidden fields; maintain anchored buttons and stable dialog geometry.

**Verification:** UI-state round trips; frontend build; resize and keyboard customization walkthrough.

<a id="ux-034"></a>

### UX-034: Optional sender bundles for bulk triage (later)

**Effort:** Medium. **Suggested agent:** Sol 6.1 Medium. **Dependencies:** UX-003, UX-016, UX-031.

**Files:** `internal/views/bundles.go (create)`; `frontend/src/lib/components/list/SenderBundle.svelte (create)`; `frontend/src/lib/actions/target.ts`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Opt-in grouping of related threads within the current view, never replacement threading. Expand to inspect exact items, show action counts/scope, and apply existing bulk actions only to explicit members. Default off; do not hide new messages or alter delivery schedules as a side effect.

**Verification:** bundle target tests with mixed scopes/statuses and partial results; frontend build; bulk-action/undo walkthrough.

<a id="ux-035"></a>

### UX-035: Bounded work, cancellation and performance measurements

**Effort:** Medium. **Suggested agent:** Sol 6.1 High. **Dependencies:** UX-007, UX-008, UX-011, UX-021.

**Files:** `internal/email/replycontent/cache.go`; `internal/views/query.go`; `frontend/src/lib/components/viewer/ConversationTimeline.svelte`; `docs/performance/conversation-ui.md (create)`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Measure current/candidate timing and peak/retained memory on synthetic large mailboxes and 10/100/1000-message threads. Fix only measured issues: bounded page/fetch/extraction work, offscreen rendering, cancel stale selection, indexed scope queries and bounded caches. Confirm no background bulk-body/attachment downloads. Record hardware/data/build, medians/p95 and residual limits.

**Verification:** repeatable Go benchmarks/query plans and frontend interaction recordings; compare baseline/candidate with same inputs; no absolute latency claims without measurements.

<a id="ux-036"></a>

### UX-036: Integration, accessibility, localization and release handoff

**Effort:** Medium. **Suggested agent:** Sol 6.1 Medium. **Dependencies:** UX-009, UX-013, UX-014, UX-015, UX-017, UX-018, UX-021, UX-029, UX-035, UX-022, UX-023, UX-024, UX-037, UX-038.

**Files:** `docs/plans/2026-10-26-conversation-navigation-agent-api.md`; `TASKS.md`; `ROADMAP.md`; `CHANGELOG.md`; `.github/workflows/release.yml (inspect)`; `frontend/src/lib/i18n/locales/en.json`; `frontend/src/lib/i18n/locales/fi.json`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Coordinator verifies the integrated core slice in the actual app: cached/uncached/offline mail, virtual unread stability, focus/drawer recovery, original fallback, multi-identity drafts, keyboard/iframe behavior and API permissions/completeness. Check en/fi keys and assistive labels. Record exact build SHA and unresolved gates; use existing GitHub DMG workflow after integration. Optional People/Reply Later/bundles do not block the core release.

**Verification:** focused Go/Node checks, Svelte type check/lint/build plus actual macOS smoke run; verify CI artifact contains app and Applications shortcut; never infer shipped from a task-branch commit.

<a id="ux-037"></a>

### UX-037: Draft/reply/send API and idempotent send receipts

**Effort:** Medium. **Suggested agent:** Sol 6.1 High. **Dependencies:** UX-016, UX-020, UX-021.

**Files:** `internal/automation/v1/drafts.go (create)`; `internal/automation/v1/drafts_test.go (create)`; `app/draft.go`; `app/compose.go`; `internal/operations/mail.go`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Expose draft create/read/update, reply preparation and explicit send through existing compose services. Validate sender/recipient/message scope, detect stale draft revisions and reuse a single idempotency/operation receipt for delivery-sensitive retries. Unknown SMTP outcome must not become automatic resend; accepted by SMTP does not mean read/delivered to recipient. Share exact behavior with inline/full compose.

**Verification:** Go draft/conflict/identity/capability/idempotency tests with fake SMTP; independent review of send failure and retry semantics.

<a id="ux-038"></a>

### UX-038: Saved-view and navigation configuration API

**Effort:** Medium. **Suggested agent:** Sol 6.1 Medium. **Dependencies:** UX-003, UX-010, UX-020, UX-021.

**Files:** `internal/automation/v1/views.go (create)`; `internal/automation/v1/views_test.go (create)`; `app/virtual_folder.go`; `internal/appstate/store.go`; `frontend/src/lib/stores/uiState.svelte.ts`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** Expose view CRUD and navigation card/pin/layout read/patch through shared stores. System All Inboxes is configurable in presentation but its definition cannot be deleted. Use revisions/patch semantics to prevent concurrent GUI/API writes overwriting unrelated preferences, emit refresh events, and preserve the no-mail-mutation boundary. Optional current-GUI-context/open-view operations use a separate capability and return unavailable when no GUI is connected.

**Verification:** Go view/system-entry/revision/conflict tests; GUI/API layout refresh smoke check; no-provider-mutation assertion.

<a id="ux-039"></a>

### UX-039: Scheduling API adapter after P-011 integration

**Effort:** Medium. **Suggested agent:** Sol 6.1 High. **Dependencies:** UX-020, UX-022, UX-037, P-011.

**Files:** `internal/automation/v1/scheduled.go (create)`; `internal/automation/v1/scheduled_test.go (create)`; `existing P-011 app/helper/scheduled-send operation files (locate on integrated base)`. Paths marked create are proposed; inspect the integrated base before creating an equivalent second module.

**Deliverable / acceptance:** After P-011 is integrated and its endpoint trust contract is reviewed, expose schedule/list/cancel/reschedule/status through that implementation. Preserve helper encryption/pairing and sending-credential isolation; do not introduce plaintext relay persistence, a second scheduler or unsafe retry logic. Gate capability on helper readiness; unavailable/ambiguous states are explicit.

**Verification:** P-011 focused queue/helper/security checks plus API capability/idempotency/error mapping tests; live authorized-device scheduled-send/cancel/reschedule review.

<a id="ux-040"></a>

### UX-040: Per-folder automatic body-download policy

**Effort:** Medium. **Suggested agent:** Sol 6.1 Medium. **Dependencies:** UX-001, UX-002.

**Files:** `internal/folder/content_policy.go (create)`; `internal/folder/content_policy_test.go (create)`; `internal/database/migrations.go`; `internal/sync/messages.go`; `internal/sync/fetch.go`; `app/sync.go`; `app/folder_content_policy.go (create)`.

**Deliverable / acceptance:** Persist account-qualified per-folder body-policy overrides and derive an effective default from sidebar visibility. Inbox and Starred special folders, plus user-pinned real folders shown in the sidebar, default to bounded body prefetch; folders outside the sidebar default to headers/flags only. An explicit per-folder on/off choice overrides the default across restarts. Removing a pin removes only the inherited sidebar default, not an explicit choice. Sync headers/flags everywhere; schedule bounded body-only retrieval for eligible new mail in enabled folders, retaining explicit-fetch priority and cancellation. Attachment bytes remain on demand. Enabling a folder never starts an unlimited history backfill; expose a separate bounded backfill operation only if specifically requested. Disabling stops new automatic work. Expose the same effective policy read/update operation for later API clients.

**Verification:** `go test ./internal/folder ./internal/sync`; policy migration/override/account-disable/concurrency/cancel cases and a fake-provider assertion that routine synchronization fetches no attachment bytes.

<a id="ux-041"></a>

### UX-041: Folder content-policy controls and status

**Effort:** Easy. **Suggested agent:** Luna High. **Dependencies:** UX-040. Existing folder context menus provide the account-scoped control surface; the drawer is not required.

**Files:** `frontend/src/lib/components/sidebar/FolderContextMenu.svelte`; `frontend/src/lib/i18n/locales/en.json`; `frontend/src/lib/i18n/locales/fi.json`; generated Wails bindings for the UX-040 policy methods.

**Deliverable / acceptance:** Add an understandable Automatically download message content toggle for real folders, with account scope and current effective state visible in the folder context menu. Explain that attachments stay on demand and old history is not automatically backfilled. Reuse UX-040 operations, preserve explicit overrides across restart, and provide a way to restore the inherited default. Do not add attachment prefetch or a second sync service.

**Verification:** frontend type check/build; en/fi key parity; policy-toggle state test and actual account/folder/restart walkthrough.

## 10. Release and verification gates

- The actual installed app's build SHA and enabled capabilities are visible in About/diagnostics. A branch commit, remote task status and release tag are distinct evidence.
- Virtual-folder list/search/count/unread behavior agrees on real synced headers; refresh is header-only. Distinguish loading/error/empty/offline/incomplete.
- Full-original access recovers every extraction uncertainty; authored inline content, tables/code, attachments and crypto status survive.
- New remote content fetches are explicit. Reading an API response has no hidden flag changes. Existing opt-in folder body policy is honored without automatic attachment downloads.
- Drawer/cards/rail and focused thread remain usable via pointer and keyboard, including Finnish layouts and iframe focus. No row disappears under the pointer merely because reading changes flags.
- Draft identity/recipients survive inline/full-compose transitions; concurrent edits and ambiguous sends remain recoverable. E2EE Send Later retains its separate release gates.
- Authenticated API capabilities, revocation, scope isolation, cancellation and byte limits are verified. GUI/CLI/MCP share semantics; partial data remains visibly partial.
- Performance records include baseline, hardware, data shape and timings/memory. Large lists do not mount/download every message or attachment.
- Actual macOS application smoke checks and an exact integration SHA precede promoting the GitHub-built DMG. Use the existing workflow; verify the packaged application and Applications shortcut. Unavailable checks are recorded, never silently waived.

## 11. Deferred decisions and evidence sources

Reversible defaults above keep the core work executable. Revisit only when needed: People alias-linking UI, virtual-folder tabs, whether to hide empty views, optional incoming-mail presentation schedules, remote agent access, a new general Boolean filter editor, and per-view reader overrides. No deferred decision blocks virtual-folder repair, the two reader formats or local structured thread access.

Inspiration sources reviewed on 2026-10-26; these are product references, not promises that Postilaatikko already implements their behavior:

- [Spike email app](https://www.spikenow.com/email-app/) and [Conversational email](https://www.spikenow.com/features/conversational-email/): readable conversational mail and optional full-original presentation.
- [HEY Focus & Reply](https://www.hey.com/features/reply-mode/): a personal Reply Later session, separate from delivery scheduling.
- [HEY agent interface](https://www.hey.com/agents/): external agent/CLI access.
- [Shortwave customization](https://www.shortwave.com/docs/guides/customize-your-shortwave-settings/): saved query views, optional sender grouping, layout and notification control.
- [Unibox](https://www.uniboxapp.com/): correspondence by person, sent/received history and attachment discovery.
- [MailMate introduction](https://manual.mailmate-app.com/introduction): Boolean smart mailboxes, saved searches, related searches, configurable columns and contextual signatures.
- [Superhuman shortcuts](https://help.superhuman.com/hc/en-us/articles/46005701270541-Keyboard-Shortcuts-in-Superhuman-Mail) and [international keyboards](https://help.superhuman.com/hc/en-us/articles/46005584339597-Shortcuts-for-International-Keyboards): learning shortcuts through actions and explicitly checking keyboard-layout assumptions.
