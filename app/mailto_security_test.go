package app

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseMailtoURL_UntrustedFields(t *testing.T) {
	raw := "mailto:to@example.com?cc=cc@example.com&bcc=bcc@example.com&subject=Hi%0d%0aBcc%3Aevil%40example.com%00%09&body=%3Cimg%20src%3Dhttps%3A%2F%2Fevil.example%2Fx%3E%0D%0Aplain&attach=/etc/passwd&attachment=file:///private/key&in-reply-to=evil&references=evil&x-header=evil&from=evil@example.com"
	got := ParseMailtoURL(raw)
	if got == nil || len(got.Cc) != 1 || len(got.Bcc) != 1 {
		t.Fatalf("allowed recipients lost: %+v", got)
	}
	if got.Subject != "HiBcc:evil@example.com" {
		t.Fatalf("header controls retained: %q", got.Subject)
	}
	if got.Body != "<img src=https://evil.example/x>\nplain" {
		t.Fatalf("body text/newlines changed: %q", got.Body)
	}
}

func TestParseMailtoURL_RejectDuplicateAllowedHeaders(t *testing.T) {
	for _, query := range []string{"body=first&body=second", "Subject=first&subject=second", "cc=a@example.com&CC=b@example.com", "to=a@example.com&to=b@example.com", "bcc=a@example.com&bcc=b@example.com"} {
		if got := ParseMailtoURL("mailto:to@example.com?" + query); got != nil {
			t.Fatalf("ambiguous duplicate accepted: %s %+v", query, got)
		}
	}
}

func TestParseMailtoURL_ControlAndLengthLimits(t *testing.T) {
	if ParseMailtoURL("mailto:to@example.com?body="+strings.Repeat("a", maxMailtoURLLength)) != nil {
		t.Fatal("oversized URL accepted")
	}
	got := ParseMailtoURL("mailto:to@example.com?subject=" + strings.Repeat("a", 1100) + "&body=a%00%01%09b%0Ac")
	if got == nil || len(got.Subject) > maxSubjectLength || got.Body != "a\tb\nc" {
		t.Fatalf("limits/controls: %+v", got)
	}
}

func TestParseMailtoURL_OnlyFiveFieldsAndLiteralUserExample(t *testing.T) {
	got := ParseMailtoURL("mailto:dst@mail.instinct.com?subject=Postilaatikko%20mailto%20test%2017%3A59&body=Sent%20from%20Postilaatikko%20via%20a%20mailto%20link&cc=cc@example.com&bcc=bcc@example.com&attach=/etc/passwd&attachment=C%3A%5Csecret.txt&in-reply-to=id&references=id&x-any=bad")
	if got == nil || got.Subject != "Postilaatikko mailto test 17:59" || got.Body != "Sent from Postilaatikko via a mailto link" || len(got.To) != 1 || len(got.Cc) != 1 || len(got.Bcc) != 1 {
		t.Fatalf("unexpected parsed fields: %+v", got)
	}
	// No arbitrary metadata/attachment carrier exists on this parsing result.
	typ := reflect.TypeOf(*got)
	for i := 0; i < typ.NumField(); i++ {
		switch typ.Field(i).Name {
		case "To", "Cc", "Bcc", "Subject", "Body", "Raw":
		default:
			t.Fatalf("unexpected field: %s", typ.Field(i).Name)
		}
	}
}

func TestParseMailtoURL_RejectMalformedAndStripVisualHeaderControls(t *testing.T) {
	for _, raw := range []string{"mailto:to@example.com?body=%", "mailto:to@example.com?body=a;b"} {
		if ParseMailtoURL(raw) != nil {
			t.Fatalf("malformed accepted: %s", raw)
		}
	}
	got := ParseMailtoURL("mailto:to@example.com?subject=before%E2%80%A8%E2%80%A9%E2%80%AE%E2%81%A6after")
	if got == nil || got.Subject != "beforeafter" {
		t.Fatalf("visual controls retained: %+v", got)
	}
}
