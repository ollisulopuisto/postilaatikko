package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/hkdb/aerion/internal/email/replycontent"
	"github.com/hkdb/aerion/internal/email/textrepair"
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
	if a.replyContent == nil {
		return nil, errors.New("reply content service is not ready")
	}
	msg, err := a.messageStore.Get(messageID)
	if err != nil {
		return nil, fmt.Errorf("failed to get message: %w", err)
	}
	if msg == nil {
		return nil, fmt.Errorf("message not found: %s", messageID)
	}
	if accountID != "" && msg.AccountID != accountID {
		return nil, fmt.Errorf("message %s does not belong to account %s", messageID, accountID)
	}

	if fetch && !msg.BodyFetched && !msg.BodyFailed {
		msg, err = a.FetchMessageBody(messageID)
		if err != nil {
			return nil, err
		}
	}

	msg.BodyText = textrepair.FinnishUTF8(msg.BodyText)
	reply, err := a.replyContent.Extract(a.replyContext(), channel, replycontent.RefOf(msg), replycontent.BodyFromMessage(msg))
	if err != nil {
		return nil, err
	}
	return &reply, nil
}

func (a *App) replyContext() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}
