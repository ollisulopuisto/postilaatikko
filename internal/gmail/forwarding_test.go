package gmail

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestForwardingCatalogReadOnlyAcceptedExactSelection(t *testing.T) {
	calls := 0
	c := filterTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/settings/forwardingAddresses" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"forwardingAddresses": []ForwardingAddress{{Email: "accepted@example.test", VerificationStatus: "accepted"}, {Email: "pending@example.test", VerificationStatus: "pending"}}})
	})
	list, err := c.ListForwardingAddresses(context.Background())
	if err != nil || len(list) != 2 || calls != 1 {
		t.Fatal(list, err, calls)
	}
	for _, address := range []string{"pending@example.test", "other@example.test", "Accepted@example.test", "Name <accepted@example.test>", "accepted@example.test\r\nBcc:x@example.test"} {
		if err := ValidateForwardingDestination(address, list); err == nil {
			t.Fatal("accepted invalid destination", address)
		}
	}
	if err := ValidateForwardingDestination("accepted@example.test", list); err != nil {
		t.Fatal(err)
	}
}
