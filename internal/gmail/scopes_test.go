package gmail

import "testing"

func TestScopesCover(t *testing.T) {
	const full = "https://mail.google.com/"
	const readonly = "https://www.googleapis.com/auth/gmail.readonly"
	const modify = "https://www.googleapis.com/auth/gmail.modify"
	const metadata = "https://www.googleapis.com/auth/gmail.metadata"
	tests := []struct {
		name       string
		scopes     []string
		wantSearch bool
		wantModify bool
	}{
		{"unknown scopes are allowed to try", nil, true, true},
		{"full mail access", []string{"openid", full}, true, true},
		{"readonly searches but cannot modify", []string{readonly}, true, false},
		{"modify does both", []string{modify}, true, true},
		{"metadata scope cannot use q", []string{metadata}, false, false},
		{"identity scopes only (user deselected mail)", []string{"openid", "email"}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ScopesCoverSearch(tt.scopes); got != tt.wantSearch {
				t.Errorf("search = %v, want %v", got, tt.wantSearch)
			}
			if got := ScopesCoverModify(tt.scopes); got != tt.wantModify {
				t.Errorf("modify = %v, want %v", got, tt.wantModify)
			}
		})
	}
}
