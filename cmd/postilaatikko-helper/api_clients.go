package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hkdb/aerion/internal/automation"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/database"
)

type capabilityFlags []string

func (f *capabilityFlags) String() string {
	return strings.Join(*f, ",")
}

func (f *capabilityFlags) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func runAPIClientCommand(args []string, defaultData string) (result error) {
	if len(args) == 0 {
		return errors.New("usage: postilaatikko-helper api-client create|list|revoke [flags]")
	}
	action := args[0]
	if action != "create" && action != "list" && action != "revoke" {
		return fmt.Errorf("unknown api-client action %q", action)
	}

	flags := flag.NewFlagSet("api-client "+action, flag.ContinueOnError)
	dataDirectory := flags.String("data", defaultData, "Postilaatikko account data directory")
	name := flags.String("name", "", "display name for a new API client")
	clientID := flags.String("client", "", "client ID to revoke")
	var capabilities capabilityFlags
	flags.Var(&capabilities, "capability", "API capability to grant (repeatable; includes account.manage)")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if action == "create" && *name == "" {
		return errors.New("api-client create requires --name")
	}
	if action == "revoke" && *clientID == "" {
		return errors.New("api-client revoke requires --client")
	}
	if err := prepareExistingServiceDatabase(*dataDirectory, true); err != nil {
		return fmt.Errorf("prepare account database: %w", err)
	}

	databasePath := filepath.Join(*dataDirectory, "aerion.db")
	info, err := os.Stat(databasePath)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("configure mail accounts in Postilaatikko before managing API clients")
	}
	db, err := database.Open(databasePath)
	if err != nil {
		return fmt.Errorf("open account database: %w", err)
	}
	var schemaOwnerRelease func() error
	defer func() {
		if err := db.Close(); err != nil {
			result = errors.Join(result, fmt.Errorf("close account database: %w", err))
		}
		if schemaOwnerRelease != nil {
			result = errors.Join(result, schemaOwnerRelease())
		}
	}()
	owner, err := acquireSchemaOwnership(db, *dataDirectory)
	if err != nil {
		return fmt.Errorf("prepare account database: %w", err)
	}
	schemaOwnerRelease = owner.Release
	store := automation.NewSQLiteTokenStore(db)

	switch action {
	case "create":
		parsedCapabilities := make([]v1.Capability, 0, len(capabilities))
		for _, rawCapability := range capabilities {
			capability := v1.Capability(rawCapability)
			if !capability.Known() {
				return fmt.Errorf("unknown API capability %q", rawCapability)
			}
			parsedCapabilities = append(parsedCapabilities, capability)
		}
		grant, err := store.RegisterClient(*name, parsedCapabilities)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(struct {
			ClientID     string          `json:"clientId"`
			Name         string          `json:"name"`
			Capabilities []v1.Capability `json:"capabilities"`
			CreatedAt    string          `json:"createdAt"`
			BearerToken  string          `json:"bearerToken"`
		}{
			ClientID:     grant.ClientID,
			Name:         grant.Name,
			Capabilities: grant.Capabilities,
			CreatedAt:    grant.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			BearerToken:  grant.Token,
		})
	case "list":
		grants, err := store.ListClients()
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(grants)
	case "revoke":
		if err := store.RevokeClient(*clientID); err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(struct {
			ClientID string `json:"clientId"`
			Revoked  bool   `json:"revoked"`
		}{ClientID: *clientID, Revoked: true})
	default:
		return errors.New("unreachable API client action")
	}
}
