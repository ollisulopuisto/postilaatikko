# macOS Contacts Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Browse macOS contacts and complete recipient addresses without writing to Apple's contact database.

**Architecture:** A small CNContactStore bridge checks authorization without prompting; an explicit Settings action requests access. Existing contact search merges native email addresses in both composer windows. A separate Settings browser opens selected records in Contacts.app for edits.

**Tech Stack:** Go, Objective-C Contacts/AppKit, Svelte 5, Wails.

---

### Task 1: Permission boundary and native reads

Create `internal/macoscontacts/source.go`, platform bridge/stub and `source_test.go`. Write tests proving denied/unknown permission cannot enumerate, request only occurs explicitly, result limits are clamped and opening validates identifiers. Run the tests red, implement the service, and rerun green. The bridge fetches name/email only, bounds enumeration, and has no save API. Add usage descriptions to both Darwin plists.

### Task 2: Recipient lookup

Modify `internal/contact/store.go` to merge native email pairs after authorized lookup; retain existing deduplication/ranking and degrade gracefully on native failures. Add tests in `internal/contact/macos_test.go`, run red and green. Both App and ComposerApp already call Store.Search, so no composer-specific bindings change.

### Task 3: Browser and release handoff

Add `app/macos_contacts.go`, a small typed frontend binding adapter and `frontend/src/lib/components/settings/MacOSContacts.svelte`; mount in ContactsTab. Provide explicit access/browse/search and Open in Contacts, status/error text, no edit/delete actions. Add English/Finnish strings with locale fallback parity. Verify Go suite, Svelte check, frontend production build and diff whitespace. Update CHANGELOG, TASKS and native acceptance notes; commit and push the task branch for review. Live permission dialogs and Contacts.app handoff require a signed installed app and remain explicitly unverified here.
