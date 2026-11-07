package alwayson

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/logging"
	"google.golang.org/api/option"
)

var cloudResourcePart = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)

// GmailPushConfig enables Gmail watch registration and Pub/Sub pull delivery.
// The topic and subscription are provisioned by the operator.
type GmailPushConfig struct {
	ProjectID       string
	Topic           string
	Subscription    string
	CredentialsFile string
}

// Validate ensures the configured resources are fully qualified and belong to
// the selected Cloud project.
func (c GmailPushConfig) Validate() error {
	if c.ProjectID == "" || c.Topic == "" || c.Subscription == "" {
		return errors.New("Gmail Pub/Sub requires a project, topic, and subscription")
	}
	if !cloudResourcePart.MatchString(c.ProjectID) {
		return errors.New("Gmail Pub/Sub project ID is invalid")
	}
	if c.CredentialsFile != "" && !filepath.IsAbs(c.CredentialsFile) {
		return errors.New("Gmail Pub/Sub credentials path must be absolute")
	}
	topic := strings.Split(c.Topic, "/")
	if len(topic) != 4 || topic[0] != "projects" || topic[1] != c.ProjectID || topic[2] != "topics" || !cloudResourcePart.MatchString(topic[3]) {
		return errors.New("Gmail Pub/Sub topic must be projects/{project}/topics/{topic} in the configured project")
	}
	subscription := strings.Split(c.Subscription, "/")
	if len(subscription) != 4 || subscription[0] != "projects" || subscription[1] != c.ProjectID || subscription[2] != "subscriptions" || !cloudResourcePart.MatchString(subscription[3]) {
		return errors.New("Gmail Pub/Sub subscription must be projects/{project}/subscriptions/{subscription} in the configured project")
	}
	return nil
}

type gmailPushNotification struct {
	EmailAddress string `json:"emailAddress"`
	HistoryID    string `json:"historyId"`
}

