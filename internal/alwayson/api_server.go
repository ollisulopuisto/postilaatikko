package alwayson

import (
	"errors"

	"github.com/hkdb/aerion/internal/automation"
)

// NewMailboxAPIServer composes the headless mailbox read handler with the
// authenticated automation server. The caller owns Start and Stop so the
// listener can follow the helper's service lifecycle.
func (r *SyncRuntime) NewMailboxAPIServer(config automation.ServerConfig) (*automation.Server, error) {
	if config.TokenStore == nil {
		return nil, errors.New("always-on API requires an explicit token store")
	}
	if config.V1Handler != nil {
		return nil, errors.New("always-on API manages its own v1 handler")
	}

	// Long-lived event streams revalidate against the same token store the
	// authenticating middleware uses, so a revoked grant stops receiving
	// events within the stream's revalidation interval (PERF-SC-004).
	r.grantChecker = tokenStoreGrantChecker{store: config.TokenStore}

	handler, err := r.MailboxReadAPIHandler()
	if err != nil {
		return nil, err
	}
	config.V1Handler = handler
	return automation.NewServer(config), nil
}

// tokenStoreGrantChecker adapts the automation token store to the events
// route's revocation check.
type tokenStoreGrantChecker struct {
	store automation.TokenStore
}

// GrantRevoked reports whether the grant for clientID is revoked or gone.
func (c tokenStoreGrantChecker) GrantRevoked(clientID string) (bool, error) {
	grants, err := c.store.ListClients()
	if err != nil {
		return false, err
	}
	for _, grant := range grants {
		if grant.ClientID == clientID {
			return grant.Revoked, nil
		}
	}
	return true, nil
}
