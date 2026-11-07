package app

import (
	"github.com/hkdb/aerion/internal/email/replycontent"
)

// GetReplyContent returns the reply extraction (new text, quoted history,
// signature) of a message whose body is already stored. It never downloads
// anything unless fetch is true, and then it uses the same body fetch as
// FetchMessageBody. The stored body is not changed.
//
// channel names the caller's view, for example "reader". A newer call on the
// same channel supersedes an older one still running, which then returns an
// error instead of a stale result. Messages with an encrypted body are
// analyzed from the stored body only and are never cached.
func (a *App) GetReplyContent(accountID, messageID, channel string, fetch bool) (*replycontent.Reply, error) {
	return a.currentMailService().GetReplyContent(a.ctx, accountID, messageID, channel, fetch)
}
