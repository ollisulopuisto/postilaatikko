package replycontent

import (
	"github.com/hkdb/aerion/internal/message"
	"github.com/hkdb/aerion/internal/views"
)

// RefOf is the account-qualified reference of a stored message.
func RefOf(m *message.Message) views.MessageRef {
	return views.MessageRef{AccountID: m.AccountID, MessageID: m.ID}
}

// BodyFromMessage maps a stored message to the body the service analyzes.
// Encrypted messages are marked sensitive so nothing derived from them is
// cached; the stored body of such a message is not the decrypted text, and
// extraction from decrypted text is a separate, explicit step.
func BodyFromMessage(m *message.Message) Body {
	return Body{
		Text:      m.BodyText,
		HTML:      m.BodyHTML,
		Fetched:   m.BodyFetched,
		Failed:    m.BodyFailed,
		Sensitive: m.SMIMEEncrypted || m.PGPEncrypted,
	}
}
