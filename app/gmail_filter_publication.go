package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/mailfilter"
)

type accountFilterProvider struct {
	accountID string
	binding   string
	email     string
	app       *App
	client    *gmail.Client
}

func (p accountFilterProvider) AccountID() string    { return p.accountID }
func (p accountFilterProvider) OwnerBinding() string { return p.binding }
func gmailOwnerBinding(email, clientID string) string {
	data, _ := json.Marshal([]string{strings.ToLower(strings.TrimSpace(email)), clientID})
	return string(data)
}

// snapshotClient pins one token across identity verification and the following
// operation. Account/credential replacement cannot switch the write's mailbox.
func (p accountFilterProvider) snapshotClient(ctx context.Context) (*gmail.Client, error) {
	if err := p.app.requireStandaloneFilterOwner(); err != nil {
		return nil, err
	}
	email, previous, cfg, err := p.app.prepareGmailFilterConsent(p.accountID)
	if err != nil {
		return nil, err
	}
	previous.Clear()
	if gmailOwnerBinding(email, cfg.ClientID) != p.binding {
		return nil, fmt.Errorf("Gmail account or OAuth project changed")
	}
	tokens, err := p.app.gmailTokensFor(p.accountID, gmail.ScopesCoverFilters, false)
	if err != nil {
		return nil, err
	}
	defer tokens.Clear()
	token := tokens.AccessToken
	client := gmail.NewClient(func(_ context.Context, force bool) (string, error) {
		if force {
			return "", fmt.Errorf("verified publication token expired; no write retry")
		}
		return token, nil
	})
	actual, err := client.ProfileEmail(ctx)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(actual), strings.TrimSpace(p.email)) {
		return nil, fmt.Errorf("authenticated Gmail identity does not match reviewed account")
	}
	return client, nil
}
func (p accountFilterProvider) ListFilters(ctx context.Context) ([]gmail.Filter, error) {
	client, err := p.snapshotClient(ctx)
	if err != nil {
		return nil, err
	}
	return client.ListFilters(ctx)
}
func (p accountFilterProvider) CreateFilter(ctx context.Context, f gmail.Filter) (*gmail.Filter, error) {
	client, err := p.snapshotClient(ctx)
	if err != nil {
		return nil, err
	}
	return client.CreateFilter(ctx, f)
}
func (p accountFilterProvider) GetFilter(ctx context.Context, id string) (*gmail.Filter, error) {
	client, err := p.snapshotClient(ctx)
	if err != nil {
		return nil, err
	}
	return client.GetFilter(ctx, id)
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
	email, tokens, cfg, err := a.prepareGmailFilterConsent(accountID)
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
	return accountFilterProvider{accountID: accountID, binding: gmailOwnerBinding(email, cfg.ClientID), email: email, app: a, client: client}, nil
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
	if draft.Action.Forward != "" {
		addresses, err := provider.client.ListForwardingAddresses(ctx)
		if err != nil {
			return nil, err
		}
		return a.filterStore.PrepareGmailForwardingPublication(ruleID, accountID, draft, catalog, allowed, addresses, provider.OwnerBinding())
	}
	return a.filterStore.PrepareGmailPublication(ruleID, accountID, draft, catalog, allowed, provider.OwnerBinding())
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
	if plan.OwnerBinding == "" {
		return nil, fmt.Errorf("publication predates owner verification; cancel and review again")
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
	if plan.OwnerBinding == "" {
		return nil, fmt.Errorf("publication lacks durable owner verification; inspect manually")
	}
	return a.filterStore.ReconcileGmailPublication(ctx, id, provider)
}

// PrepareFilterCreate verifies identity before the journal consumes its attempt.
func (p accountFilterProvider) PrepareFilterCreate(ctx context.Context) (mailfilter.NativeFilterProvider, error) {
	client, err := p.snapshotClient(ctx)
	if err != nil {
		return nil, err
	}
	return pinnedAccountFilterCreate{accountFilterProvider: p, pinned: client}, nil
}

type pinnedAccountFilterCreate struct {
	accountFilterProvider
	pinned *gmail.Client
}

func (p pinnedAccountFilterCreate) CreateFilter(ctx context.Context, f gmail.Filter) (*gmail.Filter, error) {
	return p.pinned.CreateFilter(ctx, f)
}

func (p pinnedAccountFilterCreate) ListForwardingAddresses(ctx context.Context) ([]gmail.ForwardingAddress, error) {
	return p.pinned.ListForwardingAddresses(ctx)
}
