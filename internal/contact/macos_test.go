package contact

import (
	"errors"
	"github.com/hkdb/aerion/internal/macoscontacts"
	"testing"
)

type fakeNativeContacts struct {
	status string
	people []macoscontacts.Person
	calls  int
	err    error
}

func (f *fakeNativeContacts) Status() string { return f.status }
func (f *fakeNativeContacts) Search(string, int) (macoscontacts.Result, error) {
	f.calls++
	return macoscontacts.Result{People: f.people}, f.err
}

func TestMacOSAutocompleteMerge(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db.DB)
	if err := store.AddOrUpdate("ada@example.com", "Saved Ada"); err != nil {
		t.Fatal(err)
	}
	native := &fakeNativeContacts{status: "authorized", people: []macoscontacts.Person{{ID: "ada", Name: "Native Ada", Emails: []string{"ADA@example.com", "ada@work.test", "bad address", ""}}}}
	store.nativeContacts = native
	got, err := store.Search("ada", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].DisplayName != "Saved Ada" || got[1].Email != "ada@work.test" || got[1].Source != "macos" {
		t.Fatalf("results %#v", got)
	}
	if native.calls != 1 {
		t.Fatalf("calls=%d", native.calls)
	}
	stored, err := store.Get("ada@work.test")
	if err != nil || stored != nil {
		t.Fatalf("native contact was copied into local storage: %+v %v", stored, err)
	}
	got, err = store.Search("ada", 1)
	if err != nil || len(got) != 1 {
		t.Fatalf("limit results=%d err=%v", len(got), err)
	}
}
func TestMacOSAutocompletePermissionAndFailure(t *testing.T) {
	for _, status := range []string{"denied", "not-determined", "restricted", "unavailable", "authorized", "limited"} {
		t.Run(status, func(t *testing.T) {
			db := openTestDB(t)
			store := NewStore(db.DB)
			if err := store.AddOrUpdate("ada@example.com", "Ada"); err != nil {
				t.Fatal(err)
			}
			native := &fakeNativeContacts{status: status, err: errors.New("native failure")}
			store.nativeContacts = native
			got, err := store.Search("ada", 10)
			if err != nil || len(got) != 1 {
				t.Fatalf("local lookup lost: %v %v", got, err)
			}
			expected := 0
			if status == "authorized" || status == "limited" {
				expected = 1
			}
			if native.calls != expected {
				t.Fatalf("calls=%d want=%d", native.calls, expected)
			}
		})
	}
}
