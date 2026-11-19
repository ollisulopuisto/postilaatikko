package message

import (
	"os"
	"testing"
)

func TestKeysetPhasePlans(t *testing.T) {
	if os.Getenv("PERF_PROFILE") != "1" {
		t.Skip("set PERF_PROFILE=1")
	}
	store := seedProfileStore(t, 2000)
	q1 := `SELECT COALESCE(m.thread_id, m.id), m.date FROM messages m WHERE m.folder_id = ? AND m.date <= ?` + conversationVisibilityClauses(false) + ` ORDER BY m.date DESC, COALESCE(m.thread_id, m.id) ASC`
	t.Log("phase 1 plan:")
	logPlan(t, store, q1, "acc-1-inbox", "2026-01-01 00:00:00")
	q2 := buildConversationAggregateQuery(1, false) + ` AND m.account_id = ? AND COALESCE(m.thread_id, m.id) IN (?) GROUP BY COALESCE(m.thread_id, m.id), a.id`
	t.Log("phase 2 plan:")
	logPlan(t, store, q2, "acc-1-inbox", "acc-1", "thread-1")
}

func logPlan(t *testing.T, store *Store, q string, args ...interface{}) {
	t.Helper()
	rows, err := store.db.Query("EXPLAIN QUERY PLAN "+q, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, parent, nu int
		var detail string
		if err := rows.Scan(&id, &parent, &nu, &detail); err != nil {
			t.Fatal(err)
		}
		t.Logf("  %s", detail)
	}
}
