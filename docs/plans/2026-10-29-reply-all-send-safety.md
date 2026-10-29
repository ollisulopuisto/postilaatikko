# Reply All and recipient repair

**Status:** Planned as UX-042. This is separate from the Reply Later queue.

## Goal

Make it difficult to reply to the wrong audience, and make it easy to correct
the recipient list before a queued send is handed to SMTP. The recovery path
covers both accidental Reply All and forgetting a recipient.

## Product behavior

- Show whether the draft is Reply or Reply All and summarize the actual To/Cc
  recipients before the Undo Send countdown finishes.
- During that countdown, offer **Edit recipients**. It cancels the pending send,
  restores the same draft (including body, attachments, sender identity and
  quoted content), and focuses To/Cc for correction. The user can then send
  again and receive a new countdown.
- Keep ordinary Undo Send available throughout the countdown.
- If SMTP delivery has begun or its outcome is unknown, say that the message
  may already have been delivered. Do not present recipient editing as recall.
- Do not infer “reply all” only from the number of recipients; retain the
  user's explicit reply mode and display the actual recipient list.

## Acceptance

- Reply and Reply All are visually distinct, with the destination recipients
  inspectable before delivery.
- Editing recipients cancels only a still-pending send and reopens the exact
  associated draft without losing or duplicating content or attachments.
- Retry creates a new send attempt and a fresh countdown; a repeated click or
  stale countdown cannot send twice.
- Once transport starts or returns an ambiguous result, the UI does not claim
  that Undo or recipient editing recalled the message.
- Keyboard, detached composer, localization and window restart behavior retain
  the same safety guarantees.

## Dependencies and capability

**Effort:** Medium. **Suggested agent:** Sol 6.1 Medium. Reuse the integrated
COMPOSER-UNDO 30-second pending-send flow, durable drafts and send receipts.
This requires careful state-machine review; provider recall is not assumed.

