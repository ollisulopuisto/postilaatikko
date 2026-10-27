package gmail

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

type ThreadPage struct {
	ThreadIDs     []string
	NextPageToken string
	EstimateTotal int
}

func (c *Client) Threads(ctx context.Context, r SearchRequest) (*ThreadPage, error) {
	limit := r.Limit
	if limit <= 0 || limit > maxSearchResults {
		limit = maxSearchResults
	}
	q := url.Values{"maxResults": {strconv.Itoa(limit)}, "fields": {"threads/id,nextPageToken,resultSizeEstimate"}}
	if r.Query != "" {
		q.Set("q", r.Query)
	}
	if r.PageToken != "" {
		q.Set("pageToken", r.PageToken)
	}
	if r.IncludeSpamTrash {
		q.Set("includeSpamTrash", "true")
	}
	for _, label := range r.LabelIDs {
		q.Add("labelIds", label)
	}
	var data struct {
		Threads []struct {
			ID string `json:"id"`
		} `json:"threads"`
		Next     string `json:"nextPageToken"`
		Estimate int    `json:"resultSizeEstimate"`
	}
	if err := c.do(ctx, http.MethodGet, "/threads", q, nil, &data); err != nil {
		return nil, err
	}
	page := &ThreadPage{NextPageToken: data.Next, EstimateTotal: data.Estimate}
	for _, thread := range data.Threads {
		if thread.ID == "" {
			return nil, fmt.Errorf("gmail: thread without an identifier")
		}
		page.ThreadIDs = append(page.ThreadIDs, thread.ID)
	}
	return page, nil
}
