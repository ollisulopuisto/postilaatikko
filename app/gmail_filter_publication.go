package app

import (
	"context"
	"fmt"

	"github.com/ollisulopuisto/postilaatikko/internal/gmail"
	"github.com/ollisulopuisto/postilaatikko/internal/mailfilter"
)

type accountFilterProvider struct {
	accountID string
	client    *gmail.Client
}

func (p accountFilterProvider) AccountID() string { return p.accountID }
func (p accountFilterProvider) ListFilters(ctx context.Context) ([]gmail.Filter, error) {
	return p.client.ListFilters(ctx)
}
func (p accountFilterProvider) CreateFilter(ctx context.Context, f gmail.Filter) (*gmail.Filter, error) {
	return p.client.CreateFilter(ctx, f)
}
func (p accountFilterProvider) GetFilter(ctx context.Context, id string) (*gmail.Filter, error) {
	return p.client.GetFilter(ctx, id)
}

// requireStandaloneFilterOwner refuses both remote and localhost service owners.
// A disabled helper or a failed status probe is not proof of no other executor.
// Helper publication must run on that helper through its own guarded API.
func (a *App) requireStandaloneFilterOwner() error {
	if a.mailboxService != nil {
		return fmt.Errorf("Gmail publication requires standalone ownership; mailbox-service publication is not available")
	}
	if a.filterStore == nil || a.accountStore == nil || a.credStore == nil {
		return fmt.Errorf("local filter owner unavailable")
	}
	return nil
}
func (a *App) gmailFilterProvider(accountID string) (accountFilterProvider, error) {
	if err := a.requireStandaloneFilterOwner(); err != nil {
		return accountFilterProvider{}, err
	}
	email, tokens, _, err := a.prepareGmailFilterConsent(accountID)
	if err != nil {
		return accountFilterProvider{}, err
	}
	defer tokens.Clear()
	if email == "" || !gmail.ScopesCoverFilters(tokens.Scopes) {
		return accountFilterProvider{}, fmt.Errorf("explicit Gmail settings consent required")
	}
	if a.composeOps.credStore == nil {
		return accountFilterProvider{}, fmt.Errorf("account token owner unavailable")
	}
	client := gmail.NewClient(func(_ context.Context, force bool) (string, error) {
		if err := a.requireStandaloneFilterOwner(); err != nil {
			return "", err
		}
		tokens, err := a.gmailTokensFor(accountID, gmail.ScopesCoverFilters, force)
		if err != nil {
			return "", err
		}
		defer tokens.Clear()
		return tokens.AccessToken, nil
	})
	return accountFilterProvider{accountID: accountID, client: client}, nil
}

// prepareLocalGmailPublication is internal until the UI can supply an explicitly
// reviewed provider-native payload. It never stops a source rule implicitly.
func (a *App) prepareLocalGmailPublication(ctx context.Context, ruleID, accountID string, draft gmail.Filter) (*mailfilter.GmailPublication, error) {
	a.filterExecutionMu.Lock()
	defer a.filterExecutionMu.Unlock()
	provider, err := a.gmailFilterProvider(accountID)
	if err != nil {
		return nil, err
	}
	catalog, err := provider.ListFilters(ctx)
	if err != nil {
		return nil, err
	}
	labels, err := provider.client.Labels(ctx)
	if err != nil {
		return nil, err
	}
	allowed := make([]string, 0, len(labels))
	for _, label := range labels {
		allowed = append(allowed, label.ID)
	}
	return a.filterStore.PrepareGmailPublication(ruleID, accountID, draft, catalog, allowed)
}

// publishLocalGmailPublication excludes direct in-flight app filtering, then
// verifies the durable source guard before making the one provider attempt.
func (a *App) publishLocalGmailPublication(ctx context.Context, id string) (*mailfilter.GmailPublication, error) {
	a.filterExecutionMu.Lock()
	defer a.filterExecutionMu.Unlock()
	if err := a.requireStandaloneFilterOwner(); err != nil {
		return nil, err
	}
	plan, err := a.filterStore.GetGmailPublication(id)
	if err != nil {
		return nil, err
	}
	provider, err := a.gmailFilterProvider(plan.AccountID)
	if err != nil {
		return nil, err
	}
	return a.filterStore.PublishGmailFilter(ctx, id, provider)
}
func (a *App) reconcileLocalGmailPublication(ctx context.Context, id string) (*mailfilter.GmailPublication, error) {
	if err := a.requireStandaloneFilterOwner(); err != nil {
		return nil, err
	}
	plan, err := a.filterStore.GetGmailPublication(id)
	if err != nil {
		return nil, err
	}
	provider, err := a.gmailFilterProvider(plan.AccountID)
	if err != nil {
		return nil, err
	}
	return a.filterStore.ReconcileGmailPublication(ctx, id, provider)
}
