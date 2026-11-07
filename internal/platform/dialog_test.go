package platform

import (
	"testing"
)

func TestValidateDialogURL(t *testing.T) {
	tests := []struct {
		url     string
		allowed bool
	}{
		{"https://github.com/hkdb/aerion/blob/main/docs/SQL_ROLLBACK.md", true},
		{"http://example.com/docs", true},
		{"HTTP://example.com", true},
		{"HTTPS://example.com/path?foo=bar", true},
		{"mailto:support@example.com", true},
		{"mailto:?subject=Help", true},
		{"MAILTO:support@example.com", true},
		{"file:///etc/passwd", false},
		{"javascript:alert(1)", false},
		{"data:text/html,<script>alert(1)</script>", false},
		{"custom:something", false},
		{"http://", false},
		{"https://", false},
		{"", false},
		{"   ", false},
		{"http://-flag", false},
		{"https://--help", false},
		{"--help", false},
		{"-oProxyCommand=calc", false},
		{"mailto:--help", false},
		{"mailto:-flag@example.com", false},
		{"https://example.com\r\ninjected", false},
		{"https://example.com\x00injected", false},
	}

	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			got := ValidateDialogURL(tc.url)
			if got != tc.allowed {
				t.Errorf("ValidateDialogURL(%q) = %v, want %v", tc.url, got, tc.allowed)
			}
		})
	}
}
