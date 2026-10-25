# Contextual toolbars, selection popovers and context menus — design proposal

- **Task:** D-001 (design only; no UI is implemented by this document)
- **Date:** 2026-10-25
- **Question from the owner:** "Would a Photoshop style floating toolbar be a good idea? What about contextual menus, e.g. if you highlight something first and then the relevant options pop up?"
- **Related work:** P-026 (shortcut registry, `?` overlay, per-action hints — done), P-027 (Ctrl/Cmd+P command panel and Gmail-standard single keys — done), P-012 (ICS handoff, done), P-022 (date/time recognition, unclaimed), P-018 (quick replies, done), P-010/P-019 (Gmail API search and native spam — done), P-020 (templates), P-015 (contextual signatures), P-014 (cross-account filters).
- **Surveyed revision:** `main` at `8d0eed4`. A first draft was written against `d84930b` and revised after P-026 and P-027 landed.

## 1. Recommendation in short

1. **Do not build a Photoshop/Figma-style floating or dockable tool palette.** Mail has no tool modes and no canvas. Every action applies to something that is already selected, so a free-floating palette would repeat the toolbars the app already has, take space in a window that can be 360 px wide, and help keyboard users not at all.
2. **Build contextual surfaces, in this order:**
   1. **A complete selection action bar for checked messages.** It sits in the list header and does not float. A partial bar already exists (translated since P-027) but offers only three actions.
   2. **One accessible context menu model, also opened from the keyboard,** shared by the message body (text and links), sender/recipient chips and attachments. The body menu today has no keyboard support.
   3. **A selection popover over message bodies.** It is a mouse affordance only. Its items are the same as the context menu's selection items, and it never takes focus.
   4. **A formatting bubble in the composer** when text is selected, mainly for the compact inline composer, which hides its toolbar.
3. **Base every surface on a shared action layer, and feed the existing command panel from it.** Each action definition refers to a P-026 registry id for its label, icon and shortcut. The P-027 command panel (Ctrl/Cmd+P) becomes one more consumer of the same definitions instead of keeping its own list. The panel is the keyboard-first way to reach any action from anywhere. Contextual surfaces show the subset that applies to what is selected, next to it. Nothing is defined twice.
4. **Defer** extension-contributed menu items and per-entity smart chips. **Reject** auto-translation, a pinned or draggable palette, and any surface that opens on keyboard selection or moves focus by itself.

The rest of this document gives the evidence, the evaluation, the building blocks and a phased plan.

## 2. What exists today (survey)

All line numbers refer to `main` at `8d0eed4`.

### 2.1 Fixed toolbars already present

| Surface | Where | Notes |
| --- | --- | --- |
| Conversation viewer header | `frontend/src/lib/components/viewer/ConversationViewer.svelte` ~1640–1765 | Reply, Reply all, Forward, (Move to inbox in Trash), Archive, Delete, Spam, Star, Read/Unread, Expand/Collapse all, Focus, Print. It is always visible for the open conversation. Since P-026, tooltips include the shortcut (`withShortcut`, for example 1646, 1691). |
| Composer formatting toolbar | `frontend/src/lib/components/composer/EditorToolbar.svelte` (`role="toolbar"` at 236–237) | Has an **Alt+T hint mode** (15–60): it labels each button 1–9/a–z, like Vimium. The compact composer hides the toolbar behind a toggle (`Composer.svelte` 94, 2161, 2227). |
| Composer footer | `Composer.svelte` ~2240–2251 | Attach, Quick replies dropdown (`QuickReplies.svelte`, P-018), formatting toggle in compact mode. |
| Message list "bulk" bar | `frontend/src/lib/components/list/MessageList.svelte` 2398–2415 | Shown when `checkedThreadIds.size > 0`. Since P-027 it is translated (`messageList.selectedActions`, `messageList.selectedCount`), and Archive and Close show their shortcuts in tooltips. It still offers only Done (mark read), Move to inbox (Trash only), Archive and Close: no Star, Spam, Delete, Move or Unread, and the buttons have no `aria-keyshortcuts` or roving focus. |
| Command panel (P-027) | `frontend/src/lib/components/command/CommandPalette.svelte`, `frontend/src/lib/stores/commandPalette.svelte.ts` | Ctrl/Cmd+P (and Ctrl/Cmd+Shift+P). A modal `role="dialog"` with a `listbox` (351, 388), registered with the dialog guard. See §2.9. |
| Row hover quick actions | `frontend/src/lib/components/list/ConversationRow.svelte` 529–564 | Done, Read toggle and Delete appear on hover (`group-hover`). They are mouse-only by design. |

### 2.2 Context menus

- **Message rows and viewer message cards** use `MessageContextMenu.svelte` (bits-ui `ContextMenu`). The action handlers are local to that component (~157–366) and the menu sits at 369–456. Since P-026 the items show `<ShortcutHint>` (for example 382, 387). Rows wrap themselves in it (`ConversationRow.svelte` 480–491). The viewer wraps each message card in it (`ConversationViewer.svelte` 1789 onward).
- **Keyboard invocation:** `App.svelte` ~1297–1330 maps the `ContextMenu` key and **Right Alt** to a synthetic `contextmenu` event on the focused row (`MessageList.openContextMenu`, 1890) or the focused message card (`ConversationViewer.openContextMenu`, 1513). Left Alt + Right Alt targets the folder. Focus then moves to the first menu item (`focusContextMenu`, `App.svelte` 1000).
- **The message body has its own custom menu** in `EmailBody.svelte`. The iframe script posts `{type:'contextmenu', text, url, x, y}` (323–338). The parent draws a `<div role="menu">` with Copy, Copy link and Select all (859–892), plus a full-screen click-catcher (895–904). **This menu has no keyboard navigation, does not move focus, and has no menu-item roles.**
- **A side effect to check:** `App.svelte` 1036–1037 stops *all* global shortcut handling while any `[role="menu"]` element exists. The body menu uses `role="menu"` and never takes focus. Reading the code, the keyboard looks inert while it is open until the user clicks, and its Escape handler sits on an unfocused button. **I have not reproduced this in the running app.** It matters here because a selection popover that used `role="menu"` would switch off every shortcut while visible.
- **Plain-text bodies** render straight into the host DOM (`EmailBody.svelte` 825–844), inside the viewer card's `MessageContextMenu`. So right-clicking selected text in a plain-text mail shows the *message* menu (Reply, Archive…), not Copy. HTML mail shows the body menu instead. The two behave differently.
- **Sidebar folders** have `FolderContextMenu.svelte`.
- **Extensions:** `coreapi.UI().RegisterContextMenuItem` exists with targets `message-row`, `folder-row` and `contact-row` (`internal/core/api/v1/ui.go` 22–38). `docs/EXTENSIONS.md` (line 774) says *no frontend consumer reads these registrations yet*.

