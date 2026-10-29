package gmail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	if err != nil || !count.Complete || count.Total != 3 || count.Unread != 1 || count.Scanned != 3 {
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
		if r.URL.Path == "/threads/b" {
			_, _ = w.Write([]byte(`{"messages":[{"id":"b","internalDate":"3000"}]}`))
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

func TestHeaderCountSkipsRedundantFlagEnumerations(t *testing.T) {
	countQueries := []string{}
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/threads" {
			countQueries = append(countQueries, r.URL.Query().Get("q"))
			_, _ = w.Write([]byte(`{"threads":[{"id":"one"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"messages":[{"id":"m","internalDate":"3000","labelIds":["UNREAD","STARRED","SENT"],"payload":{"headers":[{"name":"In-Reply-To","value":"<parent>"}]}}]}`))
	})
	count, err := c.CountThreads(context.Background(), CountQuery{MembershipQuery: "scope", ReadStatus: "unread", StarStatus: "starred", ReplyStatus: "replied"}, nil)
	if err != nil || !count.Complete || count.Total != 1 || count.Unread != 1 {
		t.Fatalf("count %+v %v", count, err)
	}
	if len(countQueries) != 1 || countQueries[0] != "scope" {
		t.Fatalf("metadata already supplies flags; unnecessary account scans: %v", countQueries)
	}
}

func TestCountCancelledAtLastPageCannotClaimCompleteness(t *testing.T) {
	for _, empty := range []bool{true, false} {
		t.Run(fmt.Sprint("empty=", empty), func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if empty {
					_, _ = w.Write([]byte(`{}`))
					return
				}
				_, _ = w.Write([]byte(`{"threads":[{"id":"one"}]}`))
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reports := 0
			count, err := c.CountThreads(ctx, CountQuery{}, func(progress ThreadCount) {
				reports++
				if empty || reports == 2 {
					cancel()
				}
			})
			if !errors.Is(err, context.Canceled) || count.Complete {
				t.Fatalf("cancelled final page claimed completeness: %+v %v", count, err)
			}
		})
	}
}
