package v1

import (
	"net/http"
	"strconv"

	"github.com/hkdb/aerion/internal/mailfilter"
)

// FilterJobPage is one bounded page of uncertain automatic filter moves.
type FilterJobPage struct {
	Items      []mailfilter.FilterJobInspection `json:"items"`
	NextCursor string                           `json:"nextCursor,omitempty"`
	Complete   bool                             `json:"complete"`
}

// FilterJobInspectionOperations reads uncertain jobs without changing them.
type FilterJobInspectionOperations interface {
	ListUncertainFilterJobs(offset, limit int) ([]mailfilter.FilterJobInspection, error)
}

// MountFilterJobRoutes mounts read-only inspection of uncertain filter moves.
func MountFilterJobRoutes(mux *http.ServeMux, ops FilterJobInspectionOperations, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("/v1/filter-jobs", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailRead) {
			return
		}
		if r.Method != http.MethodGet {
			writeHeaderError(w, r, http.StatusMethodNotAllowed, CodeInvalidRequest, "Method not allowed", false)
			return
		}
		state := r.URL.Query().Get("state")
		if state != "" && state != "outcome_unknown" {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Only outcome_unknown filter jobs are available for inspection", false)
			return
		}

		pageRequest := PageRequest{Limit: DefaultPageSize}
		if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
			limit, err := strconv.Atoi(rawLimit)
			if err != nil {
				writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "limit must be an integer", false)
				return
			}
			pageRequest.Limit = limit
		}
		pageRequest, err := pageRequest.Normalized()
		if err != nil {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, err.Error(), false)
			return
		}
		offset := 0
		if cursor := r.URL.Query().Get("cursor"); cursor != "" {
			offset, err = strconv.Atoi(cursor)
			if err != nil || offset < 0 || offset > MaximumMessageOffset {
				writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "cursor is invalid or exceeds the maximum offset", false)
				return
			}
		}

		items, err := ops.ListUncertainFilterJobs(offset, pageRequest.Limit+1)
		if err != nil {
			writeHeaderError(w, r, http.StatusInternalServerError, CodeInternal, "Failed to list uncertain filter jobs", true)
			return
		}
		page := FilterJobPage{Items: items, Complete: true}
		if len(items) > pageRequest.Limit {
			page.Items = items[:pageRequest.Limit]
			page.Complete = false
			page.NextCursor = strconv.Itoa(offset + pageRequest.Limit)
		}
		if page.Items == nil {
			page.Items = []mailfilter.FilterJobInspection{}
		}
		writeAPIResponse(w, r, http.StatusOK, page)
	})
}