### 2.3 Selection and action hierarchy

- `docs/KEYBOARD_SHORTCUTS.md` "Message Actions Hierarchy": actions go to **checked messages** if there are any, otherwise to the **keyboard-focused row**. The open conversation is a separate target. `App.svelte` implements this case by case: the Ctrl chords (Ctrl+U/K/J, ~1250–1290) and, since P-027, the Gmail single keys (`e`, `!`, `#`, `Shift+I`, `Shift+U` and so on, ~1585–1730) each repeat "checked → `getCheckedMessageIds()`, else `getSelectedMessageIds()`". `MessageList` exposes `hasCheckedMessages`, `getCheckedMessageIds` (1855), `getCheckedHasUnstarred`, `getCheckedHasUnread` and `getSelectedMessageIds` (1770–1775), built from `selectedMessageIds` (704).
- **The same mutation logic is written several times:** `MessageContextMenu.svelte` (handlers), `App.svelte` (`handleBulkArchive` 1797, `handleBulkSpam` 1812, plus the key cases above and the command-panel handlers in §2.9) and `ConversationViewer.svelte` (`ArchiveWithUndo` at 1016, `MarkAsSpamWithUndo` at 1174, and so on). Each copy calls the backend with its own undo toast. P-019's Gmail-native spam is routed in the backend move path, so every copy already gets it. The duplication is about targeting and feedback, not provider logic.

### 2.4 Keyboard guards that any new surface must respect

- `isInputElement` (`frontend/src/lib/stores/keyboard.svelte.ts` 97–112) treats `INPUT`, `TEXTAREA` and contenteditable (TipTap) as editable. Single-key shortcuts do not fire there.
- `isDialogGuardActive()` swallows keys while a modal is open (`App.svelte` 1056).
- Inside the body iframe, keydown is forwarded to the host **only when Alt, Ctrl or Meta is held, or for Escape** (`EmailBody.svelte` 340–362). Single keys such as `j`, `k` or the `ContextMenu` key do not reach the host while the iframe has focus.

### 2.5 How message bodies are rendered (this decides what a popover can do)

- HTML bodies are sanitized in Go with bluemonday (`internal/email/sanitizer.go`; `script` is not allowed; `style` is allowed globally at 105; `data-*` attributes at 128; URL schemes `cid data http https mailto` at 90, so **`tel:` links are stripped**; `<time>` and `<data>` are allowed at 56–57, but no `datetime` attribute).
- The frontend then renders the body in an `<iframe sandbox="allow-scripts">` **without** `allow-same-origin` (`EmailBody.svelte` 818–824). The host **cannot read the iframe DOM or its `Selection`**. All interaction already goes through a `postMessage` channel driven by a script the host injects (201–384). The host checks `event.source === iframeElement.contentWindow` (469). The iframe CSP is `script-src 'unsafe-inline'` (391), so the injected script runs.
- **What this means:** text selection *can* be observed, but only by adding a `selectionchange`/`mouseup` listener to the injected script and posting the text and the range's rectangle to the host. The existing `contextmenu` message already does this for the text (327). Coordinates are converted with the iframe's own rectangle, as the tooltip and menu code already does (507–528). The iframe grows to fit its content (`sendHeight`), so it does not scroll internally. The outer `.spark-viewer-scroll` container scrolls instead (`ConversationViewer.svelte` 1767).

### 2.6 Platform facts

- Main window `MinWidth: 360`, `MinHeight: 400`. The detached composer is a **separate `wails.Run` window** with `MinWidth: 500` (`main.go` 155–156, 249–250), so it shares no JS state with the main window.
- `EnableDefaultContextMenu: true` (`main.go` 159). Wherever the app does not call `preventDefault`, the webview's native context menu appears (for example in the composer).
- There is no native floating selection toolbar in desktop WebKitGTK, WKWebView on macOS or WebView2. Any popover has to be drawn by the app.
- Layout modes are `full > 1024`, `medium 768–1024` and `narrow < 768` (`frontend/src/lib/stores/layout.svelte.ts` 1–4).
- `@floating-ui/dom` 1.7.4 is in `package-lock.json` only as a dependency of bits-ui 1.8.0. It is not a direct dependency. TipTap's BubbleMenu extension is **not** installed (only `@tiptap/extensions`, which has focus/placeholder/character-count).

### 2.7 Data the contextual actions would need

- **Add to Calendar:** the calendar backend has `CreateICSFromDetails` (`extensions/calendar/backend/api.go` ~770–800). It is exposed as `Calendar_CreateICSFromDetails` (`bridge.go` 307–312) but nothing in the frontend calls it yet. `EventComposerDialog` takes `defaultStart` but no title or description (`EventComposerDialog.svelte` 38–47). Date parsing is P-022.
- **Add to Contacts:** `App.AddContact(email, displayName)` exists (`app/contact.go` 32–35). The Contacts extension's `AddContactDialog` has no prefill props (`AddContactDialog.svelte` 40–46).
- **Quick replies:** `ListQuickReplies` and `SaveQuickReply` are bound (`QuickReplies.svelte` 14). Inserting adds text at the cursor (`onInsert`).

### 2.8 Shortcut registry (P-026, on main)

`frontend/src/lib/keyboard/registry.ts` defines `ShortcutDef { id, label (i18n key), group, combos, icon?, panes? }` with ids such as `mail.archive`, `mail.spam`, `mail.moveTo`, `list.toggleCheck` and `list.contextMenu`, and the lookups `getShortcut(id)` (226), `getShortcuts(group)` (231) and `appliesToPane` (236). Extension registries are discovered from `extensions/*/frontend/keyboard/registry.ts`. `format.ts` provides `formatShortcut` (100), `withShortcut` (110) and `ariaKeyshortcuts` (116), and `components/shortcuts/ShortcutHint.svelte` renders a key label. `frontend/tests/shortcutRegistry.test.mjs` (run by `npm run check`) keeps the registry in line with the handlers. The registry's header comment (23–29) already says that "a future contextual or selection toolbar should look actions up here". This design follows that.

