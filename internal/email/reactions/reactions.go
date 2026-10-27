// Package reactions recognizes structured Gmail/RFC reactions and conservative
// provider fallback notifications. Unknown input remains an ordinary message.
package reactions

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/forPelevin/gomoji"
	"golang.org/x/net/html"
)

const GoogleMIME = "text/vnd.google.email-reaction+json"

// Info identifies an emoji and the exact RFC Message-ID to which it applies.
// Invalid records prevent malformed structured reactions falling back to a
// coincidentally matching notification body.
type Info struct {
	Emoji           string `json:"emoji,omitempty"`
	TargetMessageID string `json:"targetMessageId,omitempty"`
	Provider        string `json:"provider"`
	Invalid         bool   `json:"invalid,omitempty"`
}

var messageID = regexp.MustCompile(`^<?([^<>\s@]+@[^<>\s@]+)>?$`)

// Target accepts one ID, never a list or a previous-message guess.
func Target(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 998 || strings.HasPrefix(value, "<") != strings.HasSuffix(value, ">") {
		return ""
	}
	match := messageID.FindStringSubmatch(value)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func isEmoji(value string) bool {
	if len(value) > 128 {
		return false
	}
	_, err := gomoji.GetInfo(value)
	return err == nil
}

// ParsePart reads a transfer-decoded MIME body. Nil means an unrelated part.
func ParsePart(contentType, disposition, target string, data []byte) *Info {
	if disposition == "attachment" || (contentType != GoogleMIME && !(contentType == "text/plain" && disposition == "reaction")) {
		return nil
	}
	info := &Info{Provider: "gmail", TargetMessageID: Target(target)}
	if contentType == GoogleMIME {
		var payload struct {
			Version int    `json:"version"`
			Emoji   string `json:"emoji"`
		}
		if len(data) > 1024 || json.Unmarshal(data, &payload) != nil || payload.Version != 1 {
			info.Invalid = true
			return info
		}
		info.Emoji = payload.Emoji
	} else {
		info.Provider = "rfc9078"
		info.Emoji = strings.TrimSpace(string(data))
	}
	info.Invalid = info.TargetMessageID == "" || !isEmoji(info.Emoji)
	return info
}

// FromNotification requires the complete authored fallback to match the sender
// and a known reaction sentence. Ordinary emoji replies and digests stay whole.
func FromNotification(sender, target, text, bodyHTML string) *Info {
	target = Target(target)
	if target == "" || len(text)+len(bodyHTML) > 64<<10 {
		return nil
	}
	if bodyHTML != "" {
		text = notificationText(bodyHTML)
	}
	text = strings.Join(strings.Fields(text), " ")
	sender = strings.Join(strings.Fields(sender), " ")
	if sender == "" {
		return nil
	}
	for _, suffix := range []string{" reacted to your message:", " reacted to your message.", " reagoi viestiisi:"} {
		if !strings.HasSuffix(text, suffix) {
			continue
		}
		prefix := strings.TrimSuffix(text, suffix)
		emoji, name, ok := strings.Cut(prefix, " ")
		if ok && isEmoji(emoji) && strings.EqualFold(name, sender) {
			return &Info{Emoji: emoji, TargetMessageID: target, Provider: "notification"}
		}
	}
	return nil
}

func notificationText(value string) string {
	doc, err := html.Parse(strings.NewReader(value))
	if err != nil {
		return ""
	}
	var out strings.Builder
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if n.Data == "script" || n.Data == "style" || n.Data == "head" || n.Data == "blockquote" {
				return
			}
			for _, a := range n.Attr {
				if (a.Key == "class" || a.Key == "id") && (strings.Contains(a.Val, "gmail_quote") || strings.Contains(a.Val, "yahoo_quoted")) {
					return
				}
			}
			if n.Data == "img" {
				for _, a := range n.Attr {
					if a.Key == "alt" && isEmoji(a.Val) {
						out.WriteString(a.Val)
					}
				}
			}
			if n.Data == "div" || n.Data == "p" || n.Data == "br" {
				out.WriteByte(' ')
			}
		}
		if n.Type == html.TextNode {
			out.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
		if n.Type == html.ElementNode && (n.Data == "div" || n.Data == "p") {
			out.WriteByte(' ')
		}
	}
	visit(doc)
	return out.String()
}
