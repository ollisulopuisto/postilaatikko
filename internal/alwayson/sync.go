// Package alwayson composes the existing mailbox sync engine for a small
// headless process. It deliberately reuses sync's header-first scheduler
// instead of introducing a second provider implementation.
package alwayson

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/imap"
	"github.com/hkdb/aerion/internal/message"
	mailSync "github.com/hkdb/aerion/internal/sync"
)

// IMAPConfigSource supplies account credentials without making the runtime
// responsible for choosing or storing authentication secrets.
type IMAPConfigSource func(context.Context, string) (*imap.ClientConfig, error)

// SyncRuntime owns one local database handle, IMAP pool, sync engine, and
// scheduler. The caller starts it under the independently supervised helper's
// context and closes it before releasing the credential provider.
type SyncRuntime struct {
	db        *database.DB
	pool      *imap.Pool
	scheduler *mailSync.Scheduler
	ctxMu     sync.RWMutex
	ctx       context.Context
	startOnce sync.Once
	closeOnce sync.Once
}

// OpenSyncRuntime opens an existing Postilaatikko data directory. It refuses
// to create an empty database: the desktop must first configure accounts.
func OpenSyncRuntime(dataDirectory string, credentials IMAPConfigSource) (*SyncRuntime, error) {
	if credentials == nil {
		return nil, errors.New("always-on sync requires an IMAP credential source")
	}
	databasePath := filepath.Join(dataDirectory, "aerion.db")
	info, err := os.Stat(databasePath)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("configure mail accounts in Postilaatikko before starting the local sync service")
	}
	db, err := database.Open(databasePath)
	if err != nil {
		return nil, fmt.Errorf("open local mail database: %w", err)
	}
	closeOnError := func(err error) (*SyncRuntime, error) {
		_ = db.Close()
		return nil, err
	}
	if err := db.Migrate(); err != nil {
		return closeOnError(fmt.Errorf("migrate local mail database: %w", err))
	}

	accounts := account.NewStore(db)
	folders := folder.NewStore(db)
	runtime := &SyncRuntime{db: db, ctx: context.Background()}
	pool := imap.NewPool(imap.DefaultPoolConfig(), func(accountID string) (*imap.ClientConfig, error) {
		runtime.ctxMu.RLock()
		ctx := runtime.ctx
		runtime.ctxMu.RUnlock()
		return credentials(ctx, accountID)
	})
	engine := mailSync.NewEngine(pool, accounts, folders, message.NewStore(db), message.NewAttachmentStore(db))
	scheduler := mailSync.NewScheduler(engine, accounts, folders)
	runtime.pool, runtime.scheduler = pool, scheduler
	return runtime, nil
}

// Start begins the existing bounded, header-first periodic sync scheduler.
func (r *SyncRuntime) Start(ctx context.Context) {
	if r == nil {
		return
	}
	r.startOnce.Do(func() {
		r.ctxMu.Lock()
		r.ctx = ctx
		r.ctxMu.Unlock()
		r.scheduler.Start(ctx)
	})
}

// Close stops background work before closing its IMAP and database resources.
func (r *SyncRuntime) Close() error {
	if r == nil {
		return nil
	}
	var closeErr error
	r.closeOnce.Do(func() {
		r.scheduler.Stop()
		r.pool.CloseAll()
		closeErr = r.db.Close()
	})
	return closeErr
}
