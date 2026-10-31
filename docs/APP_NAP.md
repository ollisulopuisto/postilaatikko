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

- **IMAP IDLE / new-mail notification watches.** App Nap does not close
  sockets; it lowers scheduling priority, so a notification can arrive a
  few seconds late while napped, never lost. Asserting here would keep
  the app out of App Nap permanently and void the power saving.
- **Send Later delivery.** Scheduled sends are delivered by the separate
  `postilaatikko-helper` process, which is not an app and is never
  napped. The main app's nap state does not affect it.
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
