package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	goSync "sync"
	"time"

	"github.com/google/uuid"
	"github.com/hkdb/aerion/internal/account"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/contact"
	"github.com/hkdb/aerion/internal/draft"
	"github.com/hkdb/aerion/internal/ipc"
	"github.com/hkdb/aerion/internal/logging"
	"github.com/hkdb/aerion/internal/message"
	"github.com/hkdb/aerion/internal/pgp"
	"github.com/hkdb/aerion/internal/platform"
	"github.com/hkdb/aerion/internal/smime"
	"github.com/hkdb/aerion/internal/smtp"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const composerCallType = "composer_call"

// ComposerConfig holds the configuration for a composer window.
type ComposerConfig struct {
	AccountID  string // Required: account to compose from
	IPCAddress string // Required: address of main window's IPC server
	Mode       string // "new", "reply", "reply-all", "forward"
	MessageID  string // Original message ID (for reply/forward)
	DraftID    string // Draft ID to resume editing
	MailtoURL  string // External mailto: URL to pre-fill (detached mode)
}

// ComposeMode represents the compose mode data returned to the frontend.
type ComposeMode struct {
	AccountID string `json:"accountId"`
	Mode      string `json:"mode"`
	MessageID string `json:"messageId"`
	DraftID   string `json:"draftId"`
}

// ComposerApp is a lightweight app struct for detached composer windows.
// It owns only a Wails window and authenticated IPC connection; the parent
// process performs mailbox, credential, contact and cryptographic operations.
type ComposerApp struct {
	ctx    context.Context
	config ComposerConfig

	// Debug mode function reference (injected from main)
	debugMode func() bool

	// IPC client for communication with main window
	ipcClient ipc.Client
	ipcToken  string

	// WindowDecorationStatus is the pre-Wails decision for this composer window.
	WindowDecorationStatus platform.WindowDecorationStatus

	closeMu         goSync.Mutex
	closeAuthorized bool

	// Composer state
	originalMessage           *message.Message     // For reply/forward
	currentDraft              *draft.Draft         // Current draft being edited
	composeMessage            *smtp.ComposeMessage // Prepared compose message
	pendingDraftID            string               // Stable helper idempotency key across ambiguous create retries
	pendingSendID             string               // Stable helper idempotency key across ambiguous delivery retries
	pendingSendBody           *v1.SendRequest      // Immutable prepared MIME retained until send outcome is known
	pendingSendForm           []byte               // Serialized composer fields used to reject changed retries
	pendingSendReceiptChecked bool
}

// NewComposerApp creates a new ComposerApp with the given configuration.
func NewComposerApp(config ComposerConfig, debugModeFn func() bool) *ComposerApp {
	return &ComposerApp{
		config:    config,
		debugMode: debugModeFn,
	}
}

// Startup is called when the composer window starts.
func (c *ComposerApp) Startup(ctx context.Context) {
	c.ctx = ctx

	// Initialize logging - fatal only unless --debug flag is used
	logLevel := "fatal"
	if c.debugMode != nil && c.debugMode() {
		logLevel = "debug"
	}
	_ = logging.Init(logging.Config{
		Level:   logLevel,
		Console: true,
	})
	log := logging.WithComponent("composer")

	log.Info().
		Str("accountID", c.config.AccountID).
		Str("mode", c.config.Mode).
		Str("ipcAddress", c.config.IPCAddress).
		Msg("Composer window starting")

	// Connect to main window's IPC server
	if err := c.connectIPC(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to connect to IPC server")
		// Continue anyway - composer can still work offline
	}

	// Load initial data based on mode
	if err := c.loadInitialData(); err != nil {
		log.Error().Err(err).Msg("Failed to load initial data")
	}

	// Notify main window that we're ready
	c.notifyReady()

	log.Info().Msg("Composer window started successfully")
}

// BeforeClose routes native/window-manager close requests through the
// frontend's existing Save & Close / Discard flow. Returning true cancels the
// current native close; CloseWindow authorizes exactly one subsequent close.
func (c *ComposerApp) BeforeClose(ctx context.Context) bool {
	c.closeMu.Lock()
	if c.closeAuthorized {
		c.closeAuthorized = false
		c.closeMu.Unlock()
		return false
	}
	c.closeMu.Unlock()

	if c.ctx != nil {
		// OnBeforeClose runs on the native close path. Dispatch after returning
		// to Wails so the cancelled close cannot swallow the frontend event.
		go func() {
			wailsRuntime.EventsEmit(c.ctx, "composer:close-requested")
		}()
	}
	return true
}

