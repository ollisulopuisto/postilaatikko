package app

import (
	"context"
	"fmt"

	automationclient "github.com/ollisulopuisto/postilaatikko/internal/automation/client"
	v1 "github.com/ollisulopuisto/postilaatikko/internal/automation/v1"
	"github.com/ollisulopuisto/postilaatikko/internal/mailfilter"
)

// FilterExecutionTarget describes the current workspace, not an independent
// connection. Rule IDs and folder IDs must never be copied between workspaces.
type FilterExecutionTarget struct {
	Target          string `json:"target"`
	Endpoint        string `json:"endpoint"`
	Ready           bool   `json:"ready"`
	WriteAuthorized bool   `json:"writeAuthorized"`
	ErrorCode       string `json:"errorCode"`
}

// GetFilterExecutionTarget reports the configured owner and current readiness.
func (a *App) GetFilterExecutionTarget() FilterExecutionTarget {
	state := FilterExecutionTarget{Target: "local", Ready: true, WriteAuthorized: true}
	if a.mailboxService == nil {
		return state
	}
	if a.mailboxService.isRemote() {
		state.Target = "helper"
	}
	state.Endpoint = a.GetMailboxServiceEndpoint()
	status := a.mailboxService.status(a.helperCommandContext())
	if state.Target == "local" && status.ErrorCode == "service_unavailable" {
		return state
	}
	state.Ready, state.WriteAuthorized, state.ErrorCode = status.Connected, status.WriteAuthorized, status.ErrorCode
	return state
}

// SaveFilterRuleToTarget binds a save to the workspace the user reviewed.
// A target mismatch, offline helper or missing write grant is a hard failure.
// There is no cross-host transfer and no remote-to-local fallback.
func (a *App) SaveFilterRuleToTarget(target, endpoint string, rule, expected *mailfilter.Rule) error {
	state := a.GetFilterExecutionTarget()
	if target != state.Target || endpoint != state.Endpoint {
		return fmt.Errorf("filter workspace changed; reopen the editor")
	}
	if !state.Ready || !state.WriteAuthorized {
		return fmt.Errorf("filter execution target is not ready for verified writes")
	}
	ctx := a.helperCommandContext()
	adapter := &filterTargetStore{app: a}
	if a.mailboxService != nil {
		status := a.mailboxService.status(ctx)
		if status.Connected {
			if !status.WriteAuthorized {
				return errDesktopMailboxWriteAuthorizationRequired
			}
			client, err := a.mailboxService.getClient()
			if err != nil {
				return err
			}
			a.mailboxService.mu.Lock()
			currentEndpoint := a.mailboxService.endpoint
			if currentEndpoint == "" {
				currentEndpoint = mailboxServiceEndpoint
			}
			unchanged := currentEndpoint == endpoint && a.mailboxService.client == client
			a.mailboxService.mu.Unlock()
			if !unchanged {
				return fmt.Errorf("filter workspace changed; reopen the editor")
			}
			adapter.client = client
			return mailfilter.SaveVerifiedRule(ctx, adapter, rule, expected)
		}
		if target == "helper" || status.ErrorCode != "service_unavailable" {
			return fmt.Errorf("filter execution target became unavailable")
		}
	}
	return mailfilter.SaveVerifiedRule(ctx, adapter, rule, expected)
}

type filterTargetStore struct {
	app    *App
	client *automationclient.Client
}

func (s *filterTargetStore) List(ctx context.Context) ([]*mailfilter.Rule, error) {
	if s.client != nil {
		return s.client.GetFilterRules(ctx)
	}
	if s.app.filterStore == nil {
		return nil, fmt.Errorf("local filter store unavailable")
	}
	return s.app.filterStore.List()
}
func (s *filterTargetStore) Create(ctx context.Context, rule *mailfilter.Rule) error {
	if s.client != nil {
		return s.client.MutateFilterRule(ctx, v1.FilterRuleMutationRequest{Operation: "create", Rule: rule})
	}
	if err := s.app.validateFilterRule(rule); err != nil {
		return err
	}
	return s.app.filterStore.Create(rule)
}
func (s *filterTargetStore) Update(ctx context.Context, rule *mailfilter.Rule) error {
	if s.client != nil {
		return s.client.MutateFilterRule(ctx, v1.FilterRuleMutationRequest{Operation: "update", Rule: rule})
	}
	if err := s.app.validateFilterRule(rule); err != nil {
		return err
	}
	return s.app.filterStore.Update(rule)
}
