package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	v1 "github.com/hkdb/aerion/internal/automation/v1"
)

// resetContactsPendingKeys isolates the process-wide pending map per test.
func resetContactsPendingKeys(t *testing.T) {
	t.Helper()
	contactsPendingKeys.Lock()
	contactsPendingKeys.byContent = make(map[string]string)
	contactsPendingKeys.Unlock()
	t.Cleanup(func() {
		contactsPendingKeys.Lock()
		contactsPendingKeys.byContent = make(map[string]string)
		contactsPendingKeys.Unlock()
	})
}

func TestContactsPendingContentKeyIsFixedSizeDigest(t *testing.T) {
	body := contactsCanonicalBody(struct {
		Name  string `json:"name"`
		Photo string `json:"photo"`
	}{Name: "Liisa", Photo: strings.Repeat("QUJD", 1<<16)})
	key := contactsPendingContentKey("mutate", body)
	if len(key) != 64 {
		t.Fatalf("digest length = %d, want 64 hex chars", len(key))
	}
	if strings.Contains(key, "QUJD") || strings.Contains(key, "Liisa") {
		t.Fatal("digest must not retain request content")
	}
	if got := contactsPendingContentKey("mutate", body); got != key {
		t.Fatal("digest must be stable for the same operation and body")
	}
	if got := contactsPendingContentKey("source", body); got == key {
		t.Fatal("digest must differ per operation")
	}
	other := contactsCanonicalBody(struct {
		Name  string `json:"name"`
		Photo string `json:"photo"`
	}{Name: "Liisa", Photo: strings.Repeat("REVG", 1<<16)})
	if got := contactsPendingContentKey("mutate", other); got == key {
		t.Fatal("digest must differ per request body")
	}
}

func TestContactsIdempotencyKeyReusesRetainedKeyOnRetry(t *testing.T) {
	resetContactsPendingKeys(t)
	body := contactsCanonicalBody(v1.ContactsMutationRequest{Operation: "create"})
	first, err := contactsIdempotencyKey("mutate", body)
	if err != nil {
		t.Fatalf("first key: %v", err)
	}
	// A transport failure keeps the key: settle with an error carrying no
	// fault code.
	settleContactsMutation(errors.New("dial tcp: i/o timeout"), "mutate", body)
	second, err := contactsIdempotencyKey("mutate", body)
	if err != nil {
		t.Fatalf("retry key: %v", err)
	}
	if second != first {
		t.Fatalf("retry after transport failure must replay the retained key: %q != %q", second, first)
	}
}

func TestContactsIdempotencyKeyFailsClosedAtCapacity(t *testing.T) {
	resetContactsPendingKeys(t)
	bodies := make([]string, 0, maxContactsPendingKeys)
	keys := make([]string, 0, maxContactsPendingKeys)
	for i := 0; i < maxContactsPendingKeys; i++ {
		body := contactsCanonicalBody(v1.ContactsMutationRequest{Operation: "create", ID: fmt.Sprintf("c-%d", i)})
		key, err := contactsIdempotencyKey("mutate", body)
		if err != nil {
			t.Fatalf("key %d of %d: %v", i+1, maxContactsPendingKeys, err)
		}
		// Leave every key pending (transport failure).
		settleContactsMutation(errors.New("dial tcp: i/o timeout"), "mutate", body)
		bodies = append(bodies, body)
		keys = append(keys, key)
	}
	// The map is full of unresolved mutations. A new, distinct request must
	// fail closed before sending: sending it with an unretained key would let
	// a later retry duplicate the create.
	extra := contactsCanonicalBody(v1.ContactsMutationRequest{Operation: "create", ID: "c-extra"})
	if _, err := contactsIdempotencyKey("mutate", extra); !errors.Is(err, errContactsPendingFull) {
		t.Fatalf("65th distinct pending mutation: err = %v, want errContactsPendingFull", err)
	}
	// Retrying one of the original 64 still replays its retained key.
	again, err := contactsIdempotencyKey("mutate", bodies[7])
	if err != nil {
		t.Fatalf("retry of retained request at capacity: %v", err)
	}
	if again != keys[7] {
		t.Fatalf("retained key changed at capacity: %q != %q", again, keys[7])
	}
	// Settling one request frees capacity for the next distinct mutation.
	settleContactsMutation(nil, "mutate", bodies[3])
	if _, err := contactsIdempotencyKey("mutate", extra); err != nil {
		t.Fatalf("new mutation after a settlement freed capacity: %v", err)
	}
}

func TestSettleContactsMutationDropsKeyOnDefinitiveOutcome(t *testing.T) {
	resetContactsPendingKeys(t)
	body := contactsCanonicalBody(v1.ContactsMutationRequest{Operation: "delete", ID: "c-1"})
	first, err := contactsIdempotencyKey("mutate", body)
	if err != nil {
		t.Fatalf("first key: %v", err)
	}
	// Success settles the mutation: a later identical request is a new action.
	settleContactsMutation(nil, "mutate", body)
	second, err := contactsIdempotencyKey("mutate", body)
	if err != nil {
		t.Fatalf("key after success: %v", err)
	}
	if second == first {
		t.Fatal("a settled mutation repeated later must get a fresh key")
	}
	// A definitive structured error (contact not found) also settles.
	third, err := contactsIdempotencyKey("mutate", body)
	if err != nil {
		t.Fatalf("third key: %v", err)
	}
	settleContactsMutation(fmt.Errorf("api error (%s): no such contact", v1.CodeNotFound), "mutate", body)
	fourth, err := contactsIdempotencyKey("mutate", body)
	if err != nil {
		t.Fatalf("fourth key: %v", err)
	}
	if fourth == third {
		t.Fatal("a definitive error must settle the mutation")
	}
	// An idempotency conflict keeps the key: the helper's receipt for this
	// in-flight operation is still the safe answer.
	settleContactsMutation(fmt.Errorf("api error (%s): conflicting request in flight", v1.CodeIdempotencyConflict), "mutate", body)
	fifth, err := contactsIdempotencyKey("mutate", body)
	if err != nil {
		t.Fatalf("fifth key: %v", err)
	}
	if fifth != fourth {
		t.Fatal("an idempotency conflict must keep the retained key")
	}
}