Since P-027 the message actions have two combos each: Archive `e` / Ctrl+K, Spam `!` / Ctrl+J, Trash `#` / Delete / Backspace / `d`, Mark read `Shift+I` / Ctrl+U, Mark unread `Shift+U` / Ctrl+Shift+U, Move to `v` / Alt+M, Reply `r`, Reply all `a`, Forward `f`, Compose `c`, Search `/`, Undo `z`, Toggle check `x`, Open `o`, Clear selection `u` / Escape. The first combo in each list is the one shown in tooltips.

### 2.9 Command panel (P-027, on main)

- `CommandPalette.svelte` builds a **hard-coded `PaletteItem[]`** (`{ id, title, category, icon, shortcut, action }`, ~23–250). Ids and titles reuse registry ids and `shortcuts.action.*` keys, and shortcuts come from `formatShortcut`, but the icons and the list itself are copied by hand. Theme toggle and view switches are not in the registry, and "Switch to light/dark theme" is an English literal.
- `commandPalette.svelte.ts` keeps a `Map<id, handler>`. `App.svelte` registers the handlers at startup (~596–654). `executeCommand(id)` closes the panel and runs the handler.
- The panel lists **every** command all the time. Nothing filters by context (an empty folder or no open conversation still shows Archive and Reply).
- **Probable targeting bug, found while reading:** the panel's `mail.archive`, `mail.spam`, `mail.star`, `mail.markRead` and `mail.markUnread` handlers pass `[selectedThreadId]`, a *thread* id, to `handleBulkArchive`/`handleBulkSpam`/…, which take *message* ids (the keyboard path passes `getSelectedMessageIds()`). They also ignore checked messages, unlike the keys in §2.3. `mail.trash` does use `getSelectedMessageIds()`. **I have not run this.** If thread ids and message ids differ in this data model, palette Archive/Spam/Star/Read do nothing or act on the wrong item. Shared target resolution (§5.1) fixes it as a side effect, but it should be confirmed and filed on its own if it is real.

**What this means for the design.** The command panel already answers "keyboard-first access to any command". Contextual surfaces should not grow into a second launcher. Instead:

1. One `ActionDef` list (§5.1) drives the panel, menus, bar and popovers. The panel's `PaletteItem[]` is derived from it (plus the few non-registry entries such as theme and views), and `registerCommandHandler` calls go away for actions that have an `ActionDef`.
2. The panel shows only actions whose `isAvailable(ctx)` is true for the current target, and labels the target ("Archive 3 selected", "Archive conversation"). It uses the same `resolveListTarget()` as the keys.
3. Contextual surfaces show the small subset that applies to the current selection, right next to it. The panel stays the "everything, from anywhere" surface. A popover may end with a "More actions… Ctrl+P" button that opens the panel already scoped to that selection (deferred, Q6).

## 3. Evaluation

Scale: ++ strong, + some, 0 neutral, − cost, −− serious cost.

### 3.a Persistent or dockable floating tool palette (Photoshop/Figma)

| Criterion | Assessment |
| --- | --- |
| Power-user value | **−** Photoshop's palette selects a *tool mode* (brush, lasso) that changes what the next pointer gesture does. Mail has no modes. Every action is a verb on a selection that is already visible in the list or viewer. A palette would duplicate the viewer header (§2.1) one pane away from what it acts on. |
| Discoverability | **+** It is always visible, but so is the viewer header. Discoverability is better served by P-026's hints and `?` overlay, the P-027 command panel, and surfaces that appear *where the object is*. |
| Keyboard / focus | **−−** A floating, draggable panel needs its own place in the focus order (F6-style landmark cycling, which the app does not have). Dragging needs a keyboard alternative. For keyboard users it adds nothing over the single keys and the Ctrl/Cmd+P command panel, which already lists every action without taking space. |
| Screen readers | **−** It is a non-modal region with no context. Each button needs a long label saying what it acts on. |
| Real estate | **−−** The main window can be 360 px wide, and narrow mode is under 768 px. A floating panel covers list rows or body text. Docking it means reserving a strip, which is what the existing toolbars already do. |
| Window resize / detached composer | **−** Its position has to be clamped and persisted across resizes and layout modes. The detached composer is a separate process (§2.6), so the palette can't follow it there without a second copy. |
| Clutter | **−−** It goes against CONTRIBUTING's "Minimalist … clutter-free UI" principle. |
| Complexity | **−** Drag, dock and collapse state, persistence, z-index fights with bits-ui portals and toasts (`ToastContainer.svelte` 31 is fixed bottom-right). |

**Verdict: reject.** The only reasonable version, a collapsible docked strip, is what the viewer header and composer toolbar already are. Making those toolbars complete and labelled with shortcuts (P-026) gives the same benefit.

### 3.b Selection-triggered contextual surfaces

#### (i) Message-list selection → action bar

| Criterion | Assessment |
| --- | --- |
| Power-user value | **++** It shows the checked-versus-focused hierarchy (§2.3). Today a user can't see that `Ctrl+K` will archive 12 checked messages and not the highlighted one. The bar makes that state visible ("12 selected") and gives mouse users the full verb set without right-clicking. |
| Discoverability | **++** Each button shows its shortcut (`Archive Ctrl+K`), which teaches the keys at the moment they would be useful. |
| Keyboard / focus | **+** Docked bar with `role="toolbar"` and roving tabindex. It is never focused automatically, so `j`, `k`, `Space` and `s` keep working in the list. It is reachable with Tab or Shift+Tab from the list header. |
| Screen readers | **+** A polite live region announces "N selected". Buttons carry `aria-keyshortcuts`. |
| Real estate | **0** It replaces the list header row while a selection exists, so the rows don't shift (see §6.1). At narrow widths it collapses to icons plus a "More" menu. |
| Complexity | **+** The bar and data already exist (§2.1, §2.3). The work is completing it, translating it, and routing it through shared action definitions. |
| Floating vs docked | Docked. A Spark-style floating bottom bar would sit on top of the virtualized list's last rows and the bottom-right toast stack. |

**Verdict: build first** (Phase 1). The focused row alone does **not** show the bar. The focused row is always present, so the bar would never go away. The hover quick-actions and the viewer header already cover the single-message case.

#### (ii) Text selection inside a message body → selection popover

