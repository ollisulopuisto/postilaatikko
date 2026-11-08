package app

import (
	"context"
	"fmt"

	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/logging"
	"github.com/hkdb/aerion/internal/mailfilter"
	"github.com/hkdb/aerion/internal/mailmove"
	"github.com/hkdb/aerion/internal/message"
)

// ============================================================================
// Local Cross-Account Filter API - Exposed to frontend via Wails bindings
// ============================================================================

// GetFilterRules returns all local cross-account filter rules in priority order.
func (a *App) GetFilterRules() ([]*mailfilter.Rule, error) {
	if a.mailboxService != nil {
		ctx := a.helperCommandContext()
		status := a.mailboxService.status(ctx)
		if status.Connected {
			return a.mailboxService.filterRules(ctx)
		}
		if status.ErrorCode != "service_unavailable" {
			return nil, fmt.Errorf("load filter rules from mailbox service: %s", status.Message)
		}
	}
	return a.filterStore.List()
}

// CreateFilterRule validates and persists a new local cross-account filter rule.
func (a *App) CreateFilterRule(rule *mailfilter.Rule) error {
	if rule == nil {
		return fmt.Errorf("filter rule cannot be nil")
	}
	if handled, err := a.mutateFilterRuleThroughService(v1.FilterRuleMutationRequest{Operation: "create", Rule: rule}); handled {
		return err
	}
	if err := a.validateFilterRule(rule); err != nil {
		return err
	}
	// New rules go to the end of the priority list.
	if rule.OrderIndex == 0 {
		if existing, listErr := a.filterStore.List(); listErr == nil {
			rule.OrderIndex = len(existing)
		}
	}
	return a.filterStore.Create(rule)
}

// UpdateFilterRule validates and persists changes to an existing filter rule.
func (a *App) UpdateFilterRule(rule *mailfilter.Rule) error {
	if rule == nil {
		return fmt.Errorf("filter rule cannot be nil")
	}
	if rule.ID == "" {
		return fmt.Errorf("filter rule ID cannot be empty")
	}
	if handled, err := a.mutateFilterRuleThroughService(v1.FilterRuleMutationRequest{Operation: "update", Rule: rule}); handled {
		return err
	}
	if err := a.validateFilterRule(rule); err != nil {
		return err
	}
	return a.filterStore.Update(rule)
}

// DeleteFilterRule removes a local cross-account filter rule.
func (a *App) DeleteFilterRule(id string) error {
	if id == "" {
		return fmt.Errorf("filter rule ID cannot be empty")
	}
	if handled, err := a.mutateFilterRuleThroughService(v1.FilterRuleMutationRequest{Operation: "delete", ID: id}); handled {
		return err
	}
	return a.filterStore.Delete(id)
}

// ReorderFilterRules saves the priority order of filter rules.
func (a *App) ReorderFilterRules(ids []string) error {
	if handled, err := a.mutateFilterRuleThroughService(v1.FilterRuleMutationRequest{Operation: "reorder", IDs: ids}); handled {
		return err
	}
	return a.filterStore.Reorder(ids)
}

func (a *App) mutateFilterRuleThroughService(req v1.FilterRuleMutationRequest) (bool, error) {
	if a.mailboxService == nil {
		return false, nil
	}
	ctx := a.helperCommandContext()
	status := a.mailboxService.status(ctx)
	if !status.Connected {
		if status.ErrorCode == "service_unavailable" {
			return false, nil
		}
		return true, fmt.Errorf("mailbox service is not ready for filter rule %s: %s", req.Operation, status.Message)
	}
	if !status.WriteAuthorized {
		return true, errDesktopMailboxWriteAuthorizationRequired
	}
	if err := a.mailboxService.mutateFilterRule(ctx, req); err != nil {
		return true, fmt.Errorf("run filter rule %s through local mailbox service: %w", req.Operation, err)
	}
	return true, nil
}

// validateFilterRule enforces the cross-account invariant: the destination
// folder must exist and, when the rule is scoped to one source account, it
// must live on a different account. A rule scoped to all accounts may not
// exclude anything; matching messages on the destination account are simply
// never routed (see mailfilter.Match).
func (a *App) validateFilterRule(rule *mailfilter.Rule) error {
	return mailfilter.ValidateRule(rule, a.accountStore, a.folderStore)
}

// handleNewMessageForFilters delegates rule selection to the reusable
// backend router. The app supplies its shared move pipeline as the applier.
func (a *App) handleNewMessageForFilters(msg *message.Message) {
	if a.filterRouter != nil {
		a.filterRouter.Route(msg)
	}
}

// applyFilterRule routes one message through the standard safe move pipeline:
// the cross-account branch APPENDs the raw message to the destination first
// and only deletes the source once that copy has been confirmed. The move
// runs under the shared per-message mutation lease so a concurrent user move
// of the same message is never raced, and it deliberately records no undo
// entry of its own to keep the undo stack clear of automated moves.
func (a *App) applyFilterRule(rule *mailfilter.Rule, dest *folder.Folder, msg *message.Message) {
	defer recoverPanic("app.filters", "apply filter rule")
	log := logging.WithComponent("app.filters")
	operation := mailmove.FilterOperation{
		Messages: a.messageStore,
		Move: func(_ context.Context, ruleID string, source *message.Message, destination *folder.Folder) error {
			_, _, _, err := a.runMoveMutation([]string{source.ID}, "filter:"+ruleID, func() (bool, string, error) {
				_, moveErr := a.moveToFolder([]string{source.ID}, destination.ID, false, "")
				return false, "", moveErr
			})
			return err
		},
	}
	applied, err := operation.Apply(a.ctx, rule, dest, msg)
	if err != nil {
		log.Warn().Err(err).Str("ruleID", rule.ID).Str("destFolder", dest.Path).Msg("Filter rule move failed")
		return
	}
	if !applied {
		log.Debug().Str("messageRef", logging.ShortHash(msg.ID)).Msg("Filter target changed since match, skipping")
		return
	}
	log.Info().Str("ruleID", rule.ID).Str("destFolder", dest.Path).Msg("Filter rule routed message")
}
