# Scheduled-send rollback cleanup lint

CI on frontend-only PR #39 failed Go lint on two pre-existing unchecked deferred
`tx.Rollback()` calls in `internal/scheduledsend/store.go` (lines 159 and 247 on
main `fbb9b32`). Run: https://github.com/ollisulopuisto/postilaatikko/actions/runs/40687300032

Match the existing transaction-cleanup convention in `internal/message/store.go`
and `internal/message/gmail.go`: wrap each defer in `func() { _ = tx.Rollback() }`.
This explicitly acknowledges best-effort cleanup. It does not change which
errors are returned, when transactions commit/rollback, SQL statements, or the
checked explicit rollback on idempotent command replay. In particular, cleanup
must not replace the primary operation/Commit error, and a rollback after a
completed transaction returns `sql.ErrTxDone`.

No behavior change or new regression test is claimed. Existing scheduled-send
suite covers acceptance/replay, cancellation/stale revisions, pairing revocation,
and tamper/wrong-account rejection. `go test -race ./internal/scheduledsend` and
`go vet ./internal/scheduledsend` pass locally; `gofmt`/diff check pass. The full
project golangci-lint CI job is the decisive check and is not claimed from local
vet. No live email was sent and no scheduled-send delivery policy changed.