// Shutdown is called when the composer window is closing.
func (c *ComposerApp) Shutdown(ctx context.Context) {
	log := logging.WithComponent("composer")

	// Notify main window that we're closing
	c.notifyClosed()

	// Close IPC connection
	if c.ipcClient != nil {
		c.ipcClient.Close()
	}

	log.Info().Msg("Composer window shutdown complete")
}

// connectIPC establishes connection to the main window's IPC server.
func (c *ComposerApp) connectIPC(ctx context.Context) error {
	log := logging.WithComponent("composer.ipc")

	// Read token from stdin (passed by parent process)
	tokenBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read token from stdin: %w", err)
	}
	c.ipcToken = strings.TrimSpace(string(tokenBytes))

	if c.ipcToken == "" {
		return fmt.Errorf("no token provided via stdin")
	}

	log.Debug().Str("address", c.config.IPCAddress).Msg("Connecting to IPC server")

	// Create IPC client
	c.ipcClient = ipc.NewClient(c.config.IPCAddress)

	// Register message handler before connecting
	c.ipcClient.OnMessage(c.handleIPCMessage)

	// Connect with token authentication
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := c.ipcClient.Connect(connectCtx, c.ipcToken); err != nil {
		return fmt.Errorf("failed to connect to IPC server: %w", err)
	}

	log.Info().Msg("Connected to IPC server")
	return nil
}

// handleIPCMessage processes messages received from the main window.
func (c *ComposerApp) handleIPCMessage(msg ipc.Message) {
	log := logging.WithComponent("composer.ipc")

	log.Debug().Str("type", msg.Type).Msg("Received IPC message")

	switch msg.Type {
	case ipc.TypePasteRequested:
		wailsRuntime.EventsEmit(c.ctx, "native:paste-requested")

	case ipc.TypeUndoRequested:
		wailsRuntime.EventsEmit(c.ctx, "composer:undo-requested")

	case ipc.TypeThemeChanged:
		var payload ipc.ThemeChangedPayload
		if err := msg.ParsePayload(&payload); err == nil {
			// Emit event to frontend
			wailsRuntime.EventsEmit(c.ctx, "theme:changed", payload.Theme)
		}

	case ipc.TypeShutdown:
		var payload ipc.ShutdownPayload
		_ = msg.ParsePayload(&payload)
		log.Info().Str("reason", payload.Reason).Msg("Received shutdown request from main window")
		// Emit event to frontend to prompt user
		wailsRuntime.EventsEmit(c.ctx, "app:shutdown", payload.Reason)

	default:
		log.Debug().Str("type", msg.Type).Msg("Unknown IPC message type")
	}
}

// callParent invokes one allowlisted desktop operation over the authenticated
// IPC connection. Detached composers never open the mailbox database.
func (c *ComposerApp) callParent(operation string, args ...any) (json.RawMessage, error) {
	if c.ipcClient == nil {
		return nil, fmt.Errorf("desktop service is unavailable")
	}
	encodedArgs, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	request, err := ipc.NewMessage(composerCallType, struct {
		Operation string          `json:"operation"`
		Args      json.RawMessage `json:"args"`
	}{operation, encodedArgs})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(c.ctx, 2*time.Minute)
	defer cancel()
	reply, err := c.ipcClient.SendAndWait(ctx, request)
	if err != nil {
		return nil, err
	}
	if reply.Error != "" {
		return nil, fmt.Errorf("%s", reply.Error)
	}
	return reply.Payload, nil
}

func callParentValue[T any](c *ComposerApp, operation string, args ...any) (T, error) {
	var value T
	data, err := c.callParent(operation, args...)
	if err != nil {
		return value, err
	}
	if len(data) == 0 || string(data) == "null" {
		return value, nil
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, err
	}
	return value, nil
}