| Criterion | Assessment |
| --- | --- |
| Power-user value | **+** Quote-reply with the selection, Copy, Search mail for the selection, and actions on a recognised entity: date/time → Add to calendar (P-022), email address → Compose / Add to contacts, URL → Open / Copy link, phone number → Copy. These are real tasks, and today Copy, Copy link and Select all are the only options. |
| Discoverability | **++** for entity actions. Users won't think to look in a menu for "Add to calendar", but a small popover over a selected date makes it obvious. |
| Keyboard / focus | **Risky, so constrain it:** the popover must **never take focus**, must **never use `role="menu"`** (that would stop shortcuts, §2.2), and **must not appear for keyboard-driven selection** (Shift+arrows or `Ctrl+A` select-all inside the iframe), so keyboard users don't get flicker. Keyboard users get *the same items* from the context menu key (`ContextMenu`, Shift+F10, Right Alt). That menu is the accessible path, and the popover is a pointer shortcut to it. |
| Screen readers | **0** The popover is `role="toolbar"` with an accessible name, but it is not announced on appearance (that would be noisy). The context menu is the screen-reader path and announces normally. |
| Webview constraints | **Feasible with work:** selection is only visible through the injected iframe script (§2.5). The script must post the text (capped) and `range.getBoundingClientRect()`. The host converts coordinates and treats the payload as untrusted data (it renders it only as text and uses it only as an action argument). Clicking a host button may blur the iframe. The selection is captured when the popover is shown, so actions use the stored text. **Unverified:** whether WebKitGTK and WKWebView keep the iframe's *visible* highlight once focus moves to the host. Emails can set `user-select:none` through inline `style`, which the sanitizer allows, and nothing can be done about that. |
| Performance | **+** A debounced (~150 ms) `selectionchange` and `mouseup` in the iframe. Entity detection runs on demand only, on at most ~500 selected characters, never on the whole body and never during sync. That fits the "lightweight sync" rule in AGENTS.md. |
| Clutter | **−** It is mitigated by showing at most 4 visible items plus "More", showing only after a mouse selection of ≥ 2 non-whitespace characters, and having a setting to turn it off. |
| Complexity | **−** It needs iframe-script changes, positioning, dismissal, and a separate code path for plain-text bodies (host DOM). |

**Verdict: build after the menu model (Phase 3).** Generic actions first. Date → calendar waits for P-022's parser.

#### (iii) Text selection in the composer → formatting bubble

| Criterion | Assessment |
| --- | --- |
| Power-user value | **+** in the compact inline composer, where the toolbar is hidden behind a toggle (§2.1). **0** in the full composer, where the toolbar is already visible and `Ctrl+B/I/U` plus Alt+T hints exist. |
| Discoverability | **+** Formatting and "Link…" are visible right where the text was selected, with their shortcuts. |
| Keyboard / focus | **Constraint:** the editor is contenteditable, so `isInputElement` is true and single-key shortcuts are already off. The bubble must not take focus from the editor and must not open for keyboard selection. Keyboard users keep `Ctrl+B/I/U` and Alt+T hint mode. |
| Webview constraints | **+** Same document, no iframe. TipTap gives `editor.on('selectionUpdate')` and `posToDOMRect` from `@tiptap/core`, so the BubbleMenu extension isn't needed. The detached composer is its own window, so the bubble renders there separately at no extra cost. |
| Contents | Bold, Italic, Underline, Link…, Quote, Clear formatting, and **"Save as quick reply"** (it acts on the selection, so it belongs here). Inserting a quick reply or template is a caret action, not a selection action, so it **stays in the footer**. |
| Complexity | **+** Medium-small. Reuses `EditorToolbar` commands. |

**Verdict: build (Phase 4).** The default is on in the compact composer and off in the full composer. See open question Q3.

#### (iv) Sender/recipient chips and attachments

| Criterion | Assessment |
| --- | --- |
| Value | **+** Chips today only copy on click (`ConversationViewer.svelte` ~1840–1920). A chip context menu would offer Copy address, Compose to, Add to contacts, Search mail from/to this address, Always load images from this sender (that logic exists in `EmailBody.svelte` 595–617), and later "Create filter…" (P-014). Attachments: Open, Save, Save all, Copy file name, and later "Import to calendar" for `.ics`. |
| Discoverability / keyboard | **+** Chips and attachment buttons are already focusable. The `ContextMenu` key or Shift+F10 on a focused chip opens its menu. No popover needed. |

**Verdict: build as part of the context-menu work (Phase 2 for chips, Phase 5 for attachments).** Hover popovers on chips are rejected because they cause accidental pop-ups in a dense header.

### 3.c Context menus and their keyboard equivalent

| Criterion | Assessment |
| --- | --- |
| Value | **++** Context menus are the one surface that is consistent, can be opened from the keyboard, and adds no clutter. They are also the natural home for P-019 (Gmail spam), P-014 (filters from a message) and extension items (`RegisterContextMenuItem`). |
| Discoverability | **+** with P-026 shortcut hints in each item. **−** on its own: right-click is invisible. Hence the bar (i) and popover (ii) as visible companions. |
| Keyboard | **++** once the body menu moves onto the same bits-ui primitive. The iframe needs to forward `ContextMenu` and Shift+F10 (today it only forwards modifier chords). Right Alt already works because it sets `altKey`. |
| Complexity | **+** The bits-ui primitives are in place. The body menu is the only custom one. |

**Command panel (exists since P-027).** It is the best fit for keyboard-first users and is not contextual. The contextual surfaces are its local counterpart for pointer users and for discovery. They must **reuse** the panel's action ids and handlers, not add a parallel list (§2.9). The concrete overlap work is in P-029: derive the panel's items from `ActionDef`, filter them by availability, and route them through the shared target resolution.

### 3.d Combinations considered

| Combination | Verdict |
| --- | --- |
| Floating tool palette + context menus | Rejected. The palette's costs remain, and the menus can be done alone. |
| Docked list bar + menus (no popovers) | Acceptable minimum. It covers bulk work and the keyboard path but misses the "highlight a date, add to calendar" request. |
| **Docked list bar + unified menus + body popover + composer bubble + the existing command panel, all from one action layer** | **Recommended.** Each contextual surface appears only when there is a selection, sits next to it, and shows the same labels and shortcuts as the command panel. |
| Popovers without unified menus | Rejected. It would give mouse users features that keyboard users can't reach. |

## 4. Wireframes

### 4.1 Message list with checked messages (Phase 1)

The bar replaces the list header row while `checkedThreadIds.size > 0`. Esc clears it, as it does today.

