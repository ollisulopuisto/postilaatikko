# App Nap support (macOS)

Postilaatikko cooperates with App Nap instead of fighting it: when the
window is hidden and the app is idle, macOS may nap it — timers coalesce
and CPU is throttled, which is the battery saving App Nap exists for.

The operations that must not be throttled mid-flight hold an explicit
`NSProcessInfo` activity assertion (`internal/platform/activity*.go`), so
the app is *temporarily* ineligible for App Nap only while they run:

| Operation | Assertion | Why |
| --- | --- | --- |
| In-flight account sync (scheduler + manual) | `NSActivityBackground` | A napped sync would crawl or stall half-finished; between syncs the app naps again. |
| Undo Send countdown (`QueuePendingSend` → dispatch/cancel/expiry) | `NSActivityUserInitiatedAllowingIdleSystemSleep` | The countdown timer lives in the WKWebView and is throttled with the app; the 30 s dispatch must fire on time. |
| Outgoing SMTP send (`SendMessageWithDraftGuard`) | `NSActivityUserInitiatedAllowingIdleSystemSleep` | A dispatch can start while the window is hidden; the SMTP exchange must finish unthrottled. |

Deliberately *not* asserted:

- **IMAP IDLE / new-mail notification watches.** An armed IDLE watch
  holds an `ActivityBackground` assertion, because timely new-mail
  notifications are the default expectation and App Nap lowers the
  process's scheduling priority while the socket stays open. This
  deliberately limits the power saving: the app is un-nappable whenever
  at least one account is watching. The assertion is background-only —
  the display and the machine still sleep normally.
- **Send Later delivery.** Scheduled sends are delivered by the separate
  `postilaatikko-helper` process. App Nap's candidacy rules are
  app-centric (foreground, windows, audible), so the helper is not itself
  a candidate; the main app's nap state does not affect it.
- **Periodic sync ticks themselves.** A napped app's tickers coalesce,
  so a hidden, idle app syncs less often; it catches up on the next
  assertion, wake, or foreground. This is the intended saving.
- **Idle system sleep.** `AllowingIdleSystemSleep` and `Background`
  assertions never stop the Mac from sleeping on its normal schedule.

## Verifying

- `pmset -g assertions` while a sync or send runs shows an assertion with
  the matching reason ("Syncing email", "Sending email", "Sending email
  after Undo Send countdown").
- Activity Monitor's **App Nap** column shows **No** only while one of
  the operations above is active; **Yes** when the app is hidden and
  idle.

On Linux and Windows the manager is a no-op; behavior there is unchanged.
