package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/database"
)

func TestValidateMailboxServiceEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "loopback http", input: "http://localhost:8029/", want: mailboxServiceEndpoint},
		{name: "remote https", input: "https://mail.example.test:8029", want: "https://mail.example.test:8029"},
		{name: "remote http rejected", input: "http://mail.example.test:8029", wantErr: true},
		{name: "url credentials rejected", input: "https://user:secret@mail.example.test", wantErr: true},
		{name: "query rejected", input: "https://mail.example.test?token=x", wantErr: true},
		{name: "path rejected", input: "https://mail.example.test/api", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := validateMailboxServiceEndpoint(test.input)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateMailboxServiceEndpoint error = %v, wantErr %v", err, test.wantErr)
			}
			if err == nil && got != test.want {
				t.Fatalf("endpoint = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRemoteMailboxCredentialDoesNotUseLocalGrantDatabase(t *testing.T) {
	encoded, err := json.Marshal(desktopMailboxCredential{Token: "remote-device-token"})
	if err != nil {
		t.Fatal(err)
	}
	connector := &mailboxServiceConnector{
		db:       &database.DB{},
		keyring:  &testMailboxSecretStore{secret: string(encoded)},
		endpoint: "https://mail.example.test:8029",
		capabilitiesFunc: func(context.Context) (*v1.Capabilities, error) {
			return nil, errors.New("remote service unavailable")
		},
	}
	if _, err := connector.getClient(); err != nil {
		t.Fatalf("load remote client credential: %v", err)
	}
	status := connector.status(context.Background())
	if status.ErrorCode != "remote_unavailable" {
		t.Fatalf("status error = %q, want remote_unavailable", status.ErrorCode)
	}
}