```
full / medium width
┌───────────────────────────────────────────────────────────────────────┐
│ ☑ 3 selected │ ✓Read ⇧I  ☆Star S  ▣Archive E  ⇢Move V  ⚠Spam !  🗑 # │ ⋯ │ ✕ U │
├───────────────────────────────────────────────────────────────────────┤
│ ☑ Alice    Quarterly numbers               10:02                      │
│ ☐ Bob      Re: Lunch?                       09:40  ← keyboard focus   │
│ ☑ Carol    Invoice 2026-10                  Yesterday                 │
└───────────────────────────────────────────────────────────────────────┘
narrow (< 768 px): icons only, shortcut in tooltip/aria-keyshortcuts
┌─────────────────────────────────────────┐
│ 3 │ ✓ ☆ ▣ ⇢ ⚠ 🗑 │ ⋯ │ ✕ │
└─────────────────────────────────────────┘
⋯ More: Mark unread ⇧U · Copy to… ⌥C · Delete permanently ⇧⌫ · Select all ⌃A · All actions… ⌃P
```

### 4.2 Message body selection popover (Phase 3)

The popover appears above the selection after a pointer selection. It flips below if there is no room and never covers the selected text.

```
            ┌──────────────────────────────────────────────────┐
            │ ❝ Quote reply │ ⧉ Copy │ 🔍 Search │ 📅 Add to calendar │ ⋯ │
            └───────────────────────▲──────────────────────────┘
   …can we meet on [Thursday 30 Oct at 14:00] at the office?…
   ⋯ More: Reply all with quote · Forward with quote · Copy as quote
   Keyboard: Menu / Shift+F10 / Right Alt → same items as a focused menu
```

When the selection is an email address, the entity slot shows `✉ Compose to` and `👤 Add to contacts`. For a URL it shows `↗ Open link` and `⧉ Copy link`.

### 4.3 Body context menu (Phase 2), same items as the popover

```
┌───────────────────────────────┐
│ ❝ Reply with quote            │
│ ⧉ Copy                  ⌃C    │
│ 🔍 Search mail for "Thursday" │
│ 📅 Add to calendar…           │   ← only when an entity is detected
├───────────────────────────────┤
│ ↗ Open link                   │   ← only on a link
│ 🔗 Copy link                  │
├───────────────────────────────┤
│ Select all              ⌃A    │
│ ── Message ─────────────────  │
│ ↩ Reply                 R     │   ← message submenu/section so the
│ ▣ Archive               E     │     body menu does not hide message verbs
└───────────────────────────────┘
```

### 4.4 Composer formatting bubble (Phase 4)

```
         ┌─────────────────────────────────────────────┐
         │ B  I  U  │ 🔗 Link… │ ❝ │ Tx │ ＋ Save as quick reply │
         └─────────────────────▲───────────────────────┘
   Thanks — [I'll send the revised draft tomorrow]. Best, O.
```

### 4.5 Chip context menu (Phase 2)

```
 From: Alice Example <alice@example.com>   ← focus + Menu key or right-click
 ┌────────────────────────────────────┐
 │ ⧉ Copy address                     │
 │ ✉ New message to Alice             │
 │ 👤 Add to contacts                 │
 │ 🔍 Search mail from Alice          │
 │ 🖼 Always load images from sender  │
 │ ⚙ Create filter… (after P-014)    │
 └────────────────────────────────────┘
```

## 5. Shared building blocks

### 5.1 Action definitions (`frontend/src/lib/actions/`)

There is one format for every surface. Labels, icons and key labels come from the P-026 registry when an action has a shortcut, so they can't drift.

```ts
// frontend/src/lib/actions/types.ts  (proposed)
export type ActionContextKind =
  | 'listSelection'   // checked messages (or focused row for menus)
  | 'message'         // one message card in the viewer
  | 'bodySelection'   // selected text in a message body
  | 'link'            // a link in a message body
  | 'address'         // sender/recipient chip
  | 'attachment'
  | 'composerSelection'

export interface ActionDef<C> {
  id: string                 // reuse the P-026 registry id when one exists ('mail.archive')
  shortcutId?: string        // defaults to id; omitted when the action has no shortcut
  label?: string             // i18n key; falls back to registry label
  icon?: string              // iconify name; falls back to registry icon
  contexts: ActionContextKind[]
  priority: number           // lower = shown first; beyond the visible budget goes to "More"
  destructive?: boolean      // styling + never the default-focused menu item
  isAvailable(ctx: C): boolean          // e.g. hide Move to inbox outside Trash
  labelFor?(ctx: C): string             // dynamic (Star / Remove star; Spam / Not spam)
  run(ctx: C): Promise<void> | void
}
```

- **`resolveListTarget()`** turns the existing hierarchy into one pure function: checked → focused row → nothing. App's `Ctrl+U/K/J` cases, the bar and the row menu all call it, so they can't disagree.
- **A message action executor** (`messageActions.ts`) takes the archive, trash, spam, move, copy, star, read and undo-toast logic out of `MessageContextMenu.svelte`, `App.svelte` and `ConversationViewer.svelte` (§2.3) so every surface calls the same function. **This is a refactor that should change no behaviour** apart from fixing the command-panel targeting in §2.9 if that bug is confirmed. It is the riskiest step, so it is its own task (P-029).
- **Command panel adapter:** `paletteItemsFrom(actions, ctx)` produces the panel's `PaletteItem[]`, and `executeCommand(id)` looks up the `ActionDef` first and falls back to `registerCommandHandler` handlers for the non-action entries (views, theme, focus). The panel component keeps its UI; only its data source changes.
- **Extension items:** a later consumer of `RegisterContextMenuItem` can map each registration to an `ActionDef` whose `run` calls the extension handler. The format already allows for it.

### 5.2 `kit/ActionBar.svelte` (docked)

- Props: `actions: ActionDef[]`, `ctx`, `label` (i18n), `countText?`, `onClose?`.
- `role="toolbar"`, `aria-label`, roving tabindex (Left/Right/Home/End), and a "More" `DropdownMenu` for overflow. A `ResizeObserver` measures the space available. Each button sets `aria-keyshortcuts` and shows `<ShortcutHint>` (P-026).
- Never auto-focused. Escape inside the bar returns focus to the list and clears the selection (the same as list Escape).

### 5.3 `kit/SelectionPopover.svelte` (floating)

