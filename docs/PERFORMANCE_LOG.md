# One-switch performance capture

Start the desktop executable with `--perf-log`. No environment variables,
profiling server, curl request, helper restart, or upload is needed.
Do not combine the switch with a mailto argument. That combination is rejected
before starting or activating the app. For the current macOS bundle this is:

```sh
/Applications/Postilaatikko.app/Contents/MacOS/eterno-mail --perf-log
```

The binary name above is the existing release name, not a requirement of this
feature. If Postilaatikko is already running, the switch asks that instance to
start the capture through its existing local activation socket. Use the same
installed version for both processes. An older running app cannot understand
this new activation command and must first be quit normally.

## Files and duration

- macOS: `~/Library/Logs/Postilaatikko/perf/<UTC-session-id>/`
- Linux: `$XDG_CACHE_HOME/Postilaatikko/perf/<UTC-session-id>/`, normally
  `~/.cache/Postilaatikko/perf/<UTC-session-id>/`
- Windows: the user cache directory under `Postilaatikko/perf/<UTC-session-id>/`

Each process has its own `desktop/` or `helper/` directory. The path is printed
to stderr when capture starts. The session runs for five minutes or until the
app quits. CPU capture covers the first minute. Heap and goroutine profiles
are saved at the start and end; `performance.jsonl` contains timings and capture
status, including failures and whether the helper acknowledged capture. The first
file I/O failure is also reported to stderr, separately from the capture file.
The helper stops on its own five-minute deadline even if the desktop exits,
and finalizes active profiles on normal helper shutdown.
No running-app restart is required once both binaries support this feature.

Directories are private (0700) and files are private (0600). Existing symlinked
diagnostics directories are rejected. JSON logs are capped at 8 MiB per process;
profiles vary with process size. Sessions are not deleted automatically. Delete
the session directory when no longer needed.

## Helper and privacy

The desktop records its existing API requests and requests helper capture over
its existing authenticated connection to a literal loopback IP only. The helper
requires the desktop's existing mail-read capability, a recent session ID, and
both a loopback peer and Host. No profiling port is opened. Remote helpers do
not start profiling. An old, unreachable, or insufficiently authorized helper
is reported in the desktop capture status instead of being restarted or having
its permissions changed.

JSON timings omit mailbox IDs, request URLs, bodies, tokens, query strings, and
provider responses. Helper routes use handler patterns, not actual resource
paths. Profiles are Go runtime profiles: they can contain function names,
source paths, stack details, and allocation metadata. Treat all capture files
as private. Nothing uploads them automatically.

This does not capture WebKit/JavaScript CPU, rendered-frame timings, native
window painting, or system-wide CPU. Desktop API timing and helper Go profiles
are separate, so a quiet desktop profile alone cannot establish where a slow
mail operation spent its time. The old opt-in environment profiling flags
remain supported but are unnecessary for this workflow.
