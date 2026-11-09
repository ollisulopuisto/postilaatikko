# Compose as a bubble: consolidated build spec (2026-11-09)

Owner decisions from three sketch rounds, consolidated into one build spec for the
compose PR. Documentation only: this PR adds no code. Implementation should be split
into PR-sized tasks in TASKS.md when claimed.

Sketch sources:

- Round 1 (variants A-D): <https://files.instinct.com/7sdnsrk66p3l-postilaatikko-compose-as-a-bubble>
- Rounds 2-3 (open questions, then decisions): <https://files.instinct.com/y0arlvda4hib-compose-bubble-open-questions>
- Earlier standing decisions: COMPOSER-UNDO (integrated in PRs #43-#45), the
  per-keypress autosave and 30-second Undo Send countdown, and P-011 Send Later.

## Settled decisions

### 1. Default composer: Variant A (docked bar)

The composer is a bar docked at the bottom of the conversation view. The bar holds
text only. To/Cc, subject, sender identity and attachments all live outside the
bubble: recipients and subject in the header strip (where a subject can still
change), attachments as chips in the bar.

Variant C's idea survives as the per-message reply: replying to a specific
mid-thread bubble anchors the draft under that bubble, indented, so the answer sits
next to what it answers. A stays the default; C is what a per-message reply does.

### 2. Brand-new mail: blank conversation view

Cmd+N (or the + in the rail) opens a real conversation view that starts empty: a
slim header strip holds To and Subject, the timeline below states this is where the
thread will grow, and the docked bar is focused and ready.

- The first keystroke autosaves the draft, so the half-written thread appears
  under Drafts before anything is sent.
- On send, the view becomes the thread in place: no jump, no new window.
- Rejected: a separate "new message" card in the inbox list. Writing always
  happens inside a conversation.

### 3. Attachments: drag-and-drop plus a visible button

Drop files anywhere on the conversation; while dragging, the timeline shows a
dashed drop veil, and dropped files land as chips in the bar, next to the text but
never inside it. A visible + button in the bar opens the same attach flow (see the
visible-action rule below).

### 4. Forward: an action on the bubble, never a composer mode

Each bubble's ... menu offers Reply, Reply All, Forward. Picking Forward opens the
blank conversation view from decision 2, with the original message as a quote card.

- The quote card is metadata, not text: it sits outside the bubble. Its own ...
  menu switches it to a real .eml attachment.
- Subject exists only in the header strip, and only where it can still change
  (new threads and forwards). Replies inherit it silently as "Re: ...".
- Forwarding never marks the original thread.

HELD (owner to decide after design exploration): the order of quote versus new
text, on two axes - where the quote card sits while composing (above the draft,
DM-app style, vs below it, mail style) and the order in the sent result
(top-posted: new text first, vs bottom-posted: original first), including whether
replies and forwards should differ. Variants, pros/cons and a recommendation are
on the round-3 sketches page. Do not build the Forward quote layout until the
owner picks; the rest of this section is settled.

### 5. Send Later: a dedicated, visible button

Send Later is its own button in the bar next to Send, not only a hold-menu, so the
feature is discoverable at a glance. Clicking it opens the quick-pick menu
(Tomorrow 8:00, Monday 7:30, Pick date...); holding Send keeps the same menu as a
secondary path, and full compose keeps parity.

- A scheduled draft sits in the timeline as a dashed bubble with its send time,
  exactly where it will land. It is editable until it fires: click it to change
  the text, the time, or to send now.
- One queue, two entry points: scheduling folds into the planned send-later
  relay, the same mechanism the Undo Send delay uses (P-011 heritage).

### 6. Undo Send: bubble state and toast together

During the 30-second countdown the just-sent bubble renders as "sending": a
draining line under the bubble and an Undo chip on it. The status-area toast stays
as the global control; the two views never contradict each other.

- The bubble's Undo chip, the toast, Esc and Cmd+Z all cancel and restore the
  draft with its text intact (per-keystroke autosave, COMPOSER-UNDO).
- When the line empties, the bubble locks into a solid sent bubble; there is no
  toast-only gap where the message looks sent but is still cancelable.
- Navigating away mid-countdown leaves the toast as the way back.

### 7. Key map

| Key | Action |
| --- | --- |
| Enter | New line (plain editing; Enter never sends) |
| Cmd+Enter | Send (starts the Undo Send countdown) |
| Opt+Cmd+Enter | Send Later (opens the schedule menu) |
| Esc or Cmd+Z | Undo Send during the countdown |
| Cmd+Shift+Enter | Release an already-pending send only |
| Cmd+N | New blank conversation |

Enter stays safe for editing because apps split the newline/send binding
differently; Cmd+Enter is the single send gesture, with Opt+Cmd+Enter as its
scheduled twin. There is no send-without-delay bypass key in the editor (owner
rule from the undo-send decision: "cos then you'll just always use that until it
bites you in the ass"); Cmd+Shift+Enter only releases an already-pending send.

## Visible-action rule (whole app, from 2026-11-09)

Every action must have a visible button or a menu item. A keyboard shortcut is an
accelerator, never the only way to reach an action. Applied to this spec:

| Action | Visible control |
| --- | --- |
| Send | Send button in the bar |
| Send Later | Send Later button in the bar |
| Forward (Reply, Reply All) | Bubble ... menu |
| Undo Send | Undo chip on the sending bubble + status toast |
| Attach | + button in the bar (drag-and-drop as the second path) |
| Schedule quick picks | Menu opened from the Send Later button |

This rule belongs with the repo's durable design rules and is recorded in
ROADMAP.md's product direction.

## Implementation notes for the follow-up code PRs

- Split into PR-sized tasks on the board before claiming; keep one task per
  branch per AGENTS.md.
- Compose state builds on the integrated durable-draft, revision-safe autosave
  and pending-send work (COMPOSER-UNDO, UX-037, HOSTED-SERVER-052); extend those
  paths rather than adding a parallel draft store.
- Related open work to coordinate with: UX-042 (Reply All recipient repair and
  editing the same draft during Undo Send), UNDO-SEND-TOAST-001 (silent
  countdown toast, in Review), P-011 Send Later release gates.
- Testing per repo norms: red-then-green regression tests, frontend suite,
  svelte-check, and Chromium actual-component checks for the bubble states
  (draft, scheduled, sending, locked-sent); native WKWebView verification
  remains a device gate.
