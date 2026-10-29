# Responsiveness pass — 2026-10-29

## Scope and findings

The message list and conversation reader each polled the shared dialog guard
every 500 ms. The list uses the poll to resume a sync-triggered reload after a
blocking dialog closes; the reader uses it to resume a thread refresh. These
callbacks do no network or database work, but they wake every mounted component
twice per second even when no dialog is open. With the primary workspace and
four additional tabs, the static upper bound is ten such timers (five lists
and five open readers), or 20 callbacks per second while idle.

The first production build also exposed an oversized offline Iconify chunk:
16,547.21 kB raw / 5,660.12 kB gzip. `iconify-offline.ts` imported all glyphs
from five complete collections although this checkout references only a small
subset.

## Change

Dialog guard state now notifies subscribers only when the count changes from
zero to one or one to zero. Message lists and readers resume deferred work on
the zero transition and unsubscribe when destroyed. Nested dialogs still keep
the guard active until the last one closes. The guard also ignores an unmatched
close instead of allowing its count to become negative.

A prebuild/predev generator scans the host frontend and bundled extensions for
Iconify names, resolves aliases, and writes only those glyphs to a generated
collection. The UI registers that subset instead of importing five full icon
sets. Three stale icon references that were absent from the installed sets
were changed to available equivalents. Vite no longer prebundles those large
icon JSON packages during development.

The icon chunk fell from 16,547.21 kB raw / 5,660.12 kB gzip to 482.14 kB raw /
138.61 kB gzip: about 97.1% less raw icon data and 97.5% less compressed data
in that chunk. This is a build-size comparison from the same checkout and
toolchain, not a measured startup-time or RSS delta. The polling change removes
the recurring timers and their idle callbacks; it does not alter mailbox fetch
policy, list pagination, tab retention, or body/attachment prefetch.

## Evidence and limits

The baseline is a source-level count of two 500 ms intervals and the workspace
cap of five mounted mail lists. The candidate has no interval in either
component; callbacks occur only on actual guard transitions. This is a direct
count of scheduled callbacks, not a measured end-to-end latency or memory
benchmark. Because the old callbacks only checked a boolean, the expected gain
is reduced idle wakeups and scheduling overhead; no claim is made that this
alone makes all mail actions perceptibly faster.

Actual CPU, memory, folder-switch latency and reader latency still need
measurement with a large real mailbox on macOS. Those measurements should use
the same account, folder, thread and build before and after; they are not
available in this source-only pass. Icon names are discovered from the host
source and bundled extensions; an external extension that supplies a new
Iconify name at runtime needs a future registration path.
