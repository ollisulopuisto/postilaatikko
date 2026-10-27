package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestNativeMetadataAndRaw(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("format") == "raw" {
			_ = json.NewEncoder(w).Encode(map[string]string{"raw": base64.RawURLEncoding.EncodeToString([]byte("Subject: Test\r\n\r\nHello"))})
			return
		}
		if r.URL.Path == "/labels" {
			_, _ = w.Write([]byte(`{"labels":[{"id":"Label_1","name":"perhe/aarni","type":"user"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"m1","threadId":"t1","labelIds":["INBOX","UNREAD"],"payload":{"headers":[{"name":"To","value":"me@example.org"},{"name":"In-Reply-To","value":"<parent>"}]}}`))
	})
	ms, err := c.Metadata(context.Background(), []string{"m1"})
	if err != nil || len(ms) != 1 || ms[0].To != "me@example.org" || ms[0].InReplyTo != "<parent>" {
		t.Fatalf("metadata: %+v %v", ms, err)
	}
	raw, err := c.Raw(context.Background(), "m1")
	if err != nil || string(raw) != "Subject: Test\r\n\r\nHello" {
		t.Fatalf("raw: %s %v", raw, err)
	}
	labels, err := c.Labels(context.Background())
	if err != nil || len(labels) != 1 || labels[0].ID != "Label_1" {
		t.Fatalf("labels: %+v %v", labels, err)
	}
}
func TestSendAndInsertNeverRetryAmbiguousAcceptance(t *testing.T) {
	for _, operation := range []string{"send", "insert"} {
		t.Run(operation, func(t *testing.T) {
			calls := 0
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(503) })
			var err error
			if operation == "send" {
				_, err = c.Send(context.Background(), []byte("test"))
			} else {
				_, err = c.Insert(context.Background(), []byte("test"), []string{"INBOX"})
			}
			if err == nil || calls != 1 {
				t.Fatalf("ambiguous %s called %d times: %v", operation, calls, err)
			}
		})
	}
}

func TestGmailSendRetainsEnvelopeOnlyBccRecipients(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Raw string `json:"raw"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		raw, err := base64.RawURLEncoding.DecodeString(body.Raw)
		if err != nil {
			t.Error(err)
		}
		if !strings.Contains(string(raw), "Bcc: hidden@example.org\r\n") {
			t.Errorf("envelope-only recipient lost: %s", raw)
		}
		if strings.Contains(string(raw), "Bcc: visible@example.org") {
			t.Error("visible To recipient duplicated into Bcc")
		}
		_, _ = w.Write([]byte(`{"id":"sent"}`))
	})
	_, err := c.Send(context.Background(), []byte("From: me@example.org\r\nTo: visible@example.org\r\nSubject: Test\r\n\r\nHello"), "visible@example.org", "hidden@example.org")
	if err != nil {
		t.Fatal(err)
	}
}
