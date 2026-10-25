package gmail

const (
	scopeFullMail = "https://mail.google.com/"
	scopeReadonly = "https://www.googleapis.com/auth/gmail.readonly"
	scopeModify   = "https://www.googleapis.com/auth/gmail.modify"
)

func hasAny(scopes []string, wanted ...string) bool {
	for _, s := range scopes {
		for _, w := range wanted {
			if s == w {
				return true
			}
		}
	}
	return false
}

// ScopesCoverSearch reports whether the granted scopes allow messages.list
// with the q parameter. gmail.metadata does not. An empty list means the
// grant is unknown (accounts connected before granted scopes were stored), so
// the first API call decides.
func ScopesCoverSearch(scopes []string) bool {
	if len(scopes) == 0 {
		return true
	}
	return hasAny(scopes, scopeFullMail, scopeReadonly, scopeModify)
}

// ScopesCoverModify reports whether the granted scopes allow changing labels
// (messages.modify). Unknown grants are allowed to try.
func ScopesCoverModify(scopes []string) bool {
	if len(scopes) == 0 {
		return true
	}
	return hasAny(scopes, scopeFullMail, scopeModify)
}
