# Shared online counts

Task COUNT-REUSE, branch agent/approved-post-followup, based on main c766bc3. Separate from prepared P-047 refresh UI.

Effective account/provider identity, folder paths and all query/flag/date criteria key shared scans. Each window owns only its subscriber. Closing the last subscriber cancels active work, but exact completed results can be reused for one minute. Partial/error/cancelled work is not reusable. At most32sessions and64shared entries; expired completed entries are pruned. Relative date windows are therefore bounded by the same one-minute reuse policy.

Mail mutation/undo/sync events invalidate all subscribed snapshots, including results evicted from the shared cache. Explicit RefreshVirtualFolderCache invalidates before recomputing. Account/criteria changes naturally change the key. Conservative invalidation across accounts is intentional. Counts can become incomplete after a mutation until the next normal reload, never retained as exact. No new provider counting support.

Validation: full Go suite and app vet pass headlessly with a temporary linux !cgo NotifyStartupComplete shim; it is removed and is not part of the change. No normal GTK/native build or live Gmail validation. CGOoff cannot run race detection. Fake Gmail HTTP integration proves completed reuse and actual manual refresh rescan; additional tests cover key changes, cancellation isolation, non-exact/expired reuse refusal, mutation events and evicted snapshots. Frontend178/180 has the same two known baseline failures (Search close harness and removed nested mutation callback expectation); Svelte check0errors/2existingwarnings and production build pass. No UI change. No push/PR/CI/build artifact under publication hold.