func (r *SyncRuntime) runGmailPush(ctx context.Context, config GmailPushConfig) {
	log := logging.WithComponent("alwayson.gmail-push")
	var options []option.ClientOption
	if config.CredentialsFile != "" {
		options = append(options, option.WithAuthCredentialsFile(option.ServiceAccount, config.CredentialsFile))
	}
	var client *pubsub.Client
	backoff := time.Second
	for ctx.Err() == nil && client == nil {
		var err error
		client, err = pubsub.NewClient(ctx, config.ProjectID, options...)
		if err != nil {
			log.Error().Err(err).Msg("Gmail Pub/Sub client could not start; periodic sync remains active")
			if !sleepContext(ctx, backoff) {
				return
			}
			backoff *= 2
			if backoff > 5*time.Minute {
				backoff = 5 * time.Minute
			}
		}
	}
	if client == nil {
		return
	}
	defer func() {
		if err := client.Close(); err != nil {
			log.Warn().Err(err).Msg("Failed to close Gmail Pub/Sub client")
		}
	}()

	subscription := client.Subscriber(config.Subscription)
	subscription.ReceiveSettings.MaxOutstandingMessages = 1
	subscription.ReceiveSettings.MaxOutstandingBytes = 1 << 20
	subscription.ReceiveSettings.MaxExtension = 15 * time.Minute

	renewContext, cancelRenewal := context.WithCancel(ctx)
	renewDone := make(chan struct{})
	go func() {
		defer close(renewDone)
		r.renewGmailWatches(renewContext, config.Topic)
	}()
	defer func() {
		cancelRenewal()
		<-renewDone
	}()

	backoff = time.Second
	for ctx.Err() == nil {
		err := subscription.Receive(ctx, func(messageContext context.Context, message *pubsub.Message) {
			r.handleGmailPush(messageContext, message)
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Error().Err(err).Msg("Gmail Pub/Sub receive failed; retrying while periodic sync remains active")
		}
		if !sleepContext(ctx, backoff) {
			return
		}
		backoff *= 2
		if backoff > 5*time.Minute {
			backoff = 5 * time.Minute
		}
	}
}

func (r *SyncRuntime) renewGmailWatches(ctx context.Context, topic string) {
	log := logging.WithComponent("alwayson.gmail-watch")
	nextRenewal := map[string]time.Time{}
	retryDelay := map[string]time.Duration{}
	for ctx.Err() == nil {
		err := r.registerGmailWatches(ctx, topic, nextRenewal, retryDelay)
		if err != nil {
			log.Error().Err(err).Msg("Some Gmail watches could not be renewed; periodic sync remains active")
		}
		now := time.Now()
		nextCheck := now.Add(10 * time.Minute)
		for _, renewalAt := range nextRenewal {
			if renewalAt.Before(nextCheck) {
				nextCheck = renewalAt
			}
		}
		if !sleepContext(ctx, time.Until(nextCheck)) {
			return
		}
	}
}

func (r *SyncRuntime) registerGmailWatches(ctx context.Context, topic string, nextRenewal map[string]time.Time, retryDelay map[string]time.Duration) error {
	log := logging.WithComponent("alwayson.gmail-watch")
	accounts, err := r.accounts.List()
	if err != nil {
		return fmt.Errorf("list accounts for Gmail watch registration: %w", err)
	}
	now := time.Now()
	var failures []error
	for _, item := range accounts {
		if item == nil {
			continue
		}
		if !item.Enabled {
			delete(nextRenewal, item.ID)
			delete(retryDelay, item.ID)
			continue
		}
		if renewalAt := nextRenewal[item.ID]; renewalAt.After(now) {
			continue
		}
		client, err := r.syncEngine.GmailClient(ctx, item.ID)
		if errors.Is(err, gmail.ErrNotCapable) {
			nextRenewal[item.ID] = now.Add(24 * time.Hour)
			continue
		}
		if err == nil {
			var watch *gmail.WatchResponse
			watch, err = client.Watch(ctx, topic)
			if err == nil {
				next := now.Add(24 * time.Hour)
				expirySafetyWindow := watch.ExpiresAt.Add(-24 * time.Hour)
				if expirySafetyWindow.Before(next) {
					next = expirySafetyWindow
				}
				if !next.After(now) {
					next = now.Add(5 * time.Minute)
				}
				nextRenewal[item.ID] = next
			}
		}
		if err != nil {
			log.Error().Err(err).Str("account_id", item.ID).Msg("Gmail watch registration failed")
			delay := retryDelay[item.ID]
			if delay == 0 {
				delay = time.Minute
			}
			nextRenewal[item.ID] = now.Add(delay)
			retryDelay[item.ID] = delay * 2
			if retryDelay[item.ID] > time.Hour {
				retryDelay[item.ID] = time.Hour
			}
			failures = append(failures, fmt.Errorf("account %s: %w", item.ID, err))
			continue
		}
		if nextRenewal[item.ID].IsZero() {
			nextRenewal[item.ID] = now.Add(24 * time.Hour)
		}
		delete(retryDelay, item.ID)
	}
	return errors.Join(failures...)
}

func (r *SyncRuntime) handleGmailPush(ctx context.Context, message *pubsub.Message) {
	log := logging.WithComponent("alwayson.gmail-push")
	var notification gmailPushNotification
	if err := json.Unmarshal(message.Data, &notification); err != nil || strings.TrimSpace(notification.EmailAddress) == "" || strings.TrimSpace(notification.HistoryID) == "" {
		log.Warn().Msg("Ignoring malformed Gmail Pub/Sub notification")
		message.Ack()
		return
	}
	accounts, err := r.accounts.List()
	if err != nil {
		log.Error().Err(err).Msg("Could not map Gmail notification to a configured account")
		message.Nack()
		return
	}
	var matched *account.Account
	for _, item := range accounts {
		if item == nil || !item.Enabled || item.AuthType != account.AuthOAuth2 || !strings.EqualFold(strings.TrimSpace(item.IMAPHost), "imap.gmail.com") || !strings.EqualFold(strings.TrimSpace(item.Email), strings.TrimSpace(notification.EmailAddress)) {
			continue
		}
		if matched != nil {
			log.Warn().Msg("Ignoring Gmail notification with ambiguous account identity")
			message.Ack()
			return
		}
		matched = item
	}
	if matched == nil {
		log.Warn().Msg("Ignoring Gmail notification for an unconfigured account")
		message.Ack()
		return
	}
	if _, err := r.syncEngine.GmailClient(ctx, matched.ID); errors.Is(err, gmail.ErrNotCapable) {
		log.Warn().Str("account_id", matched.ID).Msg("Ignoring Gmail notification for a non-Gmail account")
		message.Ack()
		return
	} else if err != nil {
		log.Error().Err(err).Str("account_id", matched.ID).Msg("Could not prepare Gmail notification sync")
		message.Nack()
		return
	}
	if _, err := r.syncEngine.SyncGmailAccount(ctx, matched.ID); err != nil {
		log.Error().Err(err).Str("account_id", matched.ID).Msg("Gmail notification sync failed; notification will be retried")
		message.Nack()
		return
	}
	message.Ack()
}

func sleepContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
