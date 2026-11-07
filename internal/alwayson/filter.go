package alwayson

import (
	"context"
	"time"

	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/logging"
	"github.com/hkdb/aerion/internal/mailfilter"
	"github.com/hkdb/aerion/internal/message"
)

func (r *SyncRuntime) enqueueFilter(rule *mailfilter.Rule, destination *folder.Folder, msg *message.Message) {
	if r == nil || rule == nil || destination == nil || msg == nil {
		return
	}
	r.ctxMu.RLock()
	ctx := r.ctx
	r.ctxMu.RUnlock()
	if ctx == nil {
		return
	}
	select {
	case r.filterQ <- filterJob{rule: rule, destination: destination, message: msg}:
	case <-ctx.Done():
	}
}

func (r *SyncRuntime) runFilterQueue(ctx context.Context) {
	defer r.filterWG.Done()
	log := logging.WithComponent("alwayson.filters")
	for {
		if ctx.Err() != nil {
			return
		}
		var job filterJob
		select {
		case <-ctx.Done():
			return
		case next, open := <-r.filterQ:
			if !open {
				return
			}
			job = next
		}
		jobContext, cancel := context.WithTimeout(ctx, 45*time.Second)
		applied, err := r.filterOp.Apply(jobContext, job.rule, job.destination, job.message)
		cancel()
		if err != nil {
			log.Warn().Err(err).Str("ruleID", job.rule.ID).Msg("Automatic filter move failed")
			continue
		}
		if applied {
			log.Info().Str("ruleID", job.rule.ID).Str("destinationFolder", job.destination.Path).Msg("Automatic filter routed message")
		}
	}
}
