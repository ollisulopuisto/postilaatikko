package sync

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/google/uuid"
	"github.com/hkdb/aerion/internal/email/reactions"
	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/message"
	"mime"
	"strings"
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
				if part.Body.AttachmentID != "" {
					if err = e.attachmentStore.SetGmailReference(att.ID, id, part.Body.AttachmentID); err != nil {
						return err
					}
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
	if provider == "" || attachment == "" {
		return nil, gmail.ErrNotCapable
	}
	att, err := e.attachmentStore.Get(localID)
	if err != nil || att == nil {
		return nil, fmt.Errorf("attachment not found")
	}
	m, err := e.messageStore.Get(att.MessageID)
	if err != nil || m == nil || m.AccountID != accountID {
		return nil, fmt.Errorf("attachment account mismatch")
	}
	return c.Attachment(ctx, provider, attachment)
}