// loadInitialData loads the initial data based on compose mode.
func (c *ComposerApp) loadInitialData() error {
	log := logging.WithComponent("composer")

	log.Debug().
		Str("draftID", c.config.DraftID).
		Str("messageID", c.config.MessageID).
		Str("mode", c.config.Mode).
		Msg("Loading initial data")

	// If resuming a draft, load it
	if c.config.DraftID != "" {
		loaded, err := callParentValue[*DraftEditResult](c, "GetComposerDraftForEdit", c.config.AccountID, c.config.DraftID)
		if err != nil {
			log.Error().Err(err).Str("draftID", c.config.DraftID).Msg("Failed to get draft from store")
			return fmt.Errorf("failed to load draft: %w", err)
		}
		if loaded != nil {
			c.currentDraft = &draft.Draft{ID: loaded.DraftID, AccountID: c.config.AccountID, StorageVersion: loaded.Revision}
			c.config.DraftID = loaded.DraftID
			log.Info().
				Str("draftID", c.config.DraftID).
				Str("subject", loaded.Message.Subject).
				Msg("Loaded draft into currentDraft")
		} else {
			log.Warn().Str("draftID", c.config.DraftID).Msg("Draft not found in database")
		}
		return nil
	}

	return nil
}

// notifyReady sends a ready notification to the main window.
func (c *ComposerApp) notifyReady() {
	if c.ipcClient == nil {
		return
	}

	msg, err := ipc.NewMessage(ipc.TypeComposerReady, nil)
	if err != nil {
		return
	}
	_ = c.ipcClient.Send(msg)
}

// notifyClosed sends a closed notification to the main window.
func (c *ComposerApp) notifyClosed() {
	if c.ipcClient == nil {
		return
	}

	var draftID *int64
	if c.currentDraft != nil {
		if id, err := parseIntID(c.currentDraft.ID); err == nil {
			draftID = &id
		}
	}

	msg, err := ipc.NewMessage(ipc.TypeComposerClosed, ipc.ComposerClosedPayload{
		DraftID: draftID,
	})
	if err != nil {
		return
	}
	_ = c.ipcClient.Send(msg)
}

// Wails-bound methods (exposed to frontend)
// ============================================================================

// GetAccount returns the account for the given account ID.
func (c *ComposerApp) GetAccount(accountID string) (*account.Account, error) {
	return callParentValue[*account.Account](c, "GetAccount", accountID)
}

// GetIdentities returns all identities for the given account.
func (c *ComposerApp) GetIdentities(accountID string) ([]*account.Identity, error) {
	return callParentValue[[]*account.Identity](c, "GetIdentities", accountID)
}

// GetAllAccountIdentities returns all enabled accounts with their identities.
// Used by the detached composer to populate the cross-account From dropdown.
func (c *ComposerApp) GetAllAccountIdentities() ([]AccountIdentityGroup, error) {
	return callParentValue[[]AccountIdentityGroup](c, "GetAllAccountIdentities")
}

// GetComposeMode returns the compose mode and related data.
func (c *ComposerApp) GetComposeMode() *ComposeMode {
	return &ComposeMode{
		AccountID: c.config.AccountID,
		Mode:      c.config.Mode,
		MessageID: c.config.MessageID,
		DraftID:   c.config.DraftID,
	}
}

// GetOriginalMessage returns the account-scoped cached message for a composer.
func (a *App) GetOriginalMessage(messageID string) (*message.Message, error) {
	if a == nil || a.messageStore == nil {
		return nil, fmt.Errorf("message service is unavailable")
	}
	return a.messageStore.Get(messageID)
}

// GetComposerOriginalMessage reads account-qualified composer context through
// the helper, with the cached desktop path retained when the helper is absent.
func (a *App) GetComposerOriginalMessage(accountID, messageID string) (*message.Message, error) {
	ctx := a.accountContext()
	client, err := a.composerServiceClient(ctx, v1.CapabilityMailRead)
	if err != nil {
		return nil, err
	}
	return client.GetMessage(ctx, accountID, messageID)
}

// GetShowTitleBar returns whether the custom title bar should be shown.
func (c *ComposerApp) GetShowTitleBar() (bool, error) {
	return callParentValue[bool](c, "GetShowTitleBar")
}

