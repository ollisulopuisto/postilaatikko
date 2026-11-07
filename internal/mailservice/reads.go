package mailservice

import (
	"context"
	"errors"
	"fmt"

	"github.com/hkdb/aerion/internal/email/replycontent"
	"github.com/hkdb/aerion/internal/email/textrepair"
	"github.com/hkdb/aerion/internal/message"
)

// GetMessage returns a stored message, optionally requiring it to belong to
// accountID when a caller has an account-scoped request.
func (s *Service) GetMessage(accountID, messageID string) (*message.Message, error) {
	if s == nil || s.messageStore == nil {
		return nil, errors.New("message service is not ready")
	}
	msg, err := s.messageStore.Get(messageID)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, fmt.Errorf("message not found: %s", messageID)
	}
	if accountID != "" && msg.AccountID != accountID {
		return nil, fmt.Errorf("message %s does not belong to account %s", messageID, accountID)
	}
	return msg, nil
}

// FetchMessageBody fetches full content only when explicitly requested by the
// caller. Cached bodies are returned without contacting the provider.
func (s *Service) FetchMessageBody(ctx context.Context, accountID, messageID string) (*message.Message, error) {
	msg, err := s.GetMessage(accountID, messageID)
	if err != nil {
		return nil, err
	}
	if msg.BodyFetched {
		return msg, nil
	}
	if s.syncEngine == nil {
		return nil, errors.New("mail sync service is not ready")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	updated, err := s.syncEngine.FetchMessageBody(ctx, msg.AccountID, messageID)
	if err != nil {
		return nil, err
	}
	if updated == nil || updated.AccountID != msg.AccountID {
		return nil, errors.New("provider returned a message outside the requested account")
	}
	return updated, nil
}

// GetReplyContent extracts reply text from stored content, optionally fetching
// the body first when requested. It preserves the extractor's per-channel
// supersession behavior for concurrent reads.
func (s *Service) GetReplyContent(ctx context.Context, accountID, messageID, channel string, fetch bool) (*replycontent.Reply, error) {
	if s == nil || s.replyContent == nil {
		return nil, errors.New("reply content service is not ready")
	}
	msg, err := s.GetMessage(accountID, messageID)
	if err != nil {
		return nil, err
	}
	if fetch && !msg.BodyFetched && !msg.BodyFailed {
		msg, err = s.FetchMessageBody(ctx, accountID, messageID)
		if err != nil {
			return nil, err
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	msg.BodyText = textrepair.FinnishUTF8(msg.BodyText)
	reply, err := s.replyContent.Extract(ctx, channel, replycontent.RefOf(msg), replycontent.BodyFromMessage(msg))
	if err != nil {
		return nil, err
	}
	return &reply, nil
}
