# Contextual signatures Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Choose a signature for the current sender, recipients and compose mode, while preserving manual edits and the existing per-identity signature as fallback.

**Architecture:** Store an ordered set of signature rules on each sender identity. Match To/Cc/Bcc recipients and compose mode in a small pure helper; exact addresses outrank domains, and the existing identity signature is the fallback. The composer tracks the last auto-inserted signature and only replaces it while that region remains unedited; editing the signature becomes a manual override for that compose session.

**Tech Stack:** Go account model/store, SQLite migration, Wails bindings, Svelte 5 composer/settings, existing TipTap signature marker.

---

### Task 1: Extend the identity storage contract

**Files:** `internal/account/model.go`; `internal/account/store.go`; `internal/database/migrations.go`; account store tests; generated Wails model bindings.

Add JSON-backed `SignatureRules` to `Identity` and `IdentityConfig`, defaulting to an empty list. Each rule has a stable ID, enabled flag, recipient match entries (exact addresses or domains), message-type selection, HTML/text content, and signature placement/separator. Validate size/count bounds, recipient syntax and supported modes. Preserve old rows with no rules and retain the existing identity-wide signature unchanged.

### Task 2: Add a deterministic selector

**Files:** new pure Go selector helper under `internal/account/`; focused unit tests.

Select only from the selected sender identity's rules. Consider all To/Cc/Bcc recipients. Prefer exact-address rules over domain rules, then configuration order; unsupported/empty context falls back to the existing signature. New, reply, reply-all and forward remain distinct modes.

### Task 3: Add rule editing to identity settings

**Files:** `frontend/src/lib/components/settings/account/IdentityEditor.svelte`; localized strings; settings tests.

Allow adding, editing, reordering and removing rules. Each rule exposes recipient address/domain matching, message type, HTML/text signature and placement. The existing signature editor remains the fallback. Keep the UI keyboard-accessible and avoid changing account editor geometry unnecessarily.

### Task 4: Apply rules in compose without losing edits

**Files:** `frontend/src/lib/components/composer/Composer.svelte`; `composerSignature.ts`; focused frontend tests.

Resolve the initial signature after identity, compose mode and initial recipients load. Re-evaluate when recipients or sender/mode change. Replace only a generated signature that still matches the previous generated block; if the user edits that block, keep it as a manual override. Changing sender identity may clear the override only if that is already the established behavior; preserve message content, draft state, quote boundaries and the existing remove/reinsert path.

### Task 5: Verify and checkpoint

Run focused account-store/selector tests, signature composer tests, Svelte checks, changed-file lint and production build. Confirm legacy identities still use their original signature and manual edits survive recipient changes. Commit storage, UI, and composer checkpoints separately; update `TASKS.md` with exact verification and remaining platform checks.

**Acceptance:** Rules choose by sender identity, recipient and message type; rule order is deterministic; no matching rule uses the current identity signature; changing recipients updates only an untouched automatic signature; manually edited content remains intact; existing identities/drafts migrate without data loss.
