package oauth2

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFilterGrantRefreshHonorsCallerDeadline(t *testing.T) {
	reached := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
			return
		}
		close(reached)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := NewManager().VerifyGoogleFilterRefresh(ctx, ProviderConfig{TokenURL: server.URL}, "synthetic-refresh", []string{"mail"})
	select {
	case <-reached:
	case <-time.After(time.Second):
		t.Fatalf("refresh never reached test server: %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("refresh ignored request deadline", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("refresh exceeded caller budget")
	}
}