- Props: `anchor: DOMRect` (viewport coordinates), `actions`, `ctx`, `label`, `onDismiss`.
- `role="toolbar"`, **never `role="menu"`** (§2.2). The container is not focusable. `mousedown` on buttons calls `preventDefault()` so the selection and the editor focus survive the click.
- **Positioning** is a pure function `placePopover(anchor, size, viewport, opts)` → `{x, y, side}`:
  - centre it horizontally on the anchor and clamp to an 8 px viewport gutter;
  - put it above with an 8 px gap; flip below if `anchor.top − size.h − 8 < viewport.top + headerInset`;
  - never overlap the anchor rectangle; for a multi-line selection, anchor to the first line's rectangle (above) or the last (below);
  - at narrow widths (< 400 px) show icons only.

  A small own function is preferred over adding `@floating-ui/dom` as a direct dependency. It is ~40 lines and can be unit-tested with `node:test` (§8). `@floating-ui/dom` 1.7.4 is already in the lockfile through bits-ui if collision handling gets more complex.
- **Dismissal:** Escape, selection collapsed or changed to under 2 characters, `mousedown` outside, scroll of the containing pane, window resize or blur, change of message, pane focus change, running any action. The popover listens for **Escape in the capture phase and only consumes it when visible**. Otherwise App's Escape chain (clear checks → close conversation) runs as it does today.
- **Trigger rules:** show only after a *pointer* selection (`mouseup` with a non-collapsed range). Keyboard-made selections (Shift+arrows, `Ctrl+A`) never show it. Wait ~150 ms after `mouseup`, so a double-click to select a word doesn't flash.
- `prefers-reduced-motion`: no fade or scale.

### 5.4 Context menus

- `MessageContextMenu` renders its items from `ActionDef`s (Phase 2). The visible items, order and existing `<ShortcutHint>`s stay the same.
- The body menu moves onto the bits-ui `ContextMenu` or `DropdownMenu` primitive. That fixes focus, roles and Escape, and it fits `focusContextMenu()` (`App.svelte` 1000).
- Chip and attachment menus use the same primitive and the same `ActionDef` lists.

### 5.5 Iframe protocol additions (`EmailBody.svelte` injected script)

| Message | Payload | Notes |
| --- | --- | --- |
| `selection` | `{ text (≤ 2 000 chars), rect: {x,y,w,h}, firstRect, lastRect, pointer: boolean }` | Debounced. `pointer` is true when it follows a `mouseup`. Sent with `text: ''` when the selection collapses. |
| `contextmenu` (existing) | add `rect` of the selection when there is one | Lets the keyboard-invoked menu anchor to the selection. |
| keydown forwarding | also forward `ContextMenu` and `Shift+F10` | So the keyboard path works while the iframe has focus. |
| `quote-selection` (host → iframe) | — | **Not needed.** The host uses the captured text. |

The host already checks `event.source` (469). Payloads are treated as untrusted strings: they are rendered as text and passed to typed backend calls, never put into HTML.

### 5.6 Entity detection (`frontend/src/lib/actions/detect.ts`)

- `detectEntities(text): Entity[]` runs only on selected text.
- Email address and URL use conservative regexes; URLs reuse `isAllowedEmailURL`. Phone numbers use a conservative pattern and offer **Copy only**. The sanitizer strips `tel:` (§2.5), and the app has no dialler.
- Date/time: **delegate to the P-022 parser**, and show nothing until it lands. P-022 must expose a function that takes plain text and returns candidates with a confidence and timezone ambiguity. The popover shows Add to calendar only above P-022's threshold and opens the ICS review flow (`Calendar_CreateICSFromDetails`), never adding an event silently. That is P-022's own acceptance criterion.
- Hide actions whose extension is disabled. For example, Calendar and Contacts actions depend on the extension being enabled; reuse the rail's enabled list.

### 5.7 Data each context needs

| Context | Data | Source |
| --- | --- | --- |
| listSelection | checked thread ids → message ids, aggregate starred/unread, folderType, accountId, `moveMutationInFlight` | `MessageList` 704–724, 1855–1888 |
| message | message id, account, folder, `isStarred`/`isRead` | `ConversationViewer` card props (1789 onward) |
| bodySelection | text, rects, message id, account id, from address, detected entities | iframe `selection` message + `EmailBody` props |
| link | URL (allowlisted) | existing `contextmenu` message |
| address | name, email, role (from/to/cc/bcc/reply-to), account id | viewer header loop (~1840–1920) |
| attachment | id, filename, MIME type, encrypted flag | `AttachmentList.svelte` |
| composerSelection | TipTap editor, selection text, plain-text-mode flag | `Composer.svelte` / `EditorToolbar` |

### 5.8 i18n

