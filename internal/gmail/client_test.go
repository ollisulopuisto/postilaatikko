package gmail

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := NewClient(func(ctx context.Context, force bool) (string, error) {
		if force {
			return "fresh-token", nil
		}
		return "token", nil
	})
	c.BaseURL = srv.URL
	c.sleep = func(ctx context.Context, d time.Duration) error { return nil }
	return c
}

func writeAPIError(w http.ResponseWriter, status int, reason, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    status,
			"message": message,
			"errors":  []map[string]string{{"reason": reason}},
		},
	})
}

func TestSearchSendsQueryAndPages(t *testing.T) {
	var gotAuth, gotQuery, gotMax, gotPage, gotLabels string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Errorf("path = %q, want /messages", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.Query().Get("q")
		gotMax = r.URL.Query().Get("maxResults")
		gotPage = r.URL.Query().Get("pageToken")
		gotLabels = strings.Join(r.URL.Query()["labelIds"], ",")
		_, _ = w.Write([]byte(`{"messages":[{"id":"a1","threadId":"t1"},{"id":"a2","threadId":"t2"}],"nextPageToken":"next","resultSizeEstimate":42}`))
	})

	page, err := c.Search(context.Background(), SearchRequest{Query: "from:bob has:attachment", LabelIDs: []string{"INBOX"}, PageToken: "p1", Limit: 25})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotAuth != "Bearer token" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotQuery != "from:bob has:attachment" || gotMax != "25" || gotPage != "p1" || gotLabels != "INBOX" {
		t.Errorf("params q=%q max=%q page=%q labels=%q", gotQuery, gotMax, gotPage, gotLabels)
	}
	if len(page.MessageIDs) != 2 || page.MessageIDs[0] != "a1" || page.NextPageToken != "next" || page.EstimateTotal != 42 {
		t.Errorf("page = %+v", page)
	}
}

func TestSearchLimitIsCapped(t *testing.T) {
	var gotMax string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMax = r.URL.Query().Get("maxResults")
		_, _ = w.Write([]byte(`{}`))
	})
	if _, err := c.Search(context.Background(), SearchRequest{Query: "x", Limit: 9999}); err != nil {
		t.Fatal(err)
	}
	if gotMax != "500" {
		t.Errorf("maxResults = %q, want 500", gotMax)
	}
}

func TestMetadataParsesHeadersAndLabels(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/messages/")
		if r.URL.Query().Get("format") != "metadata" {
			t.Errorf("format = %q", r.URL.Query().Get("format"))
		}
		_, _ = w.Write([]byte(`{"id":"` + id + `","threadId":"t","labelIds":["INBOX","UNREAD"],"snippet":"hi","internalDate":"1700000000000","payload":{"headers":[{"name":"Message-ID","value":"<abc@example.com>"},{"name":"Subject","value":"Hello"}]}}`))
	})
	got, err := c.Metadata(context.Background(), []string{"m1", "m2", "m3"})
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if len(got) != 3 || got[0].ID != "m1" || got[2].ID != "m3" {
		t.Fatalf("order not preserved: %+v", got)
	}
	if got[0].RFCMessageID != "<abc@example.com>" || got[0].Subject != "Hello" || got[0].Snippet != "hi" {
		t.Errorf("fields: %+v", got[0])
	}
	if len(got[0].LabelIDs) != 2 || got[0].Date.Unix() != 1700000000 {
		t.Errorf("labels/date: %+v", got[0])
	}
}

func TestMetadataSkipsMessagesThatVanished(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/gone") {
			writeAPIError(w, http.StatusNotFound, "notFound", "Requested entity was not found.")
			return
		}
		_, _ = w.Write([]byte(`{"id":"ok","payload":{"headers":[]}}`))
	})
	got, err := c.Metadata(context.Background(), []string{"ok", "gone"})
	if err != nil {
		t.Fatalf("a deleted message must not fail the whole lookup: %v", err)
	}
	if len(got) != 1 || got[0].ID != "ok" {
		t.Errorf("got %+v", got)
	}
}

