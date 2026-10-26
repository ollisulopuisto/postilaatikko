# Reader smoke test on a Mac (UX-008 and UX-009)

What the automated tests could not check: the real app in WKWebView, with real
mail. Run it on a build from `main` at or after `0ef36e4`, with an account that
has a few multi-message threads (Gmail, Outlook and Apple Mail replies if you
have them). It takes about ten minutes. Write down anything that differs from
"Expected", with the subject line of the thread and a screenshot.

## 1. Conversation mode (the default)

1. Open a thread with several messages and quoted replies.
   Expected: the thread opens at the first unread message (else the newest),
   with a peek of the card above it; no jump while the messages above load.
2. Look at one reply. Expected: only the new text is open; the quote is one
   line, "Show quoted text" plus the first words, closed.
3. Click it, then press Enter and Space with the keyboard on it. Expected: it
   opens and closes; the message card does not collapse.
4. Find a message with a signature (a `-- ` line, "Sent from my iPhone", or a
   Gmail signature). Expected: it is folded as "Show signature".
5. Find an inline reply (answers between quoted lines). Expected: order kept,
   your answers open, the quotes folded between them.

## 2. Nothing hidden by mistake

6. Open a message that only quotes (a forward, or a reply with no new text).
   Expected: the whole message shows, nothing folded.
7. Open a message you know has text under a `>` line that you wrote yourself.
   Expected: that text is visible (it may be marked as unclear, never folded).
8. On any split card press "Show original", then "Show new text only".
   Expected: only that card changes; the way back is always one click.

## 3. Reader mode toggle

9. Press Traditional in the toolbar. Expected: every message shows in full, the
   same thread stays selected, and the card you were reading stays where it was.
10. Press Conversation again, several times, then once with the keyboard
    (Tab to the toggle, Enter). Expected: focus stays on the toggle.
11. Quit and reopen the app. Expected: the last mode is remembered.

## 4. Images, links, print

12. Open a newsletter with remote images. Expected: images blocked until you
    choose Load images; then they load (also in a split message).
13. Click a link and a mailto link in a split message. Expected: as before.
14. Print a thread in each mode. Expected: what is on screen is printed (folded
    quotes are not).

## 5. Other message kinds

15. An S/MIME or PGP message. Expected: shows and decrypts as before, no split.
16. A message whose body has not been downloaded. Expected: "downloading"
    placeholder, then the message; no blank card.
17. A thread with 20 or more messages, then Expand all. Expected: usable, no
    long freeze (the splitter runs four at a time).

## Reporting

Paste the numbers that failed with a screenshot and the thread subject into the
chat with Claude; fixes go on a branch per finding. If every step passes,
`TASKS.md` can mark UX-008 and UX-009 verified on device.
