package oauth2

import (
	"reflect"
	"testing"
)

func TestGrantedScopes(t *testing.T) {
	requested := []string{"https://mail.google.com/", "openid"}
	tests := []struct {
		name string
		resp *TokenResponse
		want []string
	}{
		{"nil response keeps requested", nil, requested},
		{"empty scope keeps requested", &TokenResponse{}, requested},
		{"granted subset wins (granular consent)", &TokenResponse{Scope: "openid https://www.googleapis.com/auth/userinfo.email"}, []string{"openid", "https://www.googleapis.com/auth/userinfo.email"}},
		{"extra whitespace is ignored", &TokenResponse{Scope: "  openid   email "}, []string{"openid", "email"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GrantedScopes(tt.resp, requested); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
