package app

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/automation"
	automationclient "github.com/hkdb/aerion/internal/automation/client"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/message"
)

type testMailboxSecretStore struct {
	secret string
	setErr error
}

func (s *testMailboxSecretStore) GetSecret(string) (string, error) {
	if s.secret == "" {
		return "", errors.New("secret missing")
	}
	return s.secret, nil
}

func (s *testMailboxSecretStore) SetSecret(_ string, secret string) error {
	if s.setErr != nil {
		return s.setErr
	}
	s.secret = secret
	return nil
}

func TestAuthorizeCapabilityReplacesCredentialAndRevokesOldGrant(t *testing.T) {
	db := openMailboxMutationTestDB(t)
	tokenStore := automation.NewSQLiteTokenStore(db)
	oldGrant, err := tokenStore.RegisterClient(desktopMailboxClientName, []v1.Capability{v1.CapabilityMailRead, v1.CapabilityMailBodyFetch})
	if err != nil {
		t.Fatal(err)
	}
	secretStore := &testMailboxSecretStore{}
	oldCredential := desktopMailboxCredential{ClientID: oldGrant.ClientID, Token: oldGrant.Token}
	encoded, err := json.Marshal(oldCredential)
	if err != nil {
		t.Fatal(err)
	}
	secretStore.secret = string(encoded)
	connector := &mailboxServiceConnector{
		db: db, keyring: secretStore, credential: &oldCredential,
		client: automationclient.New(automationclient.Config{Endpoint: mailboxServiceEndpoint, Token: oldGrant.Token}),
		capabilitiesFunc: func(context.Context) (*v1.Capabilities, error) {
			return &v1.Capabilities{Version: v1.Version, Capabilities: []v1.Capability{v1.CapabilityMailRead, v1.CapabilityMailBodyFetch}}, nil
		},
	}
	if err := connector.authorizeCapability(context.Background(), v1.CapabilityMailWrite); err != nil {
		t.Fatal(err)
	}
	var stored desktopMailboxCredential
	if err := json.Unmarshal([]byte(secretStore.secret), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.ClientID == oldGrant.ClientID || stored.Token == oldGrant.Token {
		t.Fatalf("stored credential was not replaced: %#v", stored)
	}
	newGrant, err := tokenStore.GetGrant(stored.Token)
	if err != nil {
		t.Fatalf("replacement grant is unavailable: %v", err)
	}
	if !hasCapability(newGrant.Capabilities, v1.CapabilityMailWrite) || !hasCapability(newGrant.Capabilities, v1.CapabilityMailRead) {
		t.Fatalf("replacement grant lost old or requested capability: %v", newGrant.Capabilities)
	}
	if _, err := tokenStore.GetGrant(oldGrant.Token); !errors.Is(err, automation.ErrTokenRevoked) {
		t.Fatalf("old grant was not revoked: %v", err)
	}
}

func TestAuthorizeCapabilityKeepsOldGrantWhenCredentialReplacementFails(t *testing.T) {
	db := openMailboxMutationTestDB(t)
	tokenStore := automation.NewSQLiteTokenStore(db)
	oldGrant, err := tokenStore.RegisterClient(desktopMailboxClientName, []v1.Capability{v1.CapabilityMailRead, v1.CapabilityMailBodyFetch})
	if err != nil {
		t.Fatal(err)
	}
	oldCredential := desktopMailboxCredential{ClientID: oldGrant.ClientID, Token: oldGrant.Token}
	encoded, _ := json.Marshal(oldCredential)
	secretStore := &testMailboxSecretStore{secret: string(encoded), setErr: errors.New("keyring write failed")}
	connector := &mailboxServiceConnector{
		db: db, keyring: secretStore, credential: &oldCredential,
		client: automationclient.New(automationclient.Config{Endpoint: mailboxServiceEndpoint, Token: oldGrant.Token}),
		capabilitiesFunc: func(context.Context) (*v1.Capabilities, error) {
			return &v1.Capabilities{Version: v1.Version, Capabilities: []v1.Capability{v1.CapabilityMailRead, v1.CapabilityMailBodyFetch}}, nil
		},
	}
	if err := connector.authorizeCapability(context.Background(), v1.CapabilityMailWrite); err == nil {
		t.Fatal("authorization succeeded despite keyring replacement failure")
	}
	if _, err := tokenStore.GetGrant(oldGrant.Token); err != nil {
		t.Fatalf("old grant was revoked after replacement failure: %v", err)
	}
}

func TestFlagMutationFallsBackOnlyWhenMailboxServiceIsUnavailable(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    MailboxServiceStatus
		wantRead  bool
		wantError bool
	}{
		{name: "unavailable helper uses local fallback", status: MailboxServiceStatus{ErrorCode: "service_unavailable"}, wantRead: true},
		{name: "authorization failure does not write locally", status: MailboxServiceStatus{ErrorCode: "authorization_required", Message: "authorization required"}, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openMailboxMutationTestDB(t)
			for _, statement := range []string{
				`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username,enabled) VALUES ('a','A','a@example.com','imap','smtp','a',1)`,
				`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES ('f','a','Inbox','INBOX','inbox')`,
			} {
				if _, err := db.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			messages := message.NewStore(db)
			if err := messages.Create(&message.Message{ID: "m", AccountID: "a", FolderID: "f", UID: 1, ThreadID: "m", Date: time.Now()}); err != nil {
				t.Fatal(err)
			}
			connector := &mailboxServiceConnector{
				statusFunc: func(context.Context) MailboxServiceStatus { return tc.status },
				client:     automationclient.New(automationclient.Config{Endpoint: mailboxServiceEndpoint, Token: "test"}),
			}
			a := &App{ctx: context.Background(), messageStore: messages, folderStore: folder.NewStore(db), mailboxService: connector, runtimeEventEmitter: func(string, ...interface{}) {}}
			err := a.MarkAsRead([]string{"m"})
			if (err != nil) != tc.wantError {
				t.Fatalf("MarkAsRead error = %v, wantError=%t", err, tc.wantError)
			}
			updated, err := messages.Get("m")
			if err != nil {
				t.Fatal(err)
			}
			if updated.IsRead != tc.wantRead {
				t.Fatalf("local read flag = %t, want %t", updated.IsRead, tc.wantRead)
			}
		})
	}
}

func openMailboxMutationTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "mailbox-mutation.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestMutateActionReusesKeyAfterAmbiguousFailure(t *testing.T) {
	if mailboxMutationTimeout < 30*time.Second || mailboxAPIRequestTimeout != 2*time.Second {
		t.Fatalf("mailbox request timeouts = api %s/mutation %s, want api 2s/mutation at least 30s", mailboxAPIRequestTimeout, mailboxMutationTimeout)
	}

	connector := &mailboxServiceConnector{client: automationclient.New(automationclient.Config{Endpoint: mailboxServiceEndpoint, Token: "test"})}
	keys := []string{}
	connector.mutateActionFunc = func(_ context.Context, _ v1.ActionRequest, key string) (*v1.ActionResponse, error) {
		keys = append(keys, key)
		if len(keys) == 1 {
			return nil, context.DeadlineExceeded
		}
		if len(keys) == 2 {
			return nil, errors.New("api error (idempotency_conflict): Matching action is still in progress")
		}
		return &v1.ActionResponse{}, nil
	}
	request := v1.ActionRequest{Action: v1.ActionStar}
	for index := 0; index < 3; index++ {
		_, _ = connector.mutateAction(context.Background(), request)
	}
	if len(keys) != 3 || keys[0] == "" || keys[0] != keys[1] || keys[1] != keys[2] {
		t.Fatalf("idempotency keys after timeout/retry = %v, want the same nonempty key", keys)
	}
	if _, err := connector.mutateAction(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 4 || keys[3] == keys[2] {
		t.Fatalf("new action after confirmed success reused old key: %v", keys)
	}
}

