package gmail

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCompleteThreadCountEnumeratesPagesAndIgnoresEstimate(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/threads" {
			t.Errorf("count downloaded message data: %s", r.URL)
			w.WriteHeader(500)
			return
		}
		calls++
		q := r.URL.Query().Get("q")
		page := r.URL.Query().Get("pageToken")
		if q == "is:unread" {
			_, _ = w.Write([]byte(`{"threads":[{"id":"b"},{"id":"outside"}]}`))
			return
		}
		if page == "" {
			_, _ = w.Write([]byte(`{"threads":[{"id":"a"},{"id":"b"}],"nextPageToken":"last","resultSizeEstimate":9999}`))
			return
		}
		_, _ = w.Write([]byte(`{"threads":[{"id":"b"},{"id":"c"}],"resultSizeEstimate":9999}`))
	})
	count, err := c.CountThreads(context.Background(), CountQuery{MembershipQuery: `label:"Family"`}, nil)
	if err != nil || !count.Complete || count.Total != 3 || count.Unread != 1 {
		t.Fatalf("count: %+v %v", count, err)
	}
	if calls != 3 {
		t.Fatalf("expected two member pages and one unread page, got %d", calls)
	}
}
func TestCountFiltersAreThreadWideAcrossDifferentMessages(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/threads" {
			ids := []string{"a", "b"}
			switch r.URL.Query().Get("q") {
			case "is:unread":
				ids = []string{"a"}
			case "is:starred":
				ids = []string{"a"}
			}
			threads := []map[string]string{}
			for _, id := range ids {
				threads = append(threads, map[string]string{"id": id})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"threads": threads})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/threads/") {
			_, _ = w.Write([]byte(`{"messages":[{"id":"one","internalDate":"1000","labelIds":["UNREAD"]},{"id":"two","internalDate":"3000","labelIds":["SENT","STARRED"],"payload":{"headers":[{"name":"In-Reply-To","value":"<one>"}]}}]}`))
			return
		}
		t.Errorf("unexpected body download %s", r.URL)
		w.WriteHeader(500)
	})
	count, err := c.CountThreads(context.Background(), CountQuery{MembershipQuery: "scope", ReadStatus: "unread", StarStatus: "starred", ReplyStatus: "replied", Cutoff: time.Unix(2, 0)}, nil)
	if err != nil || !count.Complete || count.Total != 1 || count.Unread != 1 {
		t.Fatalf("thread-wide count: %+v %v", count, err)
	}
}
func TestCountFailureAndCancellationNeverClaimCompleteness(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) })
	count, err := c.CountThreads(context.Background(), CountQuery{}, nil)
	if err == nil || count.Complete {
		t.Fatalf("failure reported complete: %+v %v", count, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	count, err = c.CountThreads(ctx, CountQuery{}, nil)
	if err == nil || count.Complete {
		t.Fatal("cancelled count reported complete")
	}
}

func TestRepeatedCountCursorCannotLoopOrClaimCompletion(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"threads":[{"id":"one"}],"nextPageToken":"same"}`))
	})
	count, err := c.CountThreads(context.Background(), CountQuery{}, nil)
	if err == nil || count.Complete || !strings.Contains(err.Error(), "repeated") {
		t.Fatalf("invalid pagination claimed completeness: %+v %v", count, err)
	}
}

func TestCountSkipsThreadDeletedDuringHeaderEnumeration(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/threads" {
			_, _ = w.Write([]byte(`{"threads":[{"id":"deleted"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	count, err := c.CountThreads(context.Background(), CountQuery{ReplyStatus: "replied"}, nil)
	if err != nil || !count.Complete || count.Total != 0 {
		t.Fatalf("deleted thread count: %+v %v", count, err)
	}
}
