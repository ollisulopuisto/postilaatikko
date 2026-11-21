package sync

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/google/uuid"
	"github.com/ollisulopuisto/postilaatikko/internal/email/reactions"
	"github.com/ollisulopuisto/postilaatikko/internal/gmail"
	"github.com/ollisulopuisto/postilaatikko/internal/message"
	"mime"
	"strings"
	"sync"
	"time"
)

func partHeader(part *gmail.Part, name string) string {
	for _, h := range part.Headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

// Normal Gmail bodies use format=full; file payloads remain server-side until
// the attachment is opened/saved. Cryptographic MIME retains the raw parser.
func (e *Engine) gmailBody(ctx context.Context, c *gmail.Client, m *message.Message, id string) (*ParsedBody, error) {
	root, err := c.Full(ctx, id)
	if err != nil {
		return nil, err
	}
	typ := strings.ToLower(root.MIMEType)
	if strings.Contains(typ, "pkcs7") || strings.Contains(typ, "pgp") || typ == "multipart/signed" || typ == "multipart/encrypted" {
		raw, err := c.Raw(ctx, id)
		if err != nil {
			return nil, err
		}
		return e.parseMessageBodyFull(raw, m.ID, 30*time.Second), nil
	}
	parsed := &ParsedBody{}
	var visit func(*gmail.Part, int) error
	visit = func(part *gmail.Part, depth int) error {
		if depth > 50 {
			return fmt.Errorf("Gmail MIME nesting exceeds limit")
		}
		contentType := partHeader(part, "Content-Type")
		_, params, _ := mime.ParseMediaType(contentType)
		disposition, _, _ := mime.ParseMediaType(partHeader(part, "Content-Disposition"))
		if part.Filename != "" || disposition == "attachment" || (disposition == "inline" && partHeader(part, "Content-ID") != "") {
			att := &message.Attachment{ID: uuid.NewSHA1(uuid.NameSpaceURL, []byte(m.ID+":"+part.PartID+":"+part.Body.AttachmentID)).String(), MessageID: m.ID, Filename: part.Filename, ContentType: part.MIMEType, Size: part.Body.Size, ContentID: strings.Trim(partHeader(part, "Content-ID"), "<>"), IsInline: disposition == "inline"}
			if att.Filename == "" {
				att.Filename = "attachment"
			}
			if att.IsInline && part.Body.Size <= maxInlineContentSize {
				data, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(part.Body.Data, "="))
				if err != nil {
					return err
				}
				if part.Body.AttachmentID != "" {
					data, err = c.Attachment(ctx, id, part.Body.AttachmentID)
					if err != nil {
						return err
					}
				}
				att.Content = data
			}
			parsed.HasAttachments = true
			parsed.Attachments = append(parsed.Attachments, att)
			if e.attachmentStore != nil {
				if err = e.attachmentStore.Create(att); err != nil && !strings.Contains(err.Error(), "UNIQUE constraint") {
					return err
				}
				if err = e.attachmentStore.SetGmailReference(att.ID, id, part.Body.AttachmentID); err != nil {
					return err
				}
			}
			return nil
		}
		if part.MIMEType == "text/plain" || part.MIMEType == "text/html" || part.MIMEType == reactions.GoogleMIME {
			data, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(part.Body.Data, "="))
			if err != nil {
				return err
			}
			if part.Body.AttachmentID != "" {
				if part.Body.Size > maxPartSize {
					return fmt.Errorf("Gmail text part exceeds size limit")
				}
				data, err = c.Attachment(ctx, id, part.Body.AttachmentID)
				if err != nil {
					return err
				}
			}
			if info := reactions.ParsePart(part.MIMEType, partHeader(part, "Content-Disposition"), m.InReplyTo, data); info != nil {
				parsed.Reaction = info
			}
			text := decodeCharset(data, params["charset"])
			if part.MIMEType == "text/plain" {
				parsed.BodyText += text + "\n"
			}
			if part.MIMEType == "text/html" {
				parsed.BodyHTML += text + "\n"
			}
		}
		for i := range part.Parts {
			if err := visit(&part.Parts[i], depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err = visit(root, 0); err != nil {
		return nil, err
	}
	parsed.BodyText = strings.TrimSuffix(parsed.BodyText, "\n")
	parsed.BodyHTML = strings.TrimSuffix(parsed.BodyHTML, "\n")
	return parsed, nil
}
func (e *Engine) FetchGmailAttachment(ctx context.Context, accountID, localID string) ([]byte, error) {
	c, err := e.GmailClient(ctx, accountID)
	if err != nil {
		return nil, err
	}
	provider, attachment, err := e.attachmentStore.GmailReference(localID)
	if err != nil {
		return nil, err
	}
	att, err := e.attachmentStore.Get(localID)
	if err != nil || att == nil {
		return nil, fmt.Errorf("attachment not found")
	}
	m, err := e.messageStore.Get(att.MessageID)
	if err != nil || m == nil || m.AccountID != accountID {
		return nil, fmt.Errorf("attachment account mismatch")
	}
	if provider == "" {
		// Upgrade previously ingested data-bearing parts without changing their
		// local IDs. Other legacy metadata still uses the raw-MIME fallback.
		localUUID, parseErr := uuid.Parse(localID)
		if parseErr != nil || localUUID.Version() != 5 {
			return nil, gmail.ErrNotCapable
		}
		provider, err = e.GmailProviderID(ctx, c, m)
		if err != nil {
			return nil, err
		}
	}
	if attachment != "" {
		return c.Attachment(ctx, provider, attachment)
	}
	// Data-bearing Gmail parts have no attachment ID. Match the same opaque
	// part identity used at ingestion, never a guessed fallback filename.
	root, err := gmailAttachmentFull(ctx, c, accountID, provider)
	if err != nil {
		return nil, err
	}
	var result []byte
	found := false
	var visit func(*gmail.Part, int) error
	visit = func(part *gmail.Part, depth int) error {
		if depth > 50 {
			return fmt.Errorf("Gmail MIME nesting exceeds limit")
		}
		id := uuid.NewSHA1(uuid.NameSpaceURL, []byte(m.ID+":"+part.PartID+":"+part.Body.AttachmentID)).String()
		if id == localID {
			if found {
				return fmt.Errorf("ambiguous Gmail attachment identity")
			}
			found = true
			if part.Body.Size < 0 || part.Body.Size > maxMessageSize || len(part.Body.Data) > (maxMessageSize+2)/3*4 {
				return fmt.Errorf("Gmail attachment exceeds size limit")
			}
			if part.Body.AttachmentID != "" {
				result, err = c.Attachment(ctx, provider, part.Body.AttachmentID)
			} else {
				result, err = base64.RawURLEncoding.DecodeString(strings.TrimRight(part.Body.Data, "="))
			}
			if err != nil {
				return err
			}
			if len(result) > maxMessageSize || len(result) != part.Body.Size {
				return fmt.Errorf("incomplete Gmail attachment")
			}
		}
		for i := range part.Parts {
			if err := visit(&part.Parts[i], depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err = visit(root, 0); err != nil {
		return nil, err
	}
	if !found {
		return nil, gmail.ErrNotFound
	}
	return result, nil
}

type gmailAttachmentCacheKey struct{}
type gmailAttachmentCache struct {
	mu       sync.Mutex
	messages map[[2]string]*gmail.Part
}

// WithGmailAttachmentCache scopes full-message reuse to one explicit read batch.
// It never persists provider content or shares it across requests/accounts.
func WithGmailAttachmentCache(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, gmailAttachmentCacheKey{}, &gmailAttachmentCache{messages: make(map[[2]string]*gmail.Part)})
}
func gmailAttachmentFull(ctx context.Context, c *gmail.Client, accountID, provider string) (*gmail.Part, error) {
	cache, _ := ctx.Value(gmailAttachmentCacheKey{}).(*gmailAttachmentCache)
	if cache == nil {
		return c.Full(ctx, provider)
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	key := [2]string{accountID, provider}
	if root := cache.messages[key]; root != nil {
		return root, nil
	}
	root, err := c.Full(ctx, provider)
	if err != nil {
		return nil, err
	}
	cache.messages[key] = root
	return root, nil
}
