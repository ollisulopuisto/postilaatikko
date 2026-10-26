package replycontent

import (
	"testing"

	"github.com/hkdb/aerion/internal/message"
	"github.com/hkdb/aerion/internal/views"
)

func TestBodyFromMessage(t *testing.T) {
	plain := &message.Message{ID: "m1", AccountID: "acc", BodyText: "t", BodyHTML: "<p>h</p>", BodyFetched: true}
	if got := BodyFromMessage(plain); got != (Body{Text: "t", HTML: "<p>h</p>", Fetched: true}) {
		t.Errorf("plain message: %+v", got)
	}
	if got := RefOf(plain); got != (views.MessageRef{AccountID: "acc", MessageID: "m1"}) {
		t.Errorf("RefOf = %+v", got)
	}

	failed := BodyFromMessage(&message.Message{BodyFailed: true})
	if !failed.Failed || failed.Fetched {
		t.Errorf("failed message: %+v", failed)
	}

	for _, m := range []*message.Message{
		{BodyFetched: true, SMIMEEncrypted: true},
		{BodyFetched: true, PGPEncrypted: true},
	} {
		if !BodyFromMessage(m).Sensitive {
			t.Errorf("encrypted message %+v is not marked sensitive", m)
		}
	}
}
