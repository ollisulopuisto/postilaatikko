# P-011 security and recovery regression handoff

Base: origin/main `7e96afd`. Isolated branch: `codex/P-011-security-recovery`; worktree: `/private/tmp/postilaatikko-p011-security`. Claim `fecd682`; first security-test checkpoint `4033422`; credential fix `c930989`. No merge or release is performed by this task.

## Candidate provenance

Main already contains the preserved Send Later lifecycle safeguards. Compared with `origin/agent/p011-e2ee-scheduled-send`, it includes the encrypted preview/client lifecycle, queue caps and later arbitration changes. Compared with `origin/codex/p011-local-main-backup-20261026`, the scheduledsend differences are the release-baseline tests, Gmail transport route and checked cleanup changes. No historical candidate files were copied over newer main.

## Reproduced defect

`TestSecurityMissingLocalCredentialsRequireAuthorization` failed for the Gmail OAuth case before the fix; the SMTP-password control passed. A helper-local OAuth lookup failure occurred before a Gmail send request, but the transport's catch-all classified it as unknown provider acceptance. The Gmail token callback now returns the same typed authorization-required failure used by SMTP, and the caller preserves that known pre-submission result. The original diagnostic cause is retained through `Unwrap`; queue status and public errors remain fixed codes. Actual provider/transport ambiguity still becomes outcome_unknown. The credential test uses a temporary migrated application database and mocked keyring, never live credentials.

## Automated safety coverage

- Durable acceptance with a lost HTTP receipt, exact saved-byte retry, and rejection of old encrypted receipts for fresh status queries.
- One paired client cannot inspect, cancel or replace another client's job, even with identical sender permissions.
- A delayed queued-status receipt cannot overwrite a confirmed higher-revision cancellation.
- Two independent client SQLite connections race Send Now and scheduling; exactly one path wins.
- A claimed helper job cannot be concurrently delivered, cancelled or replaced through a second connection.
- Active claims retain the 15-minute grace period; interrupted claims become unknown, never automatically resend, and stale workers cannot overwrite recovery/manual decisions.
- Manual resolution requires the current revision and a confirmed sent/not-sent result; encrypted data is retained when repair is necessary.
- Safe retry, permanent failure, credential failure, ambiguous delivery, Sent-copy failure and the eight-attempt retry ceiling preserve their documented states.
- A SQLite trigger injects a completion write failure after transport acceptance; recovery preserves unknown status and never invokes transport again.

## Verification

Commands run from the isolated worktree, using `GOCACHE=/private/tmp/postilaatikko-p011-gocache`:

```sh
go test ./...
go test -race ./internal/scheduledsend
go vet ./internal/scheduledsend ./cmd/postilaatikko-helper
go build -o /private/tmp/postilaatikko-p011-helper ./cmd/postilaatikko-helper
git diff --check
```

Frontend assets must exist for the root Go embed. The first full-suite invocation passed all packages with tests but failed root setup because `frontend/dist` was absent. A local `npm --prefix frontend run build` generated the assets; it reported two existing titlebar ARIA warnings plus existing bundle/annotation notices. Dependencies were reused from the primary checkout after verifying lock differences were only root package version; the temporary node_modules symlink was removed afterward. The full Go suite was rerun after generating assets.

Final command outcomes are recorded in the task ledger. No frontend source changed.

## Remaining gates

This is focused regression coverage, not independent cryptographic review. Existing baseline tests cover tampered ciphertext, revoked pairing and old schedule replay after cancellation. OS-keychain lifecycle, Mac Studio login agent/reboot/sleep, real network/process interruption during actual submission, Gmail/SMTP acceptance and Sent-copy behavior, S/MIME/PGP/attachments/Bcc, and local/remote draft-edit/cleanup races require native/provider verification. The stale client-status race covered here is not a live composer draft-edit test. Rotation, retention/pruning, relay transport, attachment streaming and automatic Sent repair remain follow-ups. P-011 stays Review.
