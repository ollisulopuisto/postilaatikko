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

	handler, err := r.MailboxReadAPIHandler()
	if err != nil {
		return nil, err
	}
	config.V1Handler = handler
	return automation.NewServer(config), nil
}
