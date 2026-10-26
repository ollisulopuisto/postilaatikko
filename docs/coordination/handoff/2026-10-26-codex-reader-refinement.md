# Reader refinement ownership

UX-008 and UX-009 are merged in main at 0ef36e4. User now requests compact
conversation bubbles, fewer repeated headers and signature/source controls,
and double-click opening a separate reader. Codex claims READER-001.

Files: viewer ConversationViewer/ConversationMessage/EmailBody and small new
reader presentation helpers; list ConversationRow; main.go and new detached
reader/IPC adapter; replycontent parser adapter and tests if library evaluation
supports reuse; generated bindings; en/fi and documentation. UX-015/017/018 are
not started. Existing viewer props/events stay compatible. No main merge.