func TestModifyAndBatchModify(t *testing.T) {
	var paths []string
	var bodies []map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		bodies = append(bodies, b)
		_, _ = w.Write([]byte(`{}`))
	})
	if err := c.Modify(context.Background(), []string{"one"}, []string{"SPAM"}, []string{"INBOX"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Modify(context.Background(), []string{"a", "b"}, []string{"SPAM"}, []string{"INBOX"}); err != nil {
		t.Fatal(err)
	}
	if paths[0] != "POST /messages/one/modify" || paths[1] != "POST /messages/batchModify" {
		t.Errorf("paths = %v", paths)
	}
	if ids, _ := bodies[1]["ids"].([]any); len(ids) != 2 {
		t.Errorf("batch ids = %v", bodies[1]["ids"])
	}
	if add, _ := bodies[0]["addLabelIds"].([]any); len(add) != 1 || add[0] != "SPAM" {
		t.Errorf("addLabelIds = %v", bodies[0]["addLabelIds"])
	}
}

func TestErrorClassification(t *testing.T) {
	tests := []struct {
		name   string
		status int
		reason string
		want   error
	}{
		{"scope", http.StatusForbidden, "insufficientPermissions", ErrInsufficientScope},
		{"api disabled", http.StatusForbidden, "accessNotConfigured", ErrAPIDisabled},
		{"service disabled", http.StatusForbidden, "SERVICE_DISABLED", ErrAPIDisabled},
		{"not found", http.StatusNotFound, "notFound", ErrNotFound},
		{"bad request", http.StatusBadRequest, "invalidArgument", ErrBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				writeAPIError(w, tt.status, tt.reason, "msg")
			})
			_, err := c.Search(context.Background(), SearchRequest{Query: "x"})
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestUnauthorizedRefreshesOnceThenSucceeds(t *testing.T) {
	var calls int32
	var tokens []string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		tokens = append(tokens, r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") == "Bearer token" {
			writeAPIError(w, http.StatusUnauthorized, "authError", "Invalid Credentials")
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})
	if _, err := c.Search(context.Background(), SearchRequest{Query: "x"}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if calls != 2 || tokens[1] != "Bearer fresh-token" {
		t.Errorf("calls=%d tokens=%v", calls, tokens)
	}
}

func TestUnauthorizedAfterRefreshIsReauthRequired(t *testing.T) {
	var calls int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		writeAPIError(w, http.StatusUnauthorized, "authError", "Invalid Credentials")
	})
	_, err := c.Search(context.Background(), SearchRequest{Query: "x"})
	if !errors.Is(err, ErrReauthRequired) {
		t.Fatalf("err = %v, want ErrReauthRequired", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want exactly one refresh retry", calls)
	}
}

func TestTokenSourceErrorIsReturned(t *testing.T) {
	c := NewClient(func(ctx context.Context, force bool) (string, error) { return "", ErrNotCapable })
	_, err := c.Search(context.Background(), SearchRequest{Query: "x"})
	if !errors.Is(err, ErrNotCapable) {
		t.Fatalf("err = %v", err)
	}
}

func TestRateLimitRetriesWithRetryAfterThenFails(t *testing.T) {
	var calls int32
	var slept []time.Duration
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Retry-After", "2")
		writeAPIError(w, http.StatusTooManyRequests, "rateLimitExceeded", "slow down")
	})
	c.sleep = func(ctx context.Context, d time.Duration) error { slept = append(slept, d); return nil }
	_, err := c.Search(context.Background(), SearchRequest{Query: "x"})
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	if calls != int32(maxAttempts) {
		t.Errorf("calls = %d, want %d", calls, maxAttempts)
	}
	if len(slept) == 0 || slept[0] < 2*time.Second {
		t.Errorf("Retry-After not honored: %v", slept)
	}
}

func TestServerErrorRetriesThenRecovers(t *testing.T) {
	var calls int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			writeAPIError(w, http.StatusBadGateway, "backendError", "oops")
			return
		}
		_, _ = w.Write([]byte(`{"messages":[{"id":"z"}]}`))
	})
	page, err := c.Search(context.Background(), SearchRequest{Query: "x"})
	if err != nil || len(page.MessageIDs) != 1 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}

func TestContextCancelStopsRetries(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeAPIError(w, http.StatusServiceUnavailable, "backendError", "down")
	})
	ctx, cancel := context.WithCancel(context.Background())
	c.sleep = func(ctx context.Context, d time.Duration) error { cancel(); return ctx.Err() }
	_, err := c.Search(ctx, SearchRequest{Query: "x"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
