# Gmail API Integration Contract and OAuth Scope Plan (P-017)

Status: proposal for review. This document changes no code, credentials or stored tokens. It unblocks P-010 (Gmail server-side search) and P-019 (Gmail-native spam and undo).

## 1. What exists today

Findings from the current code, with locations so the follow-up tasks can verify them.

- **Gmail accounts are IMAP accounts.** `AuthOAuth2` accounts with `tokens.Provider == "google"` authenticate IMAP and SMTP with XOAUTH2 (`internal/imap/xoauth2.go`, `internal/smtp/xoauth2.go`). There is no Gmail REST client.
- **Gmail detection is by host.** `App.isGmailAccount` and `sync/messages.go` compare `IMAPHost == "imap.gmail.com"`. `isTrustedGmailCondstore` additionally requires the live `X-GM-EXT-1` capability.
- **One broad scope is already granted.** `oauth2.GoogleProvider()` (`internal/oauth2/providers.go`) requests `https://mail.google.com/`, `userinfo.email`, `userinfo.profile` and `openid`. `https://mail.google.com/` is the only Google scope that IMAP/SMTP accept, and it also authorizes every Gmail REST method.
- **Separate consent per service.** The Google flow sets `include_granted_scopes=false`, `access_type=offline` and `prompt=consent` (`internal/oauth2/flow.go`). Mail, Contacts and Calendar each use their own client-config slot (`google-mail`, `google-contacts`, `google-calendar`, all resolved to one public Desktop client in `internal/oauth2/core_provider.go`) and their own token row keyed by `(account_id, client_config_id)`.
- **Token storage.** Access and refresh tokens live in the keyring with an encrypted-DB fallback; expiry, provider and scopes live in the `oauth_tokens` table (`internal/credentials/oauth.go`, `oauth_clientconfig.go`).
- **The stored `scopes` are the requested scopes, not the granted ones.** `app/oauth.go` writes `providerConfig.Scopes`, not the `scope` field of the token response (`oauth2.TokenResponse.Scope` exists but is not persisted). Google's granular consent lets a user deselect scopes, so the stored value can overstate the grant.
- **Refresh path.** `composeOps.getValidOAuthToken` refreshes when the token expires within 5 minutes, persists the result, and on failure emits the Wails event `oauth:reauth-required` with `accountId`, `provider` and `error`. `GetOAuthStatus` reports `NeedsReauth`. Extension HTTP traffic uses a separate refreshing transport (`internal/extensions/auth/transport.go`).
- **Search.** `sync.Engine.IMAPSearch` builds an IMAP SEARCH and returns UIDs plus envelopes. There is no provider abstraction around it.
- **Spam.** `MarkAsSpam*`, `MarkAsNotSpam*` are provider-neutral IMAP moves. For Gmail, `gmailTrashOrSpam` moves the message to `[Gmail]/Spam` through the cross-folder move path with undo.

## 2. Scope plan

Principle: request nothing new in this stage.

| Need | Smallest scope that works | Decision |
| --- | --- | --- |
| IMAP/SMTP (existing) | `https://mail.google.com/` | Keep. Required as long as IMAP is used. |
| Search with `q` (P-010) | `gmail.readonly` (or `gmail.modify`, or `mail.google.com`) | Covered by the existing grant. `gmail.metadata` is **not** acceptable: `q` is rejected under it. |
| Spam / not-spam via label change (P-019) | `gmail.modify` | Covered by the existing grant. |
| Settings (filters, aliases, signatures) | `gmail.settings.basic` | Out of scope here; needs a separate consent when someone claims it. |

Consequences:

1. Existing Google Mail accounts need no re-consent and no token migration. The Gmail API calls reuse the `google-mail` token row.
2. Narrowing the Mail grant to `gmail.modify` is not possible while IMAP/SMTP are used with the same token. Do not attempt it.
3. A separate consent flow is only needed for scopes outside `mail.google.com`, and the existing `include_granted_scopes=false` pattern already supports that as its own slot.
4. The Google Cloud project must have the Gmail API enabled (separate from OAuth scope approval). `mail.google.com` is a restricted scope; Google verification status of the shared Desktop client governs how many users can grant it. This is an operator/Google-project concern, not code.

**Capability detection (needed by P-010/P-019).** An account is Gmail-API capable when all hold:
- `AuthType == oauth2` and the Mail token row has provider `google` (not the custom "bring your own app" provider, whose endpoints and scopes are user-defined);
- the *granted* scope set contains `https://mail.google.com/`, or `gmail.modify`/`gmail.readonly` for the feature in question;
- the Gmail API has not recently answered `403 accessNotConfigured` or insufficient-scope for this account.

Because stored scopes are only the requested ones, P-010 must first persist the `scope` string returned by the token endpoint (a small change in `app/oauth.go` where tokens are saved and in refresh) and use that for the check. Until an account has been re-saved with granted scopes, treat a capability as unknown and let the first API call decide, falling back to IMAP on `403`.

## 3. Provider contract

