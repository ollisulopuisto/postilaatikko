package sync

import (
	"github.com/rs/zerolog"
	"testing"
)

func TestParseGmailReactionMIME(t *testing.T) {
	raw := []byte("From: Example Sender <sender@example.com>\r\nIn-Reply-To: <target@example.com>\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=parts\r\n\r\n--parts\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nFallback notification text\r\n--parts\r\nContent-Type: text/vnd.google.email-reaction+json\r\nContent-Transfer-Encoding: base64\r\n\r\neyJ2ZXJzaW9uIjoxLCJlbW9qaSI6IvCfkY0ifQ==\r\n--parts\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>Fallback notification HTML</p>\r\n--parts--\r\n")
	engine := &Engine{log: zerolog.Nop()}
	parsed := engine.parseMessageBodyInternal(raw, "message")
	if parsed.Reaction == nil || parsed.Reaction.Invalid || parsed.Reaction.Emoji != "👍" || parsed.Reaction.TargetMessageID != "target@example.com" {
		t.Fatalf("metadata: %+v", parsed.Reaction)
	}
	if parsed.BodyText == "" || parsed.BodyHTML == "" {
		t.Fatal("fallback body lost")
	}
}

func TestParseSinglePartAndInvalidReactions(t *testing.T) {
	engine := &Engine{log: zerolog.Nop()}
	for _, data := range []string{`{"version":1,"emoji":"👍"}`, `{"version":99,"emoji":"👍"}`} {
		raw := []byte("In-Reply-To: <target@example.com>\r\nContent-Type: text/vnd.google.email-reaction+json\r\n\r\n" + data)
		result := engine.parseMessageBodyInternal(raw, "message")
		if result.Reaction == nil || result.BodyText == "" {
			t.Fatalf("single part lost: %+v", result)
		}
		if (data == `{"version":1,"emoji":"👍"}`) == result.Reaction.Invalid {
			t.Fatalf("wrong validity: %+v", result.Reaction)
		}
	}
}
