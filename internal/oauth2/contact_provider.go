package oauth2

import "fmt"

// ContactRefreshProvider resolves the OAuth provider config used to refresh
// contact-source tokens. Accepts both the contacts-only providers and the
// historical bare names ("google"/"microsoft") stored by older flows.
func ContactRefreshProvider(name string) (ProviderConfig, error) {
	switch name {
	case "google":
		return GoogleContactsOnlyProvider(), nil
	case "microsoft":
		return MicrosoftContactsOnlyProvider(), nil
	case "google-contacts", "microsoft-contacts":
		return GetProvider(name)
	default:
		return ProviderConfig{}, fmt.Errorf("unsupported contact OAuth provider: %s", name)
	}
}
