package reactions

import "testing"

func TestStructuredReaction(t *testing.T) {
	for _, tc := range []struct {
		data, target string
		valid        bool
	}{
		{`{"version":1,"emoji":"👍"}`, "<target@example.com>", true},
		{`{"version":1,"emoji":"👍🏽"}`, "<target@example.com>", true},
		{`{"version":2,"emoji":"👍"}`, "<target@example.com>", false},
		{`{"version":"1","emoji":"👍"}`, "<target@example.com>", false},
		{`{"version":1,"emoji":"hello"}`, "<target@example.com>", false},
		{`{"version":1,"emoji":"👍👍"}`, "<target@example.com>", false},
		{`{"version":1,"emoji":"👍"}`, "<a@example.com> <b@example.com>", false},
	} {
		info := ParsePart(GoogleMIME, "", tc.target, []byte(tc.data))
		if info == nil || (!info.Invalid) != tc.valid {
			t.Fatalf("%+v -> %+v", tc, info)
		}
	}
	if got := ParsePart(GoogleMIME, "attachment", "<target@example.com>", []byte(`{"version":1,"emoji":"👍"}`)); got != nil {
		t.Fatal("attachment became a reaction")
	}
	if got := ParsePart("text/plain", "reaction", "<target@example.com>", []byte("❤️")); got == nil || got.Invalid {
		t.Fatalf("RFC reaction: %+v", got)
	}
}

func TestNotificationIsExactAndReversible(t *testing.T) {
	for _, tc := range []struct {
		text, html string
		valid      bool
	}{
		{"👍 Example Sender reacted to your message:", "", true},
		{"", `<div>👍 <b>Example Sender</b> reacted to your message:</div>`, true},
		{"👍 Someone Else reacted to your message:", "", false},
		{"👍 Example Sender reacted to your message: Also please read this.", "", false},
		{"👍", "", false},
		{"", `<blockquote>👍 Example Sender reacted to your message:</blockquote><p>A real reply</p>`, false},
	} {
		info := FromNotification("Example Sender", "<target@example.com>", tc.text, tc.html)
		if (info != nil) != tc.valid {
			t.Fatalf("%+v -> %+v", tc, info)
		}
	}
	if FromNotification("Example Sender", "", "👍 Example Sender reacted to your message:", "") != nil {
		t.Fatal("unaddressed notification collapsed")
	}
}

func TestTargetRejectsUnbalancedIDs(t *testing.T) {
	for _, value := range []string{"<target@example.com", "target@example.com>", "<one@example.com> <two@example.com>"} {
		if Target(value) != "" {
			t.Fatalf("accepted invalid target %q", value)
		}
	}
}