// GetNativeTitleBar returns whether the native OS title bar is enabled.
func (c *ComposerApp) GetNativeTitleBar() (bool, error) {
	return callParentValue[bool](c, "GetNativeTitleBar")
}

// GetWindowDecorationStatus returns the decision used to create this window.
func (c *ComposerApp) GetWindowDecorationStatus() platform.WindowDecorationStatus {
	return c.WindowDecorationStatus
}

// GetThemeMode returns the current theme mode setting.
func (c *ComposerApp) GetThemeMode() (string, error) {
	return callParentValue[string](c, "GetThemeMode")
}

// GetDarkComposerBody returns whether the composer message body should use a
// dark background while in dark mode (the detached composer reads it so it opens
// with the surface matching the user's choice).
func (c *ComposerApp) GetDarkComposerBody() (bool, error) {
	return callParentValue[bool](c, "GetDarkComposerBody")
}

// GetSystemTheme returns the current system theme preference detected via
// the XDG Settings Portal on Linux. Returns "light", "dark", or "" if not available.
func (c *ComposerApp) GetSystemTheme() string {
	return platform.ReadSystemTheme()
}

// IsFlatpak returns true if the application is running inside a Flatpak sandbox.
func (c *ComposerApp) IsFlatpak() bool {
	return platform.IsFlatpak()
}

// NotifyStartupComplete signals the desktop environment that startup is done.
// Called from the frontend after WindowShow() so KDE/Plasma sees the placeholder
// → real window handoff cleanly (avoiding the taskbar-icon flash from #154).
func (c *ComposerApp) NotifyStartupComplete() {
	platform.NotifyStartupComplete()
}

// GetOriginalMessage returns the original message for reply/forward.
func (c *ComposerApp) GetOriginalMessage() (*message.Message, error) {
	if c.originalMessage != nil {
		return c.originalMessage, nil
	}
	if c.config.MessageID == "" {
		return nil, nil
	}
	return callParentValue[*message.Message](c, "GetComposerOriginalMessage", c.config.AccountID, c.config.MessageID)
}

// GetDraft returns the current draft being edited.
func (c *ComposerApp) GetDraft() (*smtp.ComposeMessage, error) {
	if c.currentDraft == nil {
		return nil, nil
	}
	return callParentValue[*smtp.ComposeMessage](c, "GetComposerDraft", c.config.AccountID, c.currentDraft.ID)
}

// PrepareReply builds a ComposeMessage for the current mode.
func (c *ComposerApp) PrepareReply() (*smtp.ComposeMessage, error) {
	if c.config.Mode == "new" || c.config.MessageID == "" {
		// New message - return compose, optionally pre-filled from mailto URL
		acc, err := c.GetAccount(c.config.AccountID)
		if err != nil {
			return nil, err
		}
		identities, err := c.GetIdentities(c.config.AccountID)
		if err != nil {
			return nil, err
		}

		var fromIdentity *account.Identity
		for _, id := range identities {
			if id.IsDefault {
				fromIdentity = id
				break
			}
		}
		if fromIdentity == nil && len(identities) > 0 {
			fromIdentity = identities[0]
		}

		from := smtp.Address{Address: acc.Email, Name: acc.Name}
		if fromIdentity != nil {
			from = smtp.Address{Address: fromIdentity.Email, Name: fromIdentity.Name}
		}

		msg := &smtp.ComposeMessage{
			From: from,
		}

		// Pre-fill from mailto URL if provided
		if c.config.MailtoURL != "" {
			mailtoData := ParseMailtoURL(c.config.MailtoURL)
			if mailtoData != nil {
				for _, addr := range mailtoData.To {
					msg.To = append(msg.To, smtp.Address{Address: addr})
				}
				for _, addr := range mailtoData.Cc {
					msg.Cc = append(msg.Cc, smtp.Address{Address: addr})
				}
				for _, addr := range mailtoData.Bcc {
					msg.Bcc = append(msg.Bcc, smtp.Address{Address: addr})
				}
				msg.Subject = mailtoData.Subject
				msg.TextBody = mailtoData.Body
			}
		}

		return msg, nil
	}

	// For reply/forward, use the same logic as main app
	// This is a simplified version - the full logic is in app.go PrepareReply
	msg, err := callParentValue[*smtp.ComposeMessage](c, "PrepareComposerReply", c.config.AccountID, c.config.MessageID, c.config.Mode)
	if err != nil {
		return nil, err
	}
	c.composeMessage = msg
	return c.composeMessage, nil
}

