package readerbridge

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestDispatch(t *testing.T) {
	calls := 0
	methods := map[string]any{"Read": func(id string) (string, error) {
		calls++
		if id == "bad" {
			return "", errors.New("missing")
		}
		return "body:" + id, nil
	}}
	for _, tc := range []struct {
		method, args string
		ok           bool
	}{{"Read", `["id"]`, true}, {"Read", `[3]`, false}, {"Read", `[]`, false}, {"Read", `[null]`, false}, {"DeleteEverything", `[]`, false}, {"Read", `["bad"]`, false}} {
		result, err := Dispatch(methods, tc.method, json.RawMessage(tc.args))
		if (err == nil) != tc.ok {
			t.Fatalf("%+v: %v", tc, err)
		}
		if tc.ok && result != "body:id" {
			t.Fatal(result)
		}
	}
	if calls != 2 {
		t.Fatalf("invalid requests invoked methods: %d", calls)
	}
}

func TestDispatchGetterAndErrorOnlyMethods(t *testing.T) {
	methods := map[string]any{"Flag": func() bool { return true }, "Save": func(value string) error {
		if value != "ok" {
			return errors.New("failed")
		}
		return nil
	}}
	result, err := Dispatch(methods, "Flag", json.RawMessage(`[]`))
	if err != nil || result != true {
		t.Fatalf("getter: %v, %v", result, err)
	}
	result, err = Dispatch(methods, "Save", json.RawMessage(`["ok"]`))
	if err != nil || result != nil {
		t.Fatalf("error-only: %v, %v", result, err)
	}
	if _, err = Dispatch(methods, "Save", json.RawMessage(`["bad"]`)); err == nil {
		t.Fatal("error discarded")
	}
	if _, err = Dispatch(methods, "Flag", json.RawMessage(`{}`)); err == nil {
		t.Fatal("non-array args accepted")
	}
}
