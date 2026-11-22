package alwayson

import (
	"net/http"
	"strings"
	"testing"

	"github.com/ollisulopuisto/postilaatikko/internal/account"
	"github.com/ollisulopuisto/postilaatikko/internal/automation"
	v1 "github.com/ollisulopuisto/postilaatikko/internal/automation/v1"
	"github.com/ollisulopuisto/postilaatikko/internal/folder"
	"github.com/ollisulopuisto/postilaatikko/internal/virtualfolder"
)

// recordingMux collects every pattern mounted on it. The v1 mount functions
// accept v1.RouteMux precisely so this test can enumerate the whole route
// surface without standing up a runtime.
type recordingMux struct {
	patterns []string
}

func (m *recordingMux) HandleFunc(pattern string, _ func(http.ResponseWriter, *http.Request)) {
	m.patterns = append(m.patterns, pattern)
}

// stubCatalog implements CatalogOperations plus IdentityCatalogOperations so
// the conditionally mounted identities route appears in the enumeration,
// matching production where catalogReadOperations implements both.
type stubCatalog struct{}

func (stubCatalog) GetAccounts() ([]*account.Account, error)                   { return nil, nil }
func (stubCatalog) GetFolders(string) ([]*folder.Folder, error)                { return nil, nil }
func (stubCatalog) GetVirtualFolders() ([]*virtualfolder.VirtualFolder, error) { return nil, nil }
func (stubCatalog) GetIdentities(string) ([]*account.Identity, error)          { return nil, nil }

// stubContacts and stubCalendar satisfy the conditional mount dependencies
// for enumeration without implementing the full interfaces: the embedded
// nil interface promotes every method, and enumeration never invokes them.
// They stand in for production's non-nil contacts/calendar runtimes so the
// conditionally mounted routes appear in the enumeration, matching
// production where the runtimes exist.
type stubContacts struct{ v1.ContactsOperations }
type stubCalendar struct{ v1.CalendarReadOperations }
type stubEvents struct{ v1.MailboxEventSource }

// mountAllV1Routes mounts the same route surface as
// SyncRuntime.MailboxReadAPIHandler through the same mountMailboxV1Routes
// function, so the enumeration cannot drift from production (a Devin
// finding on PERF-SC-002: a hand-mirrored list stayed green when production
// added a mount call). The point of this test is that a newly mounted route
// fails classification until a human decides whether the read coalescer may
// merge it. Attachment reads remain non-coalescible because a byte read may
// contact the provider; both routes still require attachment-read capability.
func mountAllV1Routes(mux *recordingMux) {
	mountMailboxV1Routes(mux, &mailboxRouteDeps{
		catalog:       stubCatalog{},
		grantClientID: func(*http.Request) string { return "client" },
		contacts:      stubContacts{},
		calendar:      stubCalendar{},
		events:        stubEvents{},
	})
}

// routeClassification is the explicit coalescing decision for one mounted
// pattern. For patterns without a method prefix (the handler switches on the
// method internally), checks lists the methods the handler accepts.
type routeClassification struct {
	pattern string
	checks  []routeCheck
}

type routeCheck struct {
	method string
	path   string // sample path with wildcards filled in
	want   bool   // coalescible
}

func get(pattern string, want bool) routeClassification {
	return routeClassification{pattern: pattern, checks: []routeCheck{{method: http.MethodGet, path: samplePath(pattern), want: want}}}
}

func post(pattern string, want bool) routeClassification {
	return routeClassification{pattern: pattern, checks: []routeCheck{{method: http.MethodPost, path: samplePath(pattern), want: want}}}
}

func samplePath(pattern string) string {
	path := pattern
	if i := strings.IndexByte(pattern, ' '); i >= 0 {
		path = strings.TrimSpace(pattern[i+1:])
	}
	path = strings.ReplaceAll(path, "{accountID}", "acc1")
	path = strings.ReplaceAll(path, "{virtualFolderID}", "vf1")
	path = strings.ReplaceAll(path, "{draftID}", "d1")
	path = strings.ReplaceAll(path, "{messageID}", "m1")
	path = strings.ReplaceAll(path, "{idempotencyKey}", "k1")
	path = strings.ReplaceAll(path, "{id}", "e1")
	return path
}

