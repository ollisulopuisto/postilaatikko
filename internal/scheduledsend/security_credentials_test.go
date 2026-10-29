package scheduledsend

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/certificate"
	"github.com/hkdb/aerion/internal/credentials"
	"github.com/hkdb/aerion/internal/database"
	keyring "github.com/zalando/go-keyring"
)

func TestSecurityMissingLocalCredentialsRequireAuthorization(t *testing.T) {
	// Use an in-process keyring, never the developer's login keychain.
	keyring.MockInit()
	for _, tc := range []struct {
		name string
		auth account.AuthType
		host string
	}{
		{"SMTP password", account.AuthPassword, "imap.example.com"},
		{"Gmail OAuth", account.AuthOAuth2, "imap.gmail.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			db, err := database.Open(filepath.Join(directory, "aerion.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := db.Close(); err != nil {
					t.Error(err)
				}
			})
			if err := db.Migrate(); err != nil {
				t.Fatal(err)
			}
			accounts := account.NewStore(db)
			acc, err := accounts.Create(&account.AccountConfig{Name: "Helper test", DisplayName: "Sender", Email: "sender@example.com", IMAPHost: tc.host, IMAPPort: 993, IMAPSecurity: account.SecurityTLS, SMTPHost: "smtp.example.com", SMTPPort: 465, SMTPSecurity: account.SecurityTLS, AuthType: tc.auth, Username: "sender@example.com"})
			if err != nil {
				t.Fatal(err)
			}
			creds, err := credentials.NewStore(db.DB, directory)
			if err != nil {
				t.Fatal(err)
			}
			transport := &SMTPTransport{db: db, accounts: accounts, creds: creds, certs: certificate.NewStore(db.DB)}
			delivery := *baselineCommand().Delivery
			delivery.AccountID = acc.ID
			err = transport.Deliver(context.Background(), delivery)
			var failure *DeliveryError
			if !errors.As(err, &failure) || failure.Kind != AuthorizationRequired {
				t.Fatalf("missing credentials must require local authorization, got %v (%+v)", err, failure)
			}
			if !errors.Is(err, credentials.ErrCredentialNotFound) {
				t.Fatalf("local diagnostic cause lost: %v", err)
			}
		})
	}
}
