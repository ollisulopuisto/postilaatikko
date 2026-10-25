package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/crypto"
	"github.com/hkdb/aerion/internal/draft"
	"github.com/hkdb/aerion/internal/scheduledsend"
	"github.com/hkdb/aerion/internal/smtp"
	"github.com/zalando/go-keyring"
)

// SendLaterSetup contains public pairing information and the account mapping.
type SendLaterSetup struct {
	Config       scheduledsend.ClientConfig `json:"config"`
	ClientKey    string                     `json:"clientKey"`
	KeyAvailable bool                       `json:"keyAvailable"`
}

// ScheduleResult retains a persisted job ID even when no authenticated helper
// acknowledgement arrived. The composer must keep its draft in that case.
type ScheduleResult struct {
	Job   scheduledsend.ClientJob `json:"job"`
	Error string                  `json:"error,omitempty"`
}

func clientStore(data string) (*scheduledsend.ClientStore, error) {
	return scheduledsend.OpenClientStore(filepath.Join(data, "send-later-client"))
}

func getSendLaterSetup(ctx context.Context, data, accountID string) (SendLaterSetup, error) {
	var setup SendLaterSetup
	store, err := clientStore(data)
	if err != nil {
		return setup, err
	}
	defer store.Close()
	setup.Config, err = store.Config(ctx, accountID)
	if err != nil {
		return setup, err
	}
	identity, err := scheduledsend.LoadIdentity("client")
	if errors.Is(err, keyring.ErrNotFound) {
		setup.ClientKey, err = store.PinnedIdentity(ctx)
		return setup, err
	}
	if err != nil {
		return setup, err
	}
	if err := store.BindIdentity(ctx, identity); err != nil {
		return setup, err
	}
	setup.ClientKey, setup.KeyAvailable = identity.PublicKey().String(), true
	return setup, nil
}

func initializeSendLaterClient(ctx context.Context, data string) (string, error) {
	directory := filepath.Join(data, "send-later-client")
	// Reuse the existing cross-process OS lock in a separate client directory.
	// This serializes explicit key initialization across detached composers.
	lock, err := crypto.LockCredentialMigration(directory)
	if err != nil {
		return "", err
	}
	defer lock.Close()
	store, err := clientStore(data)
	if err != nil {
		return "", err
	}
	defer store.Close()
	identity, err := scheduledsend.LoadIdentity("client")
	if errors.Is(err, keyring.ErrNotFound) {
		pinned, readErr := store.PinnedIdentity(ctx)
		if readErr != nil {
			return "", readErr
		}
		if pinned != "" {
			return "", errors.New("queued jobs use a missing device key; recover it or reconcile them on the Studio before creating a replacement")
		}
		identity, err = scheduledsend.CreateIdentity("client")
	}
	if err != nil {
		return "", err
	}
	if err := store.BindIdentity(ctx, identity); err != nil {
		return "", err
	}
	return identity.PublicKey().String(), nil
}

func scheduleMessage(ctx context.Context, data string, ops *composeOps, drafts *draft.Store, accountID string, message smtp.ComposeMessage, draftID, due string) (ScheduleResult, error) {
	var result ScheduleResult
	when, err := time.Parse(time.RFC3339, due)
	if err != nil || !when.After(time.Now()) {
		return result, errors.New("choose a future delivery time with an explicit timezone")
	}
	savedDraft, err := drafts.Get(draftID)
	if err != nil || savedDraft == nil || savedDraft.AccountID != accountID {
		return result, errors.New("save the message as a draft in the selected sending account before scheduling")
	}
	acc, err := ops.accountStore.Get(accountID)
	if err != nil || acc == nil || !acc.Enabled || acc.NoOutgoingServer {
		return result, errors.New("selected sending account is unavailable")
	}
	if err := scheduledSender(ops.accountStore, accountID, message.From.Address); err != nil {
		return result, err
	}
	// Reject large compose inputs before MIME/base64 generation creates more
	// copies. Final MIME size is checked again inside the encrypted protocol.
	approximateSize := len(message.TextBody) + len(message.HTMLBody) + 4096
	for _, attachment := range message.Attachments {
		bytes := len(attachment.Content)
		if bytes == 0 {
			bytes = len(attachment.ContentBase64) * 3 / 4
		}
		approximateSize += (bytes+2)/3*4 + bytes/57*2 + 1024
		if approximateSize > scheduledsend.MaxMessageBytes {
			return result, errors.New("scheduled message exceeds the current 10 MiB MIME limit")
		}
	}
	if approximateSize > scheduledsend.MaxMessageBytes {
		return result, errors.New("scheduled message exceeds the current 10 MiB MIME limit")
	}
	identity, err := scheduledsend.LoadIdentity("client")
	if err != nil {
		return result, fmt.Errorf("unlock or initialize the Send Later device key: %w", err)
	}
	raw, err := ops.prepareMessageAt(accountID, message, when)
	if err != nil {
		return result, err
	}
	defer clear(raw)
	store, err := clientStore(data)
	if err != nil {
		return result, err
	}
	defer store.Close()
	result.Job, err = store.Schedule(ctx, accountID, draftID, scheduledsend.Delivery{
		From: message.From.Address, To: message.AllRecipients(), RFC822: raw, DueAt: when}, identity)
	if err != nil && result.Job.ID != "" {
		result.Error = err.Error()
		return result, nil
	}
	return result, err
}