// classifiedRoutes lists every /v1 pattern MailboxReadAPIHandler mounts and
// whether the PERF-SC-002 read coalescer may merge it. True only for
// read-only operations whose response depends solely on the request and the
// caller's authorization.
var classifiedRoutes = []routeClassification{
	// Catalog reads (read.go MountCatalogRoutes).
	get("/v1/accounts", true),
	{pattern: "/v1/accounts/", checks: []routeCheck{{http.MethodGet, "/v1/accounts/acc1", false}}},
	get("GET /v1/accounts/{accountID}/identities", false),
	get("/v1/folders", true),
	get("/v1/views", true),
	// Saved-view cache and unread counts.
	get("GET /v1/views/{virtualFolderID}/cache", true),
	get("GET /v1/views/unread-counts", true),
	// Account mutations.
	post("POST /v1/accounts/{accountID}/oauth", false),
	post("POST /v1/accounts", false),
	{pattern: "PUT /v1/accounts/{accountID}", checks: []routeCheck{{http.MethodPut, "/v1/accounts/acc1", false}}},
	{pattern: "DELETE /v1/accounts/{accountID}", checks: []routeCheck{{http.MethodDelete, "/v1/accounts/acc1", false}}},
	{pattern: "PATCH /v1/accounts/{accountID}/enabled", checks: []routeCheck{{http.MethodPatch, "/v1/accounts/acc1/enabled", false}}},
	{pattern: "PUT /v1/accounts/order", checks: []routeCheck{{http.MethodPut, "/v1/accounts/order", false}}},
	post("POST /v1/accounts/shared-mailboxes", false),
	// Folder mutations and visibility.
	post("/v1/folders/sync", false),
	post("/v1/folders/subscription", false),
	post("/v1/folders/subscriptions", false),
	post("/v1/folders/visibility", false),
	// Saved-view mutations.
	post("/v1/views/mutations", false),
	// Filter rules and jobs.
	post("/v1/filters/evaluate", false),
	post("/v1/filters/gmail", false),
	post("/v1/filters/gmail/consent", false),
	get("/v1/filters", false),
	post("/v1/filters/mutations", false),
	get("/v1/filter-jobs", false),
	// Drafts and send.
	get("GET /v1/drafts", false),
	get("GET /v1/drafts/{draftID}", false),
	post("POST /v1/drafts", false),
	{pattern: "PUT /v1/drafts/{draftID}", checks: []routeCheck{{http.MethodPut, "/v1/drafts/d1", false}}},
	{pattern: "DELETE /v1/drafts/{draftID}", checks: []routeCheck{{http.MethodDelete, "/v1/drafts/d1", false}}},
	post("POST /v1/replies", false),
	post("POST /v1/send", false),
	get("GET /v1/send/receipts/{idempotencyKey}", false),
	// Reply-later and triage.
	post("/v1/reply-later", false),
	post("/v1/triage/read-later", false),
	post("/v1/triage/snoozes", false),
	get("/v1/triage/snoozes/count", false),
	post("/v1/reply-later/mutations", false),
	post("/v1/triage/read-later/mutations", false),
	post("/v1/triage/later/mutations", false),
	post("/v1/triage/snoozes/mutations", false),
	// Message headers and conversation reads.
	post("/v1/messages/headers", false),
	post("/v1/conversations", true),
	post("POST /v1/conversations/count", true),
	post("POST /v1/conversations/search", true),
	post("POST /v1/workspace/bootstrap", true),
	// The event stream is a long-lived SSE response: coalescing it would
	// buffer the stream and merge distinct subscribers, so it never
	// coalesces.
	get("GET /v1/events", false),
	post("/v1/search", true),
	// Thread and body reads: thread rendering is read-only (see the comment
	// on coalescibleRead); body fetches can hit providers and cache bodies,
	// so they stay uncoalesced.
	post("/v1/messages/attachments/list", false),
	post("/v1/messages/attachments/read", false),
	post("/v1/threads/read", true),
	get("GET /v1/messages/{messageID}/header", false),
	post("/v1/messages/body", false),
	// Actions, undo, and action events.
	post("/v1/actions/resolve", false),
	post("/v1/actions", false),
	get("/v1/undo/history", false),
	post("/v1/undo/latest", false),
	{pattern: "/v1/undo/operations/", checks: []routeCheck{{http.MethodPost, "/v1/undo/operations/op1", false}}},
	get("/v1/action-events", false),
	post("/v1/action-events/ack", false),
	// Contacts surface.
	post("POST /v1/contacts/search", false),
	post("POST /v1/contacts/legacy/list", false),
	post("POST /v1/contacts/legacy/get", false),
	post("POST /v1/contacts/photos", false),
	post("POST /v1/contacts/account-photos", false),
	post("POST /v1/contacts/browse", false),
	post("POST /v1/contacts/detail", false),
	get("GET /v1/contacts/sources", false),
	get("GET /v1/contacts/stats", false),
	post("POST /v1/contacts/addressbooks", false),
	post("POST /v1/contacts/source-addressbooks", false),
	post("POST /v1/contacts/linked-accounts", false),
	post("POST /v1/contacts/mutations", false),
	post("POST /v1/contacts/sources/discover", false),
	post("POST /v1/contacts/sources/test", false),
	post("POST /v1/contacts/sources/discover-oauth", false),
	post("POST /v1/contacts/sources/mutations", false),
	post("POST /v1/contacts/sources/authorization", false),
	post("POST /v1/contacts/sources/tokens", false),
	// Calendar surface.
	get("GET /v1/calendar/sources", false),
	get("GET /v1/calendar/calendars", false),
	get("GET /v1/calendar/events/{id}", false),
	post("POST /v1/calendar/events/query", false),
}

// TestEveryMountedRouteIsClassified fails when a route is mounted without an
// explicit coalescing decision, and when the coalescer's behavior disagrees
// with the decision. This guards the singleflight allowlist against drift
// toward a future mutation or streaming route.
func TestEveryMountedRouteIsClassified(t *testing.T) {
	mux := &recordingMux{}
	mountAllV1Routes(mux)

	classified := make(map[string]routeClassification, len(classifiedRoutes))
	for _, entry := range classifiedRoutes {
		if _, dup := classified[entry.pattern]; dup {
			t.Fatalf("pattern %s classified twice", entry.pattern)
		}
		classified[entry.pattern] = entry
	}

	for _, pattern := range mux.patterns {
		entry, ok := classified[pattern]
		if !ok {
			t.Errorf("mounted route %q has no coalescing classification; add it to classifiedRoutes", pattern)
			continue
		}
		delete(classified, pattern)
		for _, check := range entry.checks {
			if got := automation.CoalescibleRead(check.method, check.path); got != check.want {
				t.Errorf("CoalescibleRead(%s, %s) = %v, want %v (pattern %s)", check.method, check.path, got, check.want, pattern)
			}
		}
	}
	for pattern := range classified {
		t.Errorf("classified route %q is not mounted; remove it from classifiedRoutes or mount it", pattern)
	}
}
