package credentials

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestOAuthCompareAndSwapRefusesStaleGrant(t *testing.T) {
	store, db := newTestStore(t)
	insertTestAccount(t, db, "cas")
	seed := &OAuthTokens{Provider: "google", AccessToken: "old", RefreshToken: "refresh", Scopes: []string{"mail"}, ExpiresAt: time.Now().Add(time.Hour)}
	if err := store.SetOAuthTokens("cas", seed); err != nil {
		t.Fatal(err)
	}
	other := *store // production creates independent Store objects over the same database
	expected, err := other.GetOAuthTokens("cas")
	if err != nil {
		t.Fatal(err)
	}
	replacement := *expected
	replacement.AccessToken = "consented"
	replacement.Scopes = []string{"mail", "settings"}
	if err = store.CompareAndSwapOAuthTokens("cas", expected, &replacement); err != nil {
		t.Fatal(err)
	}
	stale := *expected
	stale.AccessToken = "stale-refresh"
	if err = other.CompareAndSwapOAuthTokens("cas", expected, &stale); !errors.Is(err, ErrOAuthTokensChanged) {
		t.Fatal("stale refresh accepted", err)
	}
	got, err := store.GetOAuthTokens("cas")
	if err != nil || got.AccessToken != "consented" || len(got.Scopes) != 2 {
		t.Fatal("newer grant lost", err)
	}
}
func TestOAuthCompareAndSwapConcurrentWritersHaveOneWinner(t *testing.T) {
	store, db := newTestStore(t)
	insertTestAccount(t, db, "race")
	if err := store.SetOAuthTokens("race", &OAuthTokens{Provider: "google", AccessToken: "old", RefreshToken: "refresh", Scopes: []string{"mail"}, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	expected, err := store.GetOAuthTokens("race")
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, access := range []string{"refresh", "consent"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			next := *expected
			next.AccessToken = access
			results <- store.CompareAndSwapOAuthTokens("race", expected, &next)
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	successes, stales := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrOAuthTokensChanged) {
			stales++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || stales != 1 {
		t.Fatal("lost snapshot serialization", successes, stales)
	}
}
