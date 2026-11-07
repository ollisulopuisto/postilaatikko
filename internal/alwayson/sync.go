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
	"time"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/imap"
	"github.com/hkdb/aerion/internal/logging"
	"github.com/hkdb/aerion/internal/mailfilter"
	"github.com/hkdb/aerion/internal/mailmove"
	"github.com/hkdb/aerion/internal/message"
	mailSync "github.com/hkdb/aerion/internal/sync"
	"github.com/hkdb/aerion/internal/syncowner"
)

// IMAPConfigSource supplies account credentials without making the runtime
// responsible for choosing or storing authentication secrets.
type IMAPConfigSource func(context.Context, string) (*imap.ClientConfig, error)

// SyncRuntime owns one local database handle, IMAP pool, sync engine, and
// scheduler. The caller starts it under the independently supervised helper's
// context and closes it before releasing the credential provider.
type SyncRuntime struct {
	db             *database.DB
	pool           *imap.Pool
	scheduler      *mailSync.Scheduler
	filterOp       mailmove.FilterOperation
	filterer       *mailmove.IMAPFilterMover
	filterQ        chan filterJob
	syncLockPath   string
	ctxMu          sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
	filterCancel   context.CancelFunc
	filterWG       sync.WaitGroup
	lifecycleWG    sync.WaitGroup
	started        bool
	queueCloseOnce sync.Once
	startOnce      sync.Once
	closeOnce      sync.Once
}

type filterJob struct {
	rule        *mailfilter.Rule
	destination *folder.Folder
	message     *message.Message
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
	messages := message.NewStore(db)
	runtime := &SyncRuntime{db: db, ctx: context.Background(), syncLockPath: filepath.Join(dataDirectory, "mail-sync.owner.lock")}
	pool := imap.NewPool(imap.DefaultPoolConfig(), func(accountID string) (*imap.ClientConfig, error) {
		runtime.ctxMu.RLock()
		ctx := runtime.ctx
		runtime.ctxMu.RUnlock()
		return credentials(ctx, accountID)
	})
	engine := mailSync.NewEngine(pool, accounts, folders, messages, message.NewAttachmentStore(db))
	scheduler := mailSync.NewScheduler(engine, accounts, folders)
	runtime.pool, runtime.scheduler = pool, scheduler
	runtime.filterQ = make(chan filterJob, 64)
	runtime.filterer = &mailmove.IMAPFilterMover{Messages: messages, Folders: folders, Raw: engine, Pool: pool}
	runtime.filterOp = mailmove.FilterOperation{Messages: messages, Move: func(ctx context.Context, _ string, source *message.Message, destination *folder.Folder) error {
		return runtime.filterer.MoveFilterMessage(ctx, source, destination.ID)
	}}
	router := mailfilter.NewAutomaticRouter(mailfilter.NewStore(db), folders, runtime.enqueueFilter)
	engine.SetNewMessageHandler(func(_ string, _ string, msg *message.Message) { router.Route(msg) })
	return runtime, nil
}

// Start begins the existing bounded, header-first periodic sync scheduler.
func (r *SyncRuntime) Start(ctx context.Context) {
	if r == nil {
		return
	}
	r.startOnce.Do(func() {
		if ctx == nil {
			ctx = context.Background()
		}
		workerContext, cancel := context.WithCancel(ctx)
		r.ctxMu.Lock()
		r.ctx = workerContext
		r.cancel = cancel
		r.started = true
		r.ctxMu.Unlock()
		r.lifecycleWG.Add(1)
		go r.runSyncLifecycle(workerContext)
	})
}

// Close stops background work before closing its IMAP and database resources.
func (r *SyncRuntime) Close() error {
	if r == nil {
		return nil
	}
	var closeErr error
	r.closeOnce.Do(func() {
		r.ctxMu.RLock()
		cancel, started := r.cancel, r.started
		r.ctxMu.RUnlock()
		if started && cancel != nil {
			cancel()
			r.lifecycleWG.Wait()
		} else {
			r.scheduler.Stop()
		}
		r.closeFilterQueue()
		r.cancelFilterWorker()
		r.pool.CloseAll()
		closeErr = r.db.Close()
	})
	return closeErr
}

func (r *SyncRuntime) cancelFilterWorker() {
	r.ctxMu.Lock()
	cancel := r.filterCancel
	r.filterCancel = nil
	r.ctxMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (r *SyncRuntime) runSyncLifecycle(ctx context.Context) {
	defer r.lifecycleWG.Done()
	log := logging.WithComponent("alwayson.sync")
	log.Info().Msg("Waiting for exclusive mailbox sync ownership")
	owner, err := syncowner.Acquire(ctx, r.syncLockPath)
	if err != nil {
		if ctx.Err() == nil {
			log.Error().Err(err).Msg("Could not acquire mailbox sync ownership")
		}
		return
	}
	defer func() {
		if err := owner.Release(); err != nil {
			log.Warn().Err(err).Msg("Failed to release mailbox sync ownership")
		}
	}()
	if ctx.Err() != nil {
		return
	}
	log.Info().Msg("Acquired mailbox sync ownership")
	filterContext, cancel := context.WithCancel(context.WithoutCancel(ctx))
	r.ctxMu.Lock()
	r.ctx, r.filterCancel = filterContext, cancel
	r.ctxMu.Unlock()
	r.filterWG.Add(1)
	go r.runFilterQueue(filterContext)
	r.scheduler.Start(ctx)
	<-ctx.Done()
	r.scheduler.Stop()
	r.closeFilterQueue()
	filterDone := make(chan struct{})
	go func() {
		r.filterWG.Wait()
		close(filterDone)
	}()
	select {
	case <-filterDone:
	case <-time.After(30 * time.Second):
		log.Warn().Msg("Filter shutdown deadline reached; canceling unfinished filter moves")
		cancel()
		<-filterDone
	}
	cancel()
}

func (r *SyncRuntime) closeFilterQueue() {
	r.queueCloseOnce.Do(func() { close(r.filterQ) })
}
