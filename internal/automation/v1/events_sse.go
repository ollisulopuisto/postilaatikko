package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Events stream timings. The heartbeat keeps proxies and the desktop client
// from treating an idle stream as dead; the revocation interval bounds how
// long a revoked client keeps receiving events.
// Var (not const) so tests can shorten them.
var (
	eventsHeartbeatInterval  = 25 * time.Second
	eventsRevocationInterval = 30 * time.Second
)

// ErrEventSourceSaturated classifies a MailboxEventSource.Subscribe failure
// as "too many subscribers": the transport answers 503 retryable so the
// client backs off instead of erroring out.
var ErrEventSourceSaturated = errors.New("v1: event subscriber limit reached")

// MountEventsRoute mounts GET /v1/events, the authenticated server-sent
// events stream of mailbox change notifications (PERF-SC-004).
//
//   - checker may be nil; when set together with grantID, the stream
//     revalidates the grant periodically and ends once the grant is revoked
//     (the client's reconnect then fails authentication).
//   - grantID extracts the authenticated client ID from the request context
//     the auth middleware populated; nil disables revalidation.
func MountEventsRoute(mux RouteMux, source MailboxEventSource, checker GrantRevocationChecker, grantID func(*http.Request) string, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("GET /v1/events", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailRead) {
			return
		}
		controller := http.NewResponseController(w)
		// Renew a bounded deadline before each write. Flush alone does not renew
		// http.Server.WriteTimeout; Unwrap-aware control also preserves timing middleware.
		renewDeadline := func() error {
			err := controller.SetWriteDeadline(time.Now().Add(30 * time.Second))
			if errors.Is(err, http.ErrNotSupported) {
				return nil
			}
			return err
		}
		var afterSeq uint64
		if lastID := r.Header.Get("Last-Event-ID"); lastID != "" {
			parsed, err := strconv.ParseUint(lastID, 10, 64)
			if err != nil {
				writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Malformed Last-Event-ID", false)
				return
			}
			afterSeq = parsed
		}
		replay, ch, resync, cancel, err := source.Subscribe(afterSeq)
		if err != nil {
			if errors.Is(err, ErrEventSourceSaturated) {
				writeHeaderError(w, r, http.StatusServiceUnavailable, CodeRateLimited, "Too many event subscribers; retry later", true)
				return
			}
			writeHeaderError(w, r, http.StatusServiceUnavailable, CodeInternal, "Event stream unavailable", true)
			return
		}
		defer cancel()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		if err := renewDeadline(); err != nil {
			return
		}
		w.WriteHeader(http.StatusOK)
		// Flush the headers at once: a subscriber with no replay and no live
		// traffic would otherwise see the connect hang until the first event.
		if err := controller.Flush(); err != nil {
			return
		}

		writeEvent := func(ev MailboxEvent) error {
			if err := renewDeadline(); err != nil {
				return err
			}
			data, err := json.Marshal(ev)
			if err != nil {
				return err
			}
			if ev.Sequence > 0 {
				if _, err := fmt.Fprintf(w, "id: %d\n", ev.Sequence); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data); err != nil {
				return err
			}
			return controller.Flush()
		}

		if resync {
			if err := writeEvent(MailboxEvent{Type: EventResync, At: time.Now().UTC()}); err != nil {
				return
			}
		}
		for _, ev := range replay {
			if err := writeEvent(ev); err != nil {
				return
			}
		}

		heartbeat := time.NewTicker(eventsHeartbeatInterval)
		defer heartbeat.Stop()
		var revalidate <-chan time.Time
		var clientID string
		if checker != nil && grantID != nil {
			if grantID(r) == "" {
				return
			}
			if id := grantID(r); id != "" {
				clientID = id
				t := time.NewTicker(eventsRevocationInterval)
				defer t.Stop()
				revalidate = t.C
			}
		}

		for {
			select {
			case <-r.Context().Done():
				return
			case ev, open := <-ch:
				if !open {
					// Broker closed: the helper is shutting down; end the
					// stream promptly so the client reconnects to the next
					// helper process.
					return
				}
				if err := writeEvent(ev); err != nil {
					return
				}
			case <-heartbeat.C:
				if err := renewDeadline(); err != nil {
					return
				}
				if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
					return
				}
				if err := controller.Flush(); err != nil {
					return
				}
			case <-revalidate:
				gone, err := checker.GrantRevoked(clientID)
				if err != nil || gone {
					return
				}
			}
		}
	})
}
