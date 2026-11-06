package mailmove

import (
	"errors"
	"reflect"
	"testing"
)

func TestCopyBeforeCleanup(t *testing.T) {
	t.Run("copy is confirmed before cleanup", func(t *testing.T) {
		var calls []string
		err := CopyBeforeCleanup(
			func() error { calls = append(calls, "copy"); return nil },
			func() error { calls = append(calls, "cleanup"); return nil },
		)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"copy", "cleanup"}; !reflect.DeepEqual(calls, want) {
			t.Fatalf("calls = %v, want %v", calls, want)
		}
	})

	t.Run("failed or ambiguous copy preserves source", func(t *testing.T) {
		copyErr := errors.New("append response lost")
		cleaned := false
		err := CopyBeforeCleanup(func() error { return copyErr }, func() error { cleaned = true; return nil })
		if !errors.Is(err, copyErr) {
			t.Fatalf("error = %v, want wrapped %v", err, copyErr)
		}
		if cleaned {
			t.Fatal("source cleanup ran before destination confirmation")
		}
	})
}