// SearchContacts searches for contacts matching the query.
func (c *ComposerApp) SearchContacts(query string, limit int) ([]*contact.Contact, error) {
	return callParentValue[[]*contact.Contact](c, "SearchContacts", query, limit)
}

// SendMessage sends the composed email.
func (c *ComposerApp) SendMessage(accountID string, msg smtp.ComposeMessage) error {
	if c.pendingSendID == "" {
		if c.currentDraft != nil && c.currentDraft.StorageVersion != "" {
			key := "postilaatikko-send:" + c.currentDraft.ID + ":" + c.currentDraft.StorageVersion
			c.pendingSendID = uuid.NewSHA1(uuid.NameSpaceURL, []byte(key)).String()
		} else {
			c.pendingSendID = uuid.NewString()
		}
	}
	form, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if c.pendingSendBody != nil && !bytes.Equal(form, c.pendingSendForm) {
		return fmt.Errorf("the previous send outcome is unresolved; retry the unchanged message first")
	}
	if c.pendingSendBody == nil {
		if !c.pendingSendReceiptChecked {
			key := c.pendingSendID
			for attempt := 0; attempt < 100; attempt++ {
				lookup, err := callParentValue[*v1.SendReceiptLookup](c, "GetComposerSendReceipt", key)
				if err != nil {
					return err
				}
				if lookup == nil || !lookup.Found {
					c.pendingSendID = key
					c.pendingSendReceiptChecked = true
					break
				}
				if lookup.Response == nil {
					return fmt.Errorf("helper returned an empty send receipt")
				}
				switch lookup.Response.Receipt.State {
				case v1.OperationSent:
					c.currentDraft = nil
					c.pendingSendID = ""
					c.pendingSendReceiptChecked = false
					if lookup.Response.Code != "" {
						return fmt.Errorf("message was already sent, but helper reported %s", lookup.Response.Code)
					}
					return nil
				case v1.OperationUnknown, v1.OperationSending:
					return fmt.Errorf("a prior send has an unresolved delivery outcome; do not send this draft again")
				case v1.OperationRetry, v1.OperationFailed:
					key = uuid.NewSHA1(uuid.NameSpaceURL, []byte("postilaatikko-send-retry:"+lookup.Response.Receipt.OperationID)).String()
				default:
					return fmt.Errorf("helper returned an invalid send state %q", lookup.Response.Receipt.State)
				}
			}
			if !c.pendingSendReceiptChecked {
				return fmt.Errorf("too many prior send attempts to resolve safely")
			}
		}
		draftID, revision := "", ""
		if c.currentDraft != nil {
			draftID, revision = c.currentDraft.ID, c.currentDraft.StorageVersion
		}
		c.pendingSendBody, err = callParentValue[*v1.SendRequest](c, "PrepareComposerSend", accountID, msg, draftID, revision)
		if err != nil {
			return err
		}
		if c.pendingSendBody == nil {
			return fmt.Errorf("desktop returned no prepared message")
		}
		c.pendingSendForm = append(c.pendingSendForm[:0], form...)
	}
	result, err := callParentValue[*v1.SendResponse](c, "SendPreparedComposerMessage", *c.pendingSendBody, c.pendingSendID)
	if err != nil {
		return err
	}
	if result == nil {
		return fmt.Errorf("desktop returned no send receipt")
	}
	switch result.Receipt.State {
	case v1.OperationSent:
		c.currentDraft = nil
		c.pendingDraftID = ""
		c.pendingSendID = ""
		c.pendingSendBody = nil
		c.pendingSendForm = nil
		c.pendingSendReceiptChecked = false
		if result.Code != "" {
			return fmt.Errorf("message was sent, but helper reported %s", result.Code)
		}
		return nil
	case v1.OperationUnknown:
		return fmt.Errorf("message delivery outcome is unknown; retry with the unchanged message to retrieve the same receipt")
	case v1.OperationRetry, v1.OperationFailed:
		c.pendingSendID = ""
		c.pendingSendBody = nil
		c.pendingSendForm = nil
		c.pendingSendReceiptChecked = false
		if result.Code != "" {
			return fmt.Errorf("message was not sent (%s)", result.Code)
		}
		return fmt.Errorf("message was not sent (%s)", result.Receipt.State)
	default:
		return fmt.Errorf("helper returned an invalid send state %q", result.Receipt.State)
	}
}

