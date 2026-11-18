package app

import (
	"strings"
	"testing"
)

func TestParseMailtoURL_Valid(t *testing.T) {
	result := ParseMailtoURL("mailto:test@example.com")
	if result == nil {
		t.Fatal("ParseMailtoURL returned nil, want non-nil")
	}
	if len(result.To) != 1 || result.To[0] != "test@example.com" {
		t.Errorf("To = %v, want [test@example.com]", result.To)
	}
}

func TestParseMailtoURL_WithParams(t *testing.T) {
	result := ParseMailtoURL("mailto:test@example.com?subject=Hello&body=World")
	if result == nil {
		t.Fatal("ParseMailtoURL returned nil, want non-nil")
	}
	if result.Subject != "Hello" {
		t.Errorf("Subject = %q, want %q", result.Subject, "Hello")
	}
	if result.Body != "World" {
		t.Errorf("Body = %q, want %q", result.Body, "World")
	}
}

func TestParseMailtoURL_MultiRecipient(t *testing.T) {
	result := ParseMailtoURL("mailto:a@b.com,c@d.com")
	if result == nil {
		t.Fatal("ParseMailtoURL returned nil, want non-nil")
	}
	if len(result.To) != 2 {
		t.Fatalf("len(To) = %d, want 2", len(result.To))
	}
	if result.To[0] != "a@b.com" {
		t.Errorf("To[0] = %q, want %q", result.To[0], "a@b.com")
	}
	if result.To[1] != "c@d.com" {
		t.Errorf("To[1] = %q, want %q", result.To[1], "c@d.com")
	}
}

func TestParseMailtoURL_WithCcBcc(t *testing.T) {
	result := ParseMailtoURL("mailto:a@b.com?cc=c@d.com&bcc=e@f.com")
	if result == nil {
		t.Fatal("ParseMailtoURL returned nil, want non-nil")
	}
	if len(result.Cc) != 1 || result.Cc[0] != "c@d.com" {
		t.Errorf("Cc = %v, want [c@d.com]", result.Cc)
	}
	if len(result.Bcc) != 1 || result.Bcc[0] != "e@f.com" {
		t.Errorf("Bcc = %v, want [e@f.com]", result.Bcc)
	}
}

func TestParseMailtoURL_TooLong(t *testing.T) {
	longURL := "mailto:test@example.com?" + strings.Repeat("x", 3000)
	result := ParseMailtoURL(longURL)
	if result != nil {
		t.Error("ParseMailtoURL with 3000+ char URL should return nil")
	}
}

func TestParseMailtoURL_InvalidScheme(t *testing.T) {
	result := ParseMailtoURL("http://example.com")
	if result != nil {
		t.Error("ParseMailtoURL with http:// scheme should return nil")
	}
}

func TestParseMailtoURL_Empty(t *testing.T) {
	result := ParseMailtoURL("")
	if result != nil {
		t.Error("ParseMailtoURL with empty string should return nil")
	}
}

func TestParseMailtoURL_NoAddress(t *testing.T) {
	result := ParseMailtoURL("mailto:?subject=Hello")
	if result == nil {
		t.Fatal("ParseMailtoURL returned nil, want non-nil")
	}
	if len(result.To) != 0 {
		t.Errorf("To = %v, want empty", result.To)
	}
	if result.Subject != "Hello" {
		t.Errorf("Subject = %q, want %q", result.Subject, "Hello")
	}
}

func TestIsAllowedProtocol(t *testing.T) {
	tests := []struct {
		url     string
		allowed bool
	}{
		{"http://example.com", true},
		{"https://example.com", true},
		{"HTTP://example.com", true},
		{"HTTPS://example.com/path?foo=bar", true},
		{"mailto:user@example.com", true},
		{"mailto:?subject=Test", true},
		{"MAILTO:user@example.com", true},
		{"file:///etc/passwd", false},
		{"javascript:alert(1)", false},
		{"data:text/html,<script>alert(1)</script>", false},
		{"custom:something", false},
		{"http://", false},
		{"https://", false},
		{"", false},
		{"   ", false},
		{"http://-flag", false},
		{"https://--version", false},
		{"http://-flag:8080/path", false},
		{false},
		{"mailto:-flag@example.com", false},
		{"mailto:%2D%2Dhelp", false},
		{"mailto:%2d-flag@example.com", false},
		{"http://example.com\r\ninjected", false},
		{"http://example.com\x00injected", false},
	}

	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			got := isAllowedProtocol(tc.url)
			if got != tc.allowed {
				t.Errorf("isAllowedProtocol(%q) = %v, want %v", tc.url, got, tc.allowed)
			}
		})
	}
}

func TestParseMailtoURL_PlusIsLiteral(t *testing.T) {
	result := ParseMailtoURL("mailto:olli+news@example.com?subject=C%2B%2B+jobs&body=a+b")
	if result == nil {
		t.Fatal("ParseMailtoURL returned nil")
	}
	if len(result.To) != 1 || result.To[0] != "olli+news@example.com" {
		t.Errorf("To = %v, want [olli+news@example.com]", result.To)
	}
	if result.Subject != "C+++jobs" {
		t.Errorf("Subject = %q, want %q", result.Subject, "C+++jobs")
	}
	if result.Body != "a+b" {
		t.Errorf("Body = %q, want %q", result.Body, "a+b")
	}
}

func TestParseMailtoURL_PercentAndSpaces(t *testing.T) {
	result := ParseMailtoURL("mailto:hi@callsono.com?subject=Application: Forward Deployed Engineer&body=Hei%20maailma%0A%C3%A4")
	if result == nil {
		t.Fatal("ParseMailtoURL returned nil")
	}
	if result.Subject != "Application: Forward Deployed Engineer" {
		t.Errorf("Subject = %q", result.Subject)
	}
	// sanitizeField strips the newline; the UTF-8 byte pair must decode to ä.
	if result.Body != "Hei maailmaä" {
		t.Errorf("Body = %q", result.Body)
	}
}

func TestParseMailtoURL_ToHeaderAndCaseInsensitiveNames(t *testing.T) {
	result := ParseMailtoURL("mailto:a@example.com?TO=b@example.com&Cc=c@example.com&BCC=d@example.com&Subject=Hi")
	if result == nil {
		t.Fatal("ParseMailtoURL returned nil")
	}
	if len(result.To) != 2 || result.To[0] != "a@example.com" || result.To[1] != "b@example.com" {
		t.Errorf("To = %v", result.To)
	}
	if len(result.Cc) != 1 || len(result.Bcc) != 1 || result.Subject != "Hi" {
		t.Errorf("Cc=%v Bcc=%v Subject=%q", result.Cc, result.Bcc, result.Subject)
	}
}

func TestHandleOpenURL_BeforeStartupWaitsAsPending(t *testing.T) {
	a := &App{}
	a.HandleOpenURL("mailto:x@example.com?subject=Moi")
	got := a.GetPendingMailto()
	if got == nil || len(got.To) != 1 || got.To[0] != "x@example.com" || got.Subject != "Moi" {
		t.Fatalf("pending = %+v", got)
	}
	if a.GetPendingMailto() != nil {
		t.Error("pending mailto should be cleared after reading")
	}
}

func TestHandleOpenURL_IgnoresOtherSchemesAndGarbage(t *testing.T) {
	a := &App{}
	for _, raw := range []string{"https://example.com", "file:///etc/passwd", "", "javascript:alert(1)"} {
		a.HandleOpenURL(raw)
		if a.PendingMailto != nil {
			t.Fatalf("%q must not create pending mailto: %+v", raw, a.PendingMailto)
		}
	}
}