New package `internal/gmail`, a thin REST client with no UI knowledge. Sketch:

```go
// Client talks to https://gmail.googleapis.com/gmail/v1/users/me.
type Client struct { /* http.Client with token source, base URL (overridable for tests) */ }

type SearchRequest struct {
    Query     string // Gmail syntax; caller may append label:/in: clauses
    LabelIDs  []string
    PageToken string
    Limit     int    // maxResults, cap 500
}
type SearchPage struct {
    MessageIDs    []string // Gmail API ids
    NextPageToken string
    EstimateTotal int
}

func (c *Client) Search(ctx context.Context, r SearchRequest) (*SearchPage, error)
func (c *Client) Metadata(ctx context.Context, ids []string) ([]Metadata, error) // format=metadata, Message-ID/Subject/From/Date headers + labelIds
func (c *Client) Modify(ctx context.Context, ids []string, add, remove []string) error // batchModify when len>1
```

Rules:

- **Token source.** The client takes a `func(ctx) (accessToken string, err error)` that delegates to the existing `getValidOAuthToken` logic. It must not implement a second refresh path or store tokens itself.
- **Retry.** On `401`, refresh once and retry once. On `429` or `403` with a rate-limit reason, honour `Retry-After` and use capped exponential backoff with jitter; on `5xx`, same, a few attempts. Bound every call with the caller's context.
- **Errors** are typed so callers can choose a fallback: `ErrReauthRequired` (refresh failed with `invalid_grant`; also emits the existing `oauth:reauth-required` event so the UI path is unchanged), `ErrInsufficientScope`, `ErrAPIDisabled` (`accessNotConfigured`), `ErrRateLimited`, `ErrTransient`, `ErrNotFound`.
- **Batching.** List returns only ids. Fetch metadata with batched `messages.get` (Gmail batch endpoint, bounded chunk size) rather than one request per id. Quota units are per method; keep search page size modest and page on demand.
- **No body fetching during search.** Metadata and headers only, consistent with the header-first sync rule in `AGENTS.md`.

### Mapping API results to local messages

Gmail API message ids are the hex form of the IMAP `X-GM-MSGID`. The local store does not persist `X-GM-MSGID`, so a result cannot be matched by id today. P-010 should choose one:

1. **Match by RFC `Message-ID` header** from the metadata response against the local store; for non-local hits, resolve the IMAP UID in the right folder with `UID SEARCH HEADER Message-ID` and reuse `fetchEnvelopesForSearch` / `FetchServerMessage`. No schema change.
2. Persist `X-GM-MSGID` during header sync (needs the fetch item and a migration; most efficient later, larger blast radius).

Recommendation: option 1 for P-010; revisit option 2 only if matching proves slow.

Gmail "folders" over IMAP are labels, and one API message can appear in several IMAP folders with different UIDs. Search results must therefore be deduplicated by Gmail id (or Message-ID) and mapped to the folder the user is browsing, or presented as conversation results.

## 4. IMAP fallback

IMAP stays the default and the fallback; the Gmail path is additive.

- Non-Gmail accounts, custom-OAuth Google accounts, password accounts and any account failing capability detection use the existing `IMAPSearch`, `markAsSpamWithUndo` and move paths unchanged.
- The Gmail path falls back to IMAP (for that call, with a log line, not a user-visible error unless both fail) on `ErrInsufficientScope`, `ErrAPIDisabled`, offline/network failure to `gmail.googleapis.com` when IMAP works, and unrecoverable `ErrTransient`.
- `ErrReauthRequired` is **not** silently masked: IMAP uses the same token and would fail too. Surface the existing re-authorization prompt.
- A per-account runtime flag (not persisted, reset at app start) remembers a recent `ErrAPIDisabled` / insufficient-scope result so each search does not pay for a doomed request.
- Unverified alternative worth a spike in P-010: IMAP `SEARCH X-GM-RAW "<query>"` accepts Gmail query syntax over the existing IMAP connection and returns UIDs directly, with no REST client, scope or id mapping. It is a Gmail IMAP extension that `go-imap` v2 does not expose, so it needs a raw command. If it works in practice it may be a better first implementation than REST for search; spam/label actions (P-019) would still use REST `modify`. The roadmap says to use `messages.list`; this note only records that a cheaper path may exist.

## 5. Notes for P-019 (spam and undo)

- Native action: `messages.modify` / `messages.batchModify` with `addLabelIds: ["SPAM"], removeLabelIds: ["INBOX"]`; undo is the reverse (`removeLabelIds: ["SPAM"], addLabelIds: ["INBOX"]`). These need `gmail.modify`, covered by the existing grant.
- Whether adding `SPAM` through the API trains Google's classifier the same way the web UI's "Report spam" does is unconfirmed (the roadmap already flags this). The current IMAP move to `[Gmail]/Spam` is label-equivalent; confirm the behavioral difference before presenting the API path as better.
- After an API label change the local IMAP view is stale: UIDs differ per folder and the message appears in a new mailbox. Reconcile through the existing sync (`CONDSTORE` / flag sweep) or update the local store explicitly. Keep the existing undo operation ids and the `runMoveMutation` coalescing so the UI contract (`SpamUndoResult`) does not change.
- Cross-account safety rule from `AGENTS.md` is unaffected: spam is an in-account label change, not a cross-account move.