// SaveDraft saves the current compose state as a draft.
// If existingDraftID is provided, updates that draft instead of creating a new one.
func (c *ComposerApp) SaveDraft(accountID string, msg smtp.ComposeMessage, existingDraftID string) (*draft.Draft, error) {
	if existingDraftID == "" && c.currentDraft != nil {
		existingDraftID = c.currentDraft.ID
	}
	revision := ""
	if c.currentDraft != nil && c.currentDraft.ID == existingDraftID {
		revision = c.currentDraft.StorageVersion
	}
	if existingDraftID == "" && c.pendingDraftID == "" {
		c.pendingDraftID = uuid.NewString()
	}
	requestID := ""
	if existingDraftID == "" {
		requestID = c.pendingDraftID
	}
	result, err := callParentValue[*DraftResult](c, "SaveComposerDraft", accountID, msg, existingDraftID, revision, requestID)
	if err != nil {
		return nil, err
	}
	if result == nil || result.Draft == nil {
		return nil, fmt.Errorf("desktop returned no saved draft")
	}
	c.currentDraft = result.Draft
	c.config.DraftID = result.Draft.ID
	c.pendingDraftID = ""
	return result.Draft, nil
}

// DeleteDraft deletes a draft from local DB and IMAP.
// If draftID is empty, falls back to c.currentDraft.ID or c.config.DraftID.
func (c *ComposerApp) DeleteDraft(draftID string) error {
	if draftID == "" && c.currentDraft != nil {
		draftID = c.currentDraft.ID
	}
	if draftID == "" && c.config.DraftID != "" {
		draftID = c.config.DraftID
	}

	if draftID == "" {
		return nil
	}
	revision := ""
	if c.currentDraft != nil && c.currentDraft.ID == draftID {
		revision = c.currentDraft.StorageVersion
	}
	if _, err := c.callParent("DeleteComposerDraft", c.config.AccountID, draftID, revision); err != nil {
		return err
	}
	if c.currentDraft != nil && c.currentDraft.ID == draftID {
		c.currentDraft = nil
	}
	return nil
}

// CloseWindow requests the window to close.
func (c *ComposerApp) CloseWindow() {
	c.closeMu.Lock()
	c.closeAuthorized = true
	c.closeMu.Unlock()
	wailsRuntime.Quit(c.ctx)
}

// PickAttachmentFiles opens a file picker dialog and returns the selected files as attachments.
func (c *ComposerApp) PickAttachmentFiles() ([]ComposerAttachment, error) {
	return pickAttachmentFiles(c.ctx)
}

// ReadFileAsAttachment reads a file from a filesystem path and creates a ComposerAttachment.
func (c *ComposerApp) ReadFileAsAttachment(filePath string) (*ComposerAttachment, error) {
	return readFileAsAttachment(filePath)
}

// ============================================================================
// HasSMIMECertificate returns whether the account has a valid default S/MIME certificate.
func (c *ComposerApp) HasSMIMECertificate(accountID string) bool {
	value, _ := callParentValue[bool](c, "HasSMIMECertificate", accountID)
	return value
}

// GetSMIMECertificateForEmail returns the S/MIME certificate matching the given email.
// Returns nil if no matching certificate is found.
func (c *ComposerApp) GetSMIMECertificateForEmail(accountID string, email string) (*smime.Certificate, error) {
	return callParentValue[*smime.Certificate](c, "GetSMIMECertificateForEmail", accountID, email)
}

// GetSMIMESignPolicy returns the signing policy for the account.
func (c *ComposerApp) GetSMIMESignPolicy(accountID string) (string, error) {
	return callParentValue[string](c, "GetSMIMESignPolicy", accountID)
}

// GetSMIMEEncryptPolicy returns the encryption policy for the account.
func (c *ComposerApp) GetSMIMEEncryptPolicy(accountID string) (string, error) {
	return callParentValue[string](c, "GetSMIMEEncryptPolicy", accountID)
}

