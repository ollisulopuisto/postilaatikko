package automation_test

import (
	"context"
	"github.com/hkdb/aerion/internal/automation"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/mailboxevents"
	"net/http"
	"testing"
	"time"
)

func TestEventsThroughProductionTimingMiddleware(t *testing.T) {
	broker := mailboxevents.NewBroker(mailboxevents.Options{})
	defer broker.Close()
	mux := http.NewServeMux()
	v1.MountEventsRoute(mux, broker, nil, nil, func(http.ResponseWriter, *http.Request, v1.Capability) bool { return true })
	store := automation.NewMemoryTokenStore()
	grant, err := store.RegisterClient("events-test", []v1.Capability{v1.CapabilityMailRead})
	if err != nil {
		t.Fatal(err)
	}
	server := automation.NewServer(automation.ServerConfig{TokenStore: store, V1Handler: mux, RequestTiming: true})
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Stop(ctx)
	}()
	req, _ := http.NewRequest("GET", server.URL()+"/v1/events", nil)
	req.Header.Set("Authorization", "Bearer "+grant.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}
