package alwayson

import (
	"errors"

	"github.com/hkdb/aerion/internal/automation"
)

// NewPersistentMailboxAPIServer composes the API with grants stored in the
// database already owned by this runtime.
func (r *SyncRuntime) NewPersistentMailboxAPIServer(config automation.ServerConfig) (*automation.Server, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("always-on runtime is not open")
	}
	if config.TokenStore != nil {
		return nil, errors.New("persistent mailbox API manages its token store")
	}
	config.TokenStore = automation.NewSQLiteTokenStore(r.db)
	return r.NewMailboxAPIServer(config)
}