## 6. Out of scope and open items

- No credential migration, no change to stored token schema, no new consent flow, no Gmail API enablement in this task.
- Persisting granted scopes (section 2) is a prerequisite change owned by whichever of P-010/P-019 lands first; the other should reuse it.
- Operator question for the coordinator: confirm the Gmail API is enabled in the Google Cloud project behind `GoogleClientID` and that the project's verification covers `mail.google.com`.
- Testing: the `Client` base URL must be injectable so P-010/P-019 can use `httptest` servers; no live Google calls in CI.

## 7. Acceptance criteria for the follow-ups

- **P-010:** Gmail accounts search online with Gmail query syntax, paged; results map to local messages or server-only envelopes; other providers unchanged; fallback to IMAP on the errors listed above; tests use a fake Gmail server.
- **P-019:** Gmail accounts use the native spam label change with undo and clear success/failure feedback; non-Gmail path unchanged; behavior difference from the IMAP move documented.

## 8. Implementation notes (P-010)

What landed for server-side search, and where it deviates from the sketch above.

- `internal/gmail` implements `Search`, `Metadata` and `Modify` with the error taxonomy in section 3. Differences from the sketch: `Metadata` fetches with bounded concurrency (8 single `messages.get` calls) instead of the multipart batch endpoint; the quota cost is the same and it avoids a hand-written multipart parser. The batch endpoint can replace it later without changing callers. `TokenSource` takes a `forceRefresh` flag so a 401 triggers exactly one forced refresh before `ErrReauthRequired`.
- Folder scoping: the IMAP folder type maps to a system label (`INBOX`, `SENT`, `DRAFT`, `TRASH`, `SPAM`, `STARRED`); All Mail and Archive search without a label; user folders use the `label:` operator (lowercased, spaces to hyphens, `/` kept). Unknown `[Gmail]/...` folders return `ErrNotCapable` and use IMAP.
- Mapping results to local messages uses the recommendation in section 3, option 1: the API returns Message-ID headers, then `UID SEARCH HEADER Message-ID` in the selected folder resolves UIDs, and the existing local-enrichment and envelope fetch (`resolveSearchUIDs`, shared with `IMAPSearch`) produces the response. No schema change. A message that is in the Gmail result but not in the selected IMAP folder is dropped.
- Fallback: `searchFolderOnServer` uses Gmail first and IMAP on any error except `ErrReauthRequired` (which emits the existing `oauth:reauth-required` event). `ErrInsufficientScope` and `ErrAPIDisabled` block the account for 30 minutes (in memory only), so a project without the Gmail API enabled costs one failed request per half hour, then searches silently use IMAP.
- Granted scopes: `oauth2.GrantedScopes` stores the token response `scope` instead of the requested list when an account is added or re-authorized. Accounts added earlier have the requested list (or none) stored and are treated as "unknown": the first API call decides.
- Not done here: the Gmail query syntax is passed through as typed, so IMAP-style plain words behave like Gmail's full-text search (not field-limited like the IMAP path). The frontend does not yet tell users that Gmail operators work.
- Open operator question, unchanged: the Gmail API must be enabled in the Google Cloud project behind `GoogleClientID`. Without it every Gmail account silently stays on IMAP search after one failed attempt, with a warning in the log.

## 9. Implementation notes (P-019)

- Scope: only Inbox -> Spam (mark as spam) and Spam -> Inbox (mark as not spam) on Gmail accounts whose granted scopes allow label changes (`ScopesCoverModify`) are done through the API; every other move, and any doubt, keeps the existing IMAP COPY+DELETE pipeline. For these two moves the two paths are label-equivalent (`SPAM` added and `INBOX` removed, or the reverse), so the provider-neutral path remains the fallback and its local-first move, undo and UID reconciliation are untouched.
- `Client.SetSpam` resolves every message by `rfc822msgid:` first (Spam and Trash included, exactly one match required) and only then calls `Modify`, so a failed lookup changes nothing and the IMAP path runs as before.
- The Gmail API returns no COPYUID, so the move takes the existing Message-ID baseline snapshot of the destination mailbox before the label change and resolves the new UIDs afterwards, retrying the lookup a few times because IMAP can lag a label change. If the UIDs cannot be resolved, the existing delayed destination sync reconciles, as it does for servers without UIDPLUS.
- Undo is unchanged: it is the generic move command in the other direction, which for Spam -> Inbox takes the same API path.
- Unconfirmed: whether adding `SPAM` through the API trains Google's classifier like the web UI's "Report spam". IMAP already moves into the server-side `[Gmail]/Spam` mailbox, which is the same label, so this change should not be presented to users as better filtering until that is confirmed with a real account.