func TestMarkAsReadReconcilesProviderConfirmedPartialSuccess(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "partial-read.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username,enabled) VALUES ('a','A','a@example.com','imap','smtp','a',1)`,
		`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES ('f','a','Inbox','INBOX','inbox')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	messages := message.NewStore(db)
	for index, id := range []string{"m1", "m2"} {
		if err := messages.Create(&message.Message{ID: id, AccountID: "a", FolderID: "f", UID: uint32(index + 1), ThreadID: id, Date: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	connector := &mailboxServiceConnector{
		client: automationclient.New(automationclient.Config{Endpoint: mailboxServiceEndpoint, Token: "test"}),
		statusFunc: func(context.Context) MailboxServiceStatus {
			return MailboxServiceStatus{Connected: true, WriteAuthorized: true}
		},
		mutateActionFunc: func(context.Context, v1.ActionRequest, string) (*v1.ActionResponse, error) {
			read := true
			if err := messages.UpdateFlagsBatch([]string{"m1"}, &read, nil); err != nil {
				return nil, err
			}
			return nil, errors.New("second account provider failed")
		},
	}
	type emitted struct {
		name    string
		payload any
	}
	events := make(chan emitted, 8)
	a := &App{
		ctx:            context.Background(),
		messageStore:   messages,
		folderStore:    folder.NewStore(db),
		mailboxService: connector,
		runtimeEventEmitter: func(name string, args ...interface{}) {
			var payload any
			if len(args) > 0 {
				payload = args[0]
			}
			events <- emitted{name: name, payload: payload}
		},
	}
	if err := a.MarkAsRead([]string{"m1", "m2"}); err == nil {
		t.Fatal("MarkAsRead hid the partial provider failure")
	}
	var readChanged bool
	for len(events) > 0 {
		item := <-events
		if item.name != "messages:readChanged" {
			continue
		}
		payload, err := json.Marshal(item.payload)
		if err != nil {
			t.Fatal(err)
		}
		var data struct {
			MessageIDs []string `json:"messageIds"`
			IsRead     bool     `json:"isRead"`
		}
		if err := json.Unmarshal(payload, &data); err != nil {
			t.Fatal(err)
		}
		if len(data.MessageIDs) == 1 && data.MessageIDs[0] == "m1" && data.IsRead {
			readChanged = true
		}
	}
	if !readChanged {
		t.Fatal("confirmed partial read was not emitted to the desktop")
	}
	updated, err := messages.GetByIDs([]string{"m1", "m2"})
	if err != nil {
		t.Fatal(err)
	}
	readByID := map[string]bool{}
	for _, item := range updated {
		readByID[item.ID] = item.IsRead
	}
	if !readByID["m1"] || readByID["m2"] {
		t.Fatalf("provider-confirmed local flags were not retained: %v", readByID)
	}
}

func TestStarReconcilesProviderConfirmedPartialSuccess(t *testing.T) {
	db := openMailboxMutationTestDB(t)
	for _, statement := range []string{
		`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username,enabled) VALUES ('a','A','a@example.com','imap','smtp','a',1)`,
		`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES ('f','a','Inbox','INBOX','inbox')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	messages := message.NewStore(db)
	for index, id := range []string{"m1", "m2"} {
		if err := messages.Create(&message.Message{ID: id, AccountID: "a", FolderID: "f", UID: uint32(index + 1), ThreadID: id, Date: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	connector := &mailboxServiceConnector{
		client: automationclient.New(automationclient.Config{Endpoint: mailboxServiceEndpoint, Token: "test"}),
		statusFunc: func(context.Context) MailboxServiceStatus {
			return MailboxServiceStatus{Connected: true, WriteAuthorized: true}
		},
		mutateActionFunc: func(context.Context, v1.ActionRequest, string) (*v1.ActionResponse, error) {
			starred := true
			if err := messages.UpdateFlagsBatch([]string{"m1"}, nil, &starred); err != nil {
				return nil, err
			}
			return nil, errors.New("second account provider failed")
		},
	}
	events := make(chan map[string]any, 8)
	a := &App{
		ctx:            context.Background(),
		messageStore:   messages,
		folderStore:    folder.NewStore(db),
		mailboxService: connector,
		runtimeEventEmitter: func(name string, args ...interface{}) {
			if name != "messages:starredChanged" || len(args) == 0 {
				return
			}
			payload, _ := json.Marshal(args[0])
			var value map[string]any
			_ = json.Unmarshal(payload, &value)
			events <- value
		},
	}
	if err := a.Star([]string{"m1", "m2"}); err == nil {
		t.Fatal("Star hid the partial provider failure")
	}
	for len(events) > 0 {
		payload := <-events
		ids, _ := payload["messageIds"].([]any)
		if len(ids) == 1 && ids[0] == "m1" && payload["isStarred"] == true {
			return
		}
	}
	t.Fatal("confirmed partial star was not emitted to the desktop")
}