- New keys under `actions.*` (labels the registry doesn't have), `selectionBar.*` (`count` with plural forms, `ariaLabel`, `more`) and `selectionPopover.*` (`ariaLabel`, `searchFor` with `{text}` placeholder, truncated).
- Reuse `messageList.selectedActions` and `messageList.selectedCount` (added by P-027) for the bar. Replace the command panel's English "Switch to light/dark theme" literal while moving its items onto `ActionDef`.
- Add each new key to every locale file in `frontend/src/lib/i18n/locales/` (English text as placeholder where no translation exists, following `docs/LANGUAGE.md`) and extension locales as needed, keeping key and placeholder parity, which P-001 checks.

### 5.9 Accessibility requirements (acceptance criteria for every phase)

1. No new surface takes focus on its own. Focus moves only on an explicit key (Tab into the bar, the Menu key into a menu) or a click.
2. Single-key shortcuts behave exactly as before whenever a bar or popover is visible: `j/k/x/s/e/!/#/v/Space/d` in the list, `Ctrl+B/I/U` in the composer.
3. Floating surfaces never use `role="menu"` (App's guard at 1036–1037).
4. Every action in a popover can be reached from the keyboard through a context menu, and every action in a bar through Tab and arrows.
5. `aria-label`/`aria-keyshortcuts` on icon-only buttons; a polite live region for the selection count; menus announce as menus.
6. Visible focus ring; 24×24 px minimum hit target; contrast follows the theme tokens.
7. Popovers respect `prefers-reduced-motion` and the settings toggle (Q2).

## 6. Phased roadmap

| Phase | Task | Effort | Build | Depends on |
| --- | --- | --- | --- | --- |
| 1 | **P-029** | Medium | `lib/actions/` types, `resolveListTarget`, message action executor extracted from the duplicates; App key cases and the command panel's mail commands use them; panel items derived from `ActionDef` and filtered by availability (no other behaviour change) | P-026, P-027 (both on main) |
| 1 | **P-030** | Easy | `kit/ActionBar.svelte`; complete list selection bar (replaces `MessageList` 2398–2415) | P-029 |
| 2 | **P-031** | Medium | Unified context menus: body menu on bits-ui with focus/Escape; iframe forwards Menu and Shift+F10; body items Reply with quote / Copy / Search / Open link / Copy link / Select all + message section; chip menus | P-029 |
| 3 | **P-032** | Medium | `kit/SelectionPopover.svelte` + `placePopover`; iframe `selection` message; plain-text body path; generic actions plus email/URL entities; settings toggle | P-031 |
| 3b | (inside P-022) | — | Add to calendar entity in the popover and menu, using P-022's parser and the ICS review flow | P-022, P-032 |
| 4 | **P-033** | Easy | Composer formatting bubble (+ Save as quick reply) for the compact composer; optional in the full one | P-029 (action format); shares `placePopover` with P-032 (whichever ships first adds it) |
| 5 | **P-034** | Medium | Attachment context menus, plus a frontend consumer for `RegisterContextMenuItem` (`message-row` target) | P-031 |

**Smallest valuable slice:** P-029 + P-030. They make the checked-versus-focused hierarchy visible, teach the new Gmail single keys in place, and make the command panel act on the same target as the keys. If the owner wants something visible sooner, P-030 *can* ship first by calling the existing `onBulk*` callbacks, with P-029 following. In that case, record the extra duplication in the task.

### 6.1 Implementation notes for Phase 1

- Render the bar **in place of the list header toolbar row** so the rows don't move when the first box is checked. If the header and bar heights differ, give the bar the header's height. Check visually in all three layout modes.
- Bar actions, in priority order: Mark read/unread (aggregate), Star/unstar (aggregate), Archive, Move to…, Spam/Not spam (folder-aware; Gmail-native once P-019 lands, without changing the bar), Trash/Delete permanently (folder-aware), Move to inbox (Trash only). Overflow: Copy to…, Select all/none.
- "Done" (`common.done`) is currently "mark read". Keep it as a label alias only if the owner wants it (Q4).

### 6.2 Deferred

- **Selection-scoped command panel** ("More actions… Ctrl+P" from a popover or bar opens the panel filtered to that selection). Small once P-029 exists (Q6).
- **Extension-contributed items in popovers.** Menus first (P-034). Popovers are space-limited.
- **Smart chips / inline entity underlining** (highlighting dates in the body without a selection). Higher clutter and constant parsing. Revisit after P-022 shows its precision.
- **Search the web for the selection.** Privacy-sensitive (sends text to a third party). Could be done with `OpenURL` on explicit user action, but needs an owner decision (Q5).

### 6.3 Rejected

- **Photoshop/Figma floating or dockable tool palette**, including a "pinned" mode (§3.a).
- **Translate selection.** No local translation engine. It would send mail content to a third party, against CONTRIBUTING's privacy principle.
- **Popovers on keyboard selection, on hover over chips, or on the focused list row.** They would flicker, steal attention, or never go away.
- **Templates and quick-reply *insertion* in the selection bubble.** These act at the caret and belong in the footer, where P-018 put them.
- **A second launcher or command list inside contextual surfaces.** The P-027 command panel is the single launcher (§2.9).

## 7. Action × context matrix

Shortcut labels are the bindings in `frontend/src/lib/keyboard/registry.ts` on `main`, first combo first. Every action listed with a registry id is also in the command panel (P-027) once P-029 derives the panel from `ActionDef`. "—" means no shortcut. In the context columns, **B** means the selection bar, **M** the context menu and **P** the selection popover. Parentheses mean the item appears in "More".

| Action | Registry id / shortcut | List selection | Message card | Body selection | Link | Address chip | Attachment | Composer selection |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Reply | `mail.reply` Ctrl+R / R | M (single) | M | (M) | | | | |
| Reply all | `mail.replyAll` Ctrl+Shift+R / A | M (single) | M | (M) | | | | |
| Forward | `mail.forward` Ctrl+F / F | M (single) | M | (M) | | | | |
| Reply with quote | — (new) | | | **P**, M | | | | |
| Mark read / unread | `mail.markRead` Shift+I, Ctrl+U / `mail.markUnread` Shift+U, Ctrl+Shift+U | **B**, M | M | | | | | |
| Star / unstar | `mail.star` S | **B**, M | M | | | | | |
| Archive | `mail.archive` E / Ctrl+K | **B**, M | M | (M) | | | | |
| Move to… | `mail.moveTo` V / Alt+M | **B**, M | M | | | | | |
| Copy to… | `mail.copyTo` Alt+C | (B), M | M | | | | | |
| Spam / not spam (Gmail-native via P-019) | `mail.spam` ! / Ctrl+J | **B**, M | M | | | | | |
| Trash | `mail.trash` # / Delete / Backspace / D | **B**, M | M | | | | | |
| Delete permanently | `mail.deletePermanently` Shift+Delete | (B), M (Trash) | M (Trash) | | | | | |
| Move to inbox | — | B, M (Trash) | M (Trash) | | | | | |
| Copy | native Ctrl+C | | | **P**, M | | M | M (file name) | |
| Select all | `viewer.selectText` Ctrl+A | (B) select all messages via `list.selectAll` | | M | | | | |
| Search mail for … | — (new) | | | **P**, M | | M | | |
| Open link | — | | | | **P**, M | | | |
| Copy link | — | | | | **P**, M | | | |
| Compose to address | — | | | **P** (email entity), M | M (`mailto:`) | M | | |
| Add to contacts | — | | | **P** (email entity), M | | M | | |
| Add to calendar… | — (P-022) | | | **P** (date entity), M | | | M (`.ics`, P-034) | |
| Always load images from sender | Ctrl+Shift+L opens menu | | | | | M | | |
| Create filter… | — (P-014) | (B) later | M later | | | M later | | |
| Open / Save attachment | — | | | | | | M | |
| Bold / Italic / Underline | `composer.bold` Ctrl+B / `composer.italic` Ctrl+I / `composer.underline` Ctrl+U | | | | | | | **P** |
| Link… | — | | | | | | | **P** |
| Quote / Clear formatting | — | | | | | | | **P** |
| Save as quick reply | — (new) | | | | | | | **P** |
| Clear selection | `list.escape` Escape / U | **B** (✕) | | | | | | |
| All actions (command panel) | `app.commandPalette` Ctrl/Cmd+P | (B) | | (P) later | | | | |
| Open context menu | `list.contextMenu` Menu / Right Alt (+ Shift+F10 proposed) | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | native |

New actions with no shortcut stay without one in this plan. Choosing keys belongs to the registry owners (P-026/P-027 follow-ups). If shortcuts are added later, they show up automatically through the registry.

## 8. Verification plan

### 8.1 Test runner

`frontend/tests/*.test.mjs` already uses `node:test` with `ts.transpileModule` to load pure `.ts` modules (for example `tests/autoSelectNext.test.mjs` 1–12), but `package.json` has **no `test` script**. P-026 added `check:shortcuts`, which runs only `tests/shortcutRegistry.test.mjs` as part of `npm run check`.

**Proposal (in P-029):** add `"test": "node --test tests/"` to `frontend/package.json` and keep the existing pattern. Don't add Vitest or jsdom for this work: jsdom has no layout, so `getBoundingClientRect` and `Selection` rectangles would be fake, and the risky behaviour depends on real webview layout anyway.

### 8.2 Unit tests (pure modules, written red-then-green per ROADMAP guidance)

| Module | Cases |
| --- | --- |
| `resolveListTarget` | checked wins over focused; focused used when nothing is checked; empty when neither; search-result rows included (mirrors `selectedMessageIds`) |
| action availability | Trash shows Move to inbox and Delete permanently; Spam folder shows Not spam; single-only actions hidden for multi-select; aggregate star/read labels |
| registry consistency | every `ActionDef.shortcutId` exists in the P-026 registry; label and icon fall back correctly; no duplicate ids; every panel item with a registry id comes from an `ActionDef` (no hand-kept copy) |
| command-panel targeting | `mail.archive`/`spam`/`star`/`markRead` from the panel resolve to the same message ids as the matching keys, for checked and for focused-only cases (red first if the §2.9 bug is real) |
| `placePopover` | above/below flip; horizontal clamp at both edges; never overlaps anchor; narrow-width icon mode; multi-line anchor choice |
| `detectEntities` | emails, URLs (with allowlist), phone pattern, no false positives on version numbers or prices; length cap |
| dismissal reducer (if state is a reducer) | Escape consumed only when visible; collapse, scroll and message change dismiss |

### 8.3 Manual verification checklist (each phase, recorded in the TASKS update)

Run on **WebKitGTK (Linux/Flatpak)** and **WKWebView (macOS)**, plus WebView2 if Windows builds are produced:

1. With the bar or popover visible, `j/k`, `x`, `Space`, `s`, `e`, `#`, `Ctrl+K`, `Ctrl+P`, `Escape` behave as before. Escape dismisses the popover first, then clears checks, then closes the conversation.
2. In the composer, `Ctrl+B/I/U` and Alt+T hints work with the bubble visible, and typing never moves focus into the bubble.
3. Selecting text in an HTML mail by mouse shows the popover. Selecting with Shift+arrows or `Ctrl+A` does not. The Menu key, Shift+F10 and Right Alt open the menu with the same items.
4. **Webview checks this document could not settle from code:** (a) whether the iframe's visible highlight stays when a host button is clicked; (b) whether `selectionchange` fires inside a sandboxed `srcdoc` iframe on each engine; (c) whether the `ContextMenu` key and Shift+F10 inside the iframe produce a `contextmenu` event or need forwarding; (d) the position with dark-mail `filter: invert` on (filters should not affect layout rectangles); (e) behaviour at 360 px width and in the detached composer (500 px).
5. Screen reader pass (VoiceOver and Orca): bar announced as a toolbar with count; menus announce items and shortcuts; the popover's appearance is not announced.
6. Performance: no measurable idle CPU from the selection listeners (they are event-driven only). Nothing new runs during sync.

## 9. Open product questions

| # | Question | Default chosen (and why) | Status |
| --- | --- | --- | --- |
| Q1 | Floating palette at all? | **No** (§3.a). | Resolved with default; owner may overrule. |
| Q2 | Should the body popover and composer bubble be on by default, and should there be a setting? | **On, with a single "Show selection toolbars" toggle in Settings › General.** Discoverability is the goal, and one toggle stays minimalist. | Resolved with default. |
| Q3 | Composer bubble in the full composer too, or only the compact one? | **Compact only by default.** The full composer already shows the toolbar, so a bubble there would be redundant. | Resolved with default. |
| Q4 | Keep the "Done" label (= mark read) in the bar, or rename it "Mark read"? | **"Mark read" with the `Ctrl+U` hint**, so the label matches the registry and the shortcut. | **Needs owner** (Done may be a deliberate Spark-ism). |
| Q5 | Offer "Search the web for selection"? | **Not now** (privacy). | **Needs owner.** |
| Q6 | Should the command panel offer a selection-scoped mode ("More actions…" from the bar or popover opens Ctrl+P filtered to the selection)? | **Defer** until P-029 has the shared action layer. | **Needs owner.** |
| Q10 | Should the command panel hide actions that do not apply to the current target, or show them disabled? | **Hide** them (shorter list; the `?` overlay already lists every shortcut). | Resolved with default. |
| Q7 | Should Add to calendar open the ICS review flow (P-012) or the in-app `EventComposerDialog` when the Calendar extension is enabled? | **ICS review flow**, matching P-022's acceptance criterion. `EventComposerDialog` only takes `defaultStart` today. | Resolved with default; revisit with P-022. |
| Q8 | Add to contacts: local contacts (`App.AddContact`) or the Contacts extension dialog with prefill? | **Contacts extension dialog when enabled** (needs prefill props), otherwise `AddContact`. That lets the user pick the address book. | Resolved with default. |
| Q9 | Should the body context menu include message verbs (Reply/Archive) under a separator, given that HTML and plain-text bodies currently behave differently? | **Yes**, a "Message" section, so right-click anywhere in a card offers message verbs. | Resolved with default. |

## 10. What is not verified

- The command panel targeting bug in §2.9 is inferred from code (thread id passed where message ids are expected), not reproduced.
- The keyboard-dead state while the body's custom `role="menu"` is open (§2.2) is inferred from code, not reproduced.
- All webview behaviour in §8.3 item 4 is unverified. It can only be checked in the running app on each engine.
- I did not run the app, `svelte-check` or any tests for this document. It changes only documentation.
