# Bounded recurrence

The local `rrule/rrule.go` and `rrule/util.go` are copied from
`github.com/teambition/rrule-go` v1.8.2, already used by the application.
The upstream LICENSE is retained in that directory. No global module replacement
or runtime download is introduced.

Review the engine delta against that exact upstream version. The changes are:

- a private callback on the iterator, checked in every data-dependent generation
  loop, including the hour/minute/second advancement loops that can otherwise
  run forever for impossible interval/filter combinations;
- `IteratorWithCheck`, leaving the original iterator behavior intact;
- one weekday conversion constructor for the adapter.

`Between` converts already-parsed upstream options, preserves RDATE/EXDATE and
sorted unique inclusive results, and returns no partial result on a limit or
cancellation. The caller shares a work budget across events. It also bounds
Cartesian time-set construction before constructing the local rule.

Call `ValidateRuleText` for each ICS RRULE BEFORE asking go-ical to construct its
upstream recurrence set. This prevents an oversized Cartesian product before the
bounded iterator is reached. Stored ICS itself must have a separate byte limit.
The bounded path rejects the non-standard BYEASTER option (unchecked upstream mask indexes), intervals above 1,000,000 and option/time-set products above its documented limits. There are no timeout goroutines. Provider reads/writes and alarm delivery are not
part of this package.

Tests compare ordinary rules and Helsinki spring/autumn DST, month-end, leap-day,
weekday/ordinal/BYSETPOS, RDATE/EXDATE, duplicates and inclusive boundaries against
the existing engine. Impossible internal-loop rules and cancellation are tested
without evaluating them through the original unbounded iterator.
