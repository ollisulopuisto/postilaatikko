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

### 4a. Forward layout: split bubbles, note on top (decided 2026-11-09)

Owner answers to the round-3 questions ("Axis 1, variant A", "Axis 2, variant a"):

- The forwarded part and the note are two bubbles, not one blob of text. The owner
  knows breaking the message up like this is unusual and chose it anyway.
- Top-posted: the note (new text) comes first, the forwarded original below it, in the
  composer and in the sent result. Top posting is the house style.
- The two parts must read as one message, not two. The treatment is under "Connecting
  the forwarded part and the note" below.
- A traditional composer stays available, see decision 8.

Build scope: the quote card and note bubble layout for Forward, and the same split
for replies that quote. Do not start the code until the owner says go.

Note: this was recorded from the owner's chat answer. The sketch page itself
could not be re-read when this was written, so the axis labels follow the HELD text
that was here before (Axis 1 is where the quote sits while composing, Axis 2 is the
sent order). Check the labels against the sketches before building.

### 5. Send Later: a dedicated, visible button

Send Later is its own button in the bar next to Send, not only a hold-menu, so the
feature is discoverable at a glance. Owner answer (2026-11-09): a clock icon, the way
Gmail does it, not a text button. It keeps an accessible name "Send later" and a
tooltip with the shortcut. Clicking it opens the quick-pick menu
(Tomorrow 8:00, Monday 7:30, Pick date...); holding Send keeps the same menu as a
secondary path, and full compose keeps parity.

Keyboard: Opt+Cmd+Enter opens the same menu and moves focus into the list of times.
Arrow keys move through the picks, plain Enter activates the focused one, Esc closes
the menu and returns focus to the text. Mouse and keyboard use one menu.

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
| Opt+Cmd+Enter | Send Later: open the time list with focus in it; arrows move, Enter activates |
| Esc or Cmd+Z | Undo Send during the countdown |
| Cmd+Shift+Enter | Release an already-pending send only |
| Cmd+N | New blank conversation |

Enter stays safe for editing because apps split the newline/send binding
differently; Cmd+Enter is the single send gesture, with Opt+Cmd+Enter as its
scheduled twin. There is no send-without-delay bypass key in the editor (owner
rule from the undo-send decision: "cos then you'll just always use that until it
bites you in the ass"); Cmd+Shift+Enter only releases an already-pending send.

### 8. Traditional composer alongside the bubble composer (decided 2026-11-09)

Owner: "we still should have a traditional composer, just like we have a traditional
thread view." The bubble composer (decisions 1-7) is the default. A traditional
composer stays as a second mode, the way the traditional thread view stays next to the
conversation view.

- Traditional mode is the classic mail form: To, Cc, Bcc, Subject and one body field
  with the quoted original below the note (top-posted, same as the bubble result).
- One draft store, one autosave, one Send and Undo Send path for both modes
  (COMPOSER-UNDO and the durable draft work). A draft opened in the other mode keeps
  its recipients, subject, text and attachments.
- A visible switch in the composer header toggles the mode for that draft, and a
  setting picks the default for new drafts. Per the visible-action rule the switch is a
  button, not only a shortcut.
- Send Later, attachments, Cmd+Enter send and the clock icon behave the same in both
  modes.
- Open for the owner: which mode Cmd+N opens by default, and whether Forward from the
  traditional thread view opens the traditional composer.

## Connecting the forwarded part and the note

Owner: "figure out a way, sorta like Solmu does, to visually connect the forwarded part
and the note part."

How Solmu does it (src/ui/conversation.rs, `quote_block`, `quote_colours`,
`forwarded_label` and `reply_strip` in the Solmu repo):

- The quoted part is not a separate bubble. It is an inset card inside the same
  bubble, above the message text, so the two can never drift apart or be read apart.
- The card is a rounded rectangle with a full-height bar on its left edge. The bar and
  the quoted sender's name take that sender's own colour (the same hue as their name in
  groups, kept readable on the bubble). The card fill is a light wash of the window
  colour, 35 percent, so it reads as a sunken area of the bubble. On an own blue bubble
  everything on the card goes white at lower opacity.
- The card shows the sender name (semibold) and a short summary (secondary colour).
  Clicking it scrolls to the original message.
- A forwarded message gets a small arrow plus the word "Forwarded" in italic at the top
  of its own bubble.
- While composing, the pending quote sits above the text field as the same card with a
  cancel control ("Replying to ..."), so the card looks the same before and after send.

Proposed Postilaatikko treatment. Postilaatikko's split-bubble decision means the two
parts are separate boxes, so the join has to be drawn on purpose:

1. One run. The note bubble and the forwarded bubble stack as a single run: tight
   spacing, joined inner corners, one shared tail and one timestamp and receipt at the
   end. Same rule Solmu uses for consecutive messages, so they cannot look like two
   messages.
2. Same card language. The forwarded part is a quote card in the Solmu style: left bar
   and sender name in the original sender's colour, a sunken wash, name and date as the
   header line (italic arrow and "Forwarded" label), subject and the first lines as the
   summary, expand to read the whole original. The note bubble's own colour is
   unchanged, so the card reads as belonging to it.
3. The bar runs through the join. The coloured bar starts at the note bubble's bottom
   edge and continues down the forwarded card, so the eye follows one line from note to
   original.
4. Same look in the composer. While composing, the forwarded card sits docked directly
   under the note field in the draft bubble with the same bar and wash, and a visible
   remove/switch control (the existing "switch to .eml attachment" menu). Nothing
   changes between draft and sent.
5. Click goes to the source. Clicking the card opens or scrolls to the original message
   in its thread, as in Solmu.
6. Traditional thread view and traditional composer show the same content as plain
   quoted text below the note, with the same bar in the margin.

Open for the owner before building: whether the bar should take the original sender's
colour (Solmu) or one fixed accent, and whether the card collapses long originals by
default.

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
