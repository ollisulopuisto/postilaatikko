package macoscontacts

import (
	"errors"
	"testing"
)

type fakeBackend struct {
	status                    string
	requests, searches, opens int
	err                       error
	limit                     int
}

func (f *fakeBackend) Status() string { return f.status }
func (f *fakeBackend) Request() error { f.requests++; f.status = "authorized"; return f.err }
func (f *fakeBackend) Search(query string, limit int) (Result, error) {
	f.searches++
	f.limit = limit
	return Result{People: []Person{{ID: "person", Name: "Ada", Emails: []string{"ada@example.com"}}}}, f.err
}
func (f *fakeBackend) Open(id string) error { f.opens++; return f.err }

func TestSearchNeverRequestsAccess(t *testing.T) {
	for _, status := range []string{"not-determined", "denied", "restricted", "unavailable", "authorized", "limited"} {
		t.Run(status, func(t *testing.T) {
			backend := &fakeBackend{status: status}
			source := Source{backend: backend}
			_, err := source.Search("ada", 5000)
			allowed := status == "authorized" || status == "limited"
			if (err == nil) != allowed || backend.requests != 0 {
				t.Fatalf("err=%v requests=%d", err, backend.requests)
			}
			if backend.searches != boolInt(allowed) {
				t.Fatalf("searches=%d", backend.searches)
			}
			if allowed && backend.limit != 100 {
				t.Fatalf("unbounded limit %d", backend.limit)
			}
		})
	}
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func TestExplicitRequest(t *testing.T) {
	backend := &fakeBackend{status: "not-determined"}
	source := Source{backend: backend}
	if _, err := source.RequestAccess(); err != nil || backend.requests != 1 {
		t.Fatalf("%v requests=%d", err, backend.requests)
	}
	if _, err := source.RequestAccess(); err != nil || backend.requests != 1 {
		t.Fatalf("authorized request repeated: %v", err)
	}
	backend.status = "denied"
	if _, err := source.RequestAccess(); err == nil || backend.requests != 1 {
		t.Fatalf("denied request: %v", err)
	}
}
func TestOpenRequiresAccessAndIdentifier(t *testing.T) {
	for _, test := range []struct {
		status, id string
		want       bool
	}{{"authorized", "person", true}, {"authorized", "", false}, {"authorized", "bad\x00id", false}, {"denied", "person", false}} {
		backend := &fakeBackend{status: test.status}
		source := Source{backend: backend}
		err := source.Open(test.id)
		if (err == nil) != test.want || backend.opens != boolInt(test.want) {
			t.Fatalf("%+v err=%v", test, err)
		}
	}
}
func TestReadFailurePropagates(t *testing.T) {
	backend := &fakeBackend{status: "authorized", err: errors.New("revoked")}
	source := Source{backend: backend}
	if _, err := source.Search("ada", 0); !errors.Is(err, backend.err) {
		t.Fatalf("%v", err)
	}
	if backend.limit != 50 {
		t.Fatalf("default limit %d", backend.limit)
	}
}