// CheckRecipientCerts checks which recipients have S/MIME certificates available.
func (c *ComposerApp) CheckRecipientCerts(emails []string) (map[string]bool, error) {
	return callParentValue[map[string]bool](c, "CheckRecipientCerts", emails)
}

// PickRecipientCertFile opens a file picker for certificate files.
func (c *ComposerApp) PickRecipientCertFile() (string, error) {
	path, err := wailsRuntime.OpenFileDialog(c.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select Recipient Certificate",
		Filters: []wailsRuntime.FileFilter{
			{
				DisplayName: "Certificate Files (*.pem, *.cer, *.crt, *.der)",
				Pattern:     "*.pem;*.cer;*.crt;*.der",
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to open file dialog: %w", err)
	}
	return path, nil
}

// ImportRecipientCert imports a recipient's public certificate from a file.
func (c *ComposerApp) ImportRecipientCert(email, filePath string) error {
	if filePath == "" {
		return fmt.Errorf("no file selected")
	}
	if email == "" {
		return fmt.Errorf("email address required")
	}

	_, err := c.callParent("ImportRecipientCert", email, filePath)
	return err
}

// HasPGPKey returns whether the account has a valid default PGP key.
func (c *ComposerApp) HasPGPKey(accountID string) bool {
	value, _ := callParentValue[bool](c, "HasPGPKey", accountID)
	return value
}

// GetPGPKeyForEmail returns the PGP key matching the given email.
// Returns nil if no matching key is found.
func (c *ComposerApp) GetPGPKeyForEmail(accountID string, email string) (*pgp.Key, error) {
	return callParentValue[*pgp.Key](c, "GetPGPKeyForEmail", accountID, email)
}

// GetPGPSignPolicy returns the PGP signing policy for the account.
func (c *ComposerApp) GetPGPSignPolicy(accountID string) (string, error) {
	return callParentValue[string](c, "GetPGPSignPolicy", accountID)
}

// GetPGPEncryptPolicy returns the PGP encryption policy for the account.
func (c *ComposerApp) GetPGPEncryptPolicy(accountID string) (string, error) {
	return callParentValue[string](c, "GetPGPEncryptPolicy", accountID)
}

// CheckRecipientPGPKeys checks which recipients have PGP public keys available.
func (c *ComposerApp) CheckRecipientPGPKeys(emails []string) (map[string]bool, error) {
	return callParentValue[map[string]bool](c, "CheckRecipientPGPKeys", emails)
}

// PickRecipientPGPKeyFile opens a file picker for PGP public key files.
func (c *ComposerApp) PickRecipientPGPKeyFile() (string, error) {
	path, err := wailsRuntime.OpenFileDialog(c.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select Recipient PGP Public Key",
		Filters: []wailsRuntime.FileFilter{
			{
				DisplayName: "PGP Key Files (*.asc, *.gpg, *.key, *.pub)",
				Pattern:     "*.asc;*.gpg;*.key;*.pub",
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to open file dialog: %w", err)
	}
	return path, nil
}

// ImportRecipientPGPKey imports a recipient's PGP public key from a file.
func (c *ComposerApp) ImportRecipientPGPKey(email, filePath string) error {
	if filePath == "" {
		return fmt.Errorf("no file selected")
	}
	if email == "" {
		return fmt.Errorf("email address required")
	}

	_, err := c.callParent("ImportRecipientPGPKey", email, filePath)
	return err
}

// LookupWKD performs a Web Key Directory lookup for the given email address.
func (c *ComposerApp) LookupWKD(email string) (string, error) {
	return callParentValue[string](c, "LookupWKD", email)
}

// LookupHKP performs an HKP key server lookup for the given email address.
func (c *ComposerApp) LookupHKP(email string) (string, error) {
	return callParentValue[string](c, "LookupHKP", email)
}

// LookupPGPKey performs a unified WKD+HKP lookup for the given email address.
func (c *ComposerApp) LookupPGPKey(email string) (string, error) {
	return callParentValue[string](c, "LookupPGPKey", email)
}

// parseIntID parses a string ID to int64.
func parseIntID(id string) (int64, error) {
	var result int64
	_, err := fmt.Sscanf(id, "%d", &result)
	return result, err
}
