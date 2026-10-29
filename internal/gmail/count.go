package gmail

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

type CountQuery struct {
	MembershipQuery string
	ReadStatus      string
	StarStatus      string
	ReplyStatus     string
	Cutoff          time.Time
}
type ThreadCount struct {
	Total    int
	Unread   int
	Scanned  int
	Complete bool
}

// CountThreads fully enumerates server thread IDs. Provider estimates are never
// treated as completed counts. Separate thread sets let flags on different
// messages satisfy the same conversation, unlike one combined message query.
// Normal counts read IDs only; reply/date predicates use headers, never bodies.
func (c *Client) CountThreads(ctx context.Context, q CountQuery, progress func(ThreadCount)) (result ThreadCount, resultErr error) {
	if !slices.Contains([]string{"", "all", "read", "unread"}, q.ReadStatus) || !slices.Contains([]string{"", "all", "starred", "unstarred"}, q.StarStatus) || !slices.Contains([]string{"", "all", "replied", "unreplied"}, q.ReplyStatus) {
		return result, fmt.Errorf("invalid thread count criteria")
	}
	members := map[string]uint8{}
	report := func() {
		if progress != nil {
			progress(result)
		}
	}
	enumerate := func(query string, visit func(string)) error {
		token := ""
		seenTokens := map[string]bool{}
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			page, err := c.Threads(ctx, SearchRequest{Query: query, PageToken: token, IncludeSpamTrash: true})
			if err != nil {
				return err
			}
			for _, id := range page.ThreadIDs {
				visit(id)
			}
			report()
			if err := ctx.Err(); err != nil {
				return err
			}
			if page.NextPageToken == "" {
				return nil
			}
			if seenTokens[page.NextPageToken] {
				return fmt.Errorf("gmail: repeated thread-page token; count incomplete")
			}
			seenTokens[page.NextPageToken] = true
			token = page.NextPageToken
		}
	}
	if err := enumerate(q.MembershipQuery, func(id string) {
		if _, exists := members[id]; exists {
			return
		}
		members[id] = 0
		result.Scanned++
	}); err != nil {
		return result, err
	}
	if len(members) == 0 {
		result.Complete = true
		report()
		return result, nil
	}
	acceptsFlags := func(flags uint8) bool {
		unread, starred := flags&1 != 0, flags&2 != 0
		return !(q.ReadStatus == "unread" && !unread || q.ReadStatus == "read" && unread || q.StarStatus == "starred" && !starred || q.StarStatus == "unstarred" && starred)
	}
	if q.Cutoff.IsZero() && (q.ReplyStatus == "" || q.ReplyStatus == "all") {
		if err := enumerate("is:unread", func(id string) {
			if flags, ok := members[id]; ok {
				members[id] = flags | 1
			}
		}); err != nil {
			return result, err
		}
		if q.StarStatus == "starred" || q.StarStatus == "unstarred" {
			if err := enumerate("is:starred", func(id string) {
				if flags, ok := members[id]; ok {
					members[id] = flags | 2
				}
			}); err != nil {
				return result, err
			}
		}
		for _, flags := range members {
			if acceptsFlags(flags) {
				result.Total++
				if flags&1 != 0 {
					result.Unread++
				}
			}
		}
		result.Complete = true
		report()
		return result, nil
	}
	// Header work is bounded even when a view has many candidates. Accumulate
	// only counts; no body/attachment payload or local message cache is created.
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan string)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				items, err := c.ThreadMetadata(workCtx, id, "In-Reply-To")
				mu.Lock()
				if errors.Is(err, ErrNotFound) || err == nil && len(items) == 0 {
					mu.Unlock()
					continue
				}
				if err != nil {
					if firstErr == nil {
						firstErr = err
						cancel()
					}
					mu.Unlock()
					continue
				}
				unread, starred, replied := false, false, false
				var latest time.Time
				for _, item := range items {
					unread = unread || slices.Contains(item.LabelIDs, "UNREAD")
					starred = starred || slices.Contains(item.LabelIDs, "STARRED")
					replied = replied || (slices.Contains(item.LabelIDs, "SENT") && item.InReplyTo != "")
					if item.Date.After(latest) {
						latest = item.Date
					}
				}
				flags := uint8(0)
				if unread {
					flags |= 1
				}
				if starred {
					flags |= 2
				}
				matches := acceptsFlags(flags) && (q.Cutoff.IsZero() || !latest.Before(q.Cutoff)) && !(q.ReplyStatus == "replied" && !replied || q.ReplyStatus == "unreplied" && replied)
				if matches {
					result.Total++
					if unread {
						result.Unread++
					}
				}
				report()
				mu.Unlock()
			}
		}()
	}
send:
	for id := range members {
		select {
		case jobs <- id:
		case <-workCtx.Done():
			break send
		}
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return result, firstErr
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	result.Complete = true
	report()
	return result, nil
}