func scheduledSender(store *account.Store, accountID, sender string) error {
	identities, err := store.GetIdentities(accountID)
	if err != nil {
		return err
	}
	for _, identity := range identities {
		if strings.EqualFold(identity.Email, sender) {
			return nil
		}
	}
	return errors.New("select a configured sender identity before scheduling")
}

func updateScheduledMessage(ctx context.Context, data, id, action string) (ScheduleResult, error) {
	var result ScheduleResult
	identity, err := scheduledsend.LoadIdentity("client")
	if err != nil {
		return result, err
	}
	store, err := clientStore(data)
	if err != nil {
		return result, err
	}
	defer store.Close()
	switch action {
	case "retry":
		result.Job, err = store.Submit(ctx, id, identity)
	case "refresh":
		result.Job, err = store.Refresh(ctx, id, identity)
	case "cancel":
		result.Job, err = store.Cancel(ctx, id, identity)
	default:
		return result, errors.New("unknown scheduled-message action")
	}
	if err != nil && result.Job.ID != "" {
		result.Error = err.Error()
		return result, nil
	}
	return result, err
}

func guardScheduledDraft(ctx context.Context, data, accountID, draftID string) error {
	if draftID == "" {
		return nil
	}
	store, err := clientStore(data)
	if err != nil {
		return err
	}
	defer store.Close()
	job, err := store.FindDraft(ctx, accountID, draftID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("this draft already has a Send Later job (%s); inspect or cancel it before sending again", job.State)
}

// GetSendLaterSetup returns public device pairing and account helper settings.
func (a *App) GetSendLaterSetup(accountID string) (SendLaterSetup, error) {
	return getSendLaterSetup(a.ctx, a.paths.Data, accountID)
}

// InitializeSendLaterClient explicitly creates the installation's keyring key.
func (a *App) InitializeSendLaterClient() (string, error) {
	return initializeSendLaterClient(a.ctx, a.paths.Data)
}

// SaveSendLaterConfig pins the user's verified helper key and account mapping.
func (a *App) SaveSendLaterConfig(config scheduledsend.ClientConfig) error {
	store, err := clientStore(a.paths.Data)
	if err != nil {
		return err
	}
	defer store.Close()
	return store.SaveConfig(a.ctx, config)
}

// ScheduleMessage prepares MIME once and queues an E2EE delivery to the helper.
func (a *App) ScheduleMessage(accountID string, message smtp.ComposeMessage, draftID, due string) (ScheduleResult, error) {
	return scheduleMessage(a.ctx, a.paths.Data, &a.composeOps, a.draftStore, accountID, message, draftID, due)
}

// ListScheduledMessages returns bounded local metadata without loading bodies.
func (a *App) ListScheduledMessages(accountID string) ([]scheduledsend.ClientJob, error) {
	store, err := clientStore(a.paths.Data)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	return store.List(a.ctx, accountID)
}

// UpdateScheduledMessage retries, refreshes, or cancels a stored encrypted job.
func (a *App) UpdateScheduledMessage(id, action string) (ScheduleResult, error) {
	return updateScheduledMessage(a.ctx, a.paths.Data, id, action)
}

// SendMessageWithDraftGuard prevents Send Now from an already scheduled draft.
func (a *App) SendMessageWithDraftGuard(accountID string, message smtp.ComposeMessage, draftID string) error {
	if err := guardScheduledDraft(a.ctx, a.paths.Data, accountID, draftID); err != nil {
		return err
	}
	return a.SendMessage(accountID, message)
}

// GetSendLaterSetup returns public device pairing and account helper settings.
func (c *ComposerApp) GetSendLaterSetup(accountID string) (SendLaterSetup, error) {
	return getSendLaterSetup(c.ctx, c.paths.Data, accountID)
}

// InitializeSendLaterClient explicitly creates the installation's keyring key.
func (c *ComposerApp) InitializeSendLaterClient() (string, error) {
	return initializeSendLaterClient(c.ctx, c.paths.Data)
}

// SaveSendLaterConfig pins the user's verified helper key and account mapping.
func (c *ComposerApp) SaveSendLaterConfig(config scheduledsend.ClientConfig) error {
	store, err := clientStore(c.paths.Data)
	if err != nil {
		return err
	}
	defer store.Close()
	return store.SaveConfig(c.ctx, config)
}

// ScheduleMessage prepares MIME once and queues an E2EE delivery to the helper.
func (c *ComposerApp) ScheduleMessage(accountID string, message smtp.ComposeMessage, draftID, due string) (ScheduleResult, error) {
	return scheduleMessage(c.ctx, c.paths.Data, &c.composeOps, c.draftStore, accountID, message, draftID, due)
}

// ListScheduledMessages returns bounded local metadata without loading bodies.
func (c *ComposerApp) ListScheduledMessages(accountID string) ([]scheduledsend.ClientJob, error) {
	store, err := clientStore(c.paths.Data)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	return store.List(c.ctx, accountID)
}

// UpdateScheduledMessage retries, refreshes, or cancels a stored encrypted job.
func (c *ComposerApp) UpdateScheduledMessage(id, action string) (ScheduleResult, error) {
	return updateScheduledMessage(c.ctx, c.paths.Data, id, action)
}

// SendMessageWithDraftGuard prevents Send Now from an already scheduled draft.
func (c *ComposerApp) SendMessageWithDraftGuard(accountID string, message smtp.ComposeMessage, draftID string) error {
	if err := guardScheduledDraft(c.ctx, c.paths.Data, accountID, draftID); err != nil {
		return err
	}
	return c.SendMessage(accountID, message)
}
