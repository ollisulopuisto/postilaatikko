package automation

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/database"
)

const maxClientNameLength = 80

// SQLiteTokenStore persists API client grants in the mailbox database. It
// never stores bearer tokens in plaintext.
type SQLiteTokenStore struct {
	db *database.DB
}

// NewSQLiteTokenStore creates a TokenStore backed by the provided database.
func NewSQLiteTokenStore(db *database.DB) *SQLiteTokenStore {
	return &SQLiteTokenStore{db: db}
}

// RegisterClient creates a client grant and returns its bearer token once.
func (s *SQLiteTokenStore) RegisterClient(name string, capabilities []v1.Capability) (*ClientGrant, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("automation token store requires a database")
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxClientNameLength {
		return nil, fmt.Errorf("client name must be between 1 and %d bytes", maxClientNameLength)
	}
	validCapabilities := make([]v1.Capability, 0, len(capabilities))
	seen := make(map[v1.Capability]struct{}, len(capabilities))
	for _, capability := range capabilities {
		if !capability.Known() {
			return nil, fmt.Errorf("unknown API capability %q", capability)
		}
		if _, exists := seen[capability]; exists {
			continue
		}
		seen[capability] = struct{}{}
		validCapabilities = append(validCapabilities, capability)
	}
	if len(validCapabilities) == 0 {
		return nil, errors.New("at least one known API capability is required")
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generate API bearer token: %w", err)
	}
	clientIDBytes := make([]byte, 16)
	if _, err := rand.Read(clientIDBytes); err != nil {
		return nil, fmt.Errorf("generate API client ID: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)
	clientID := "client-" + hex.EncodeToString(clientIDBytes)
	tokenDigest := sha256.Sum256([]byte(token))
	capabilityJSON, err := json.Marshal(validCapabilities)
	if err != nil {
		return nil, fmt.Errorf("encode API client capabilities: %w", err)
	}
	createdAt := time.Now().UTC()
	_, err = s.db.Exec(`
		INSERT INTO automation_clients (client_id, name, token_hash, capabilities_json, created_at, revoked)
		VALUES (?, ?, ?, ?, ?, 0)
	`, clientID, name, hex.EncodeToString(tokenDigest[:]), string(capabilityJSON), createdAt.Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("persist API client grant: %w", err)
	}
	return &ClientGrant{
		ClientID:     clientID,
		Name:         name,
		Token:        token,
		Capabilities: validCapabilities,
		CreatedAt:    createdAt,
	}, nil
}

// GetGrant retrieves an active grant using the digest of its bearer token.
func (s *SQLiteTokenStore) GetGrant(token string) (*ClientGrant, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("automation token store requires a database")
	}
	tokenDigest := sha256.Sum256([]byte(token))
	grant, err := scanClientGrant(s.db.QueryRow(`
		SELECT client_id, name, capabilities_json, created_at, revoked
		FROM automation_clients WHERE token_hash = ?
	`, hex.EncodeToString(tokenDigest[:])))
	if err != nil {
		return nil, err
	}
	if grant.Revoked {
		return nil, ErrTokenRevoked
	}
	return grant, nil
}

// RevokeClient revokes a known client. Repeating the operation is safe.
func (s *SQLiteTokenStore) RevokeClient(clientID string) error {
	if s == nil || s.db == nil {
		return errors.New("automation token store requires a database")
	}
	result, err := s.db.Exec(`UPDATE automation_clients SET revoked = 1 WHERE client_id = ?`, clientID)
	if err != nil {
		return fmt.Errorf("revoke API client: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check API client revocation: %w", err)
	}
	if changed > 0 {
		return nil
	}
	var exists int
	if err := s.db.QueryRow(`SELECT 1 FROM automation_clients WHERE client_id = ?`, clientID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrClientNotFound
		}
		return fmt.Errorf("check API client: %w", err)
	}
	return nil
}

// ListClients returns client metadata without bearer tokens.
func (s *SQLiteTokenStore) ListClients() ([]*ClientGrant, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("automation token store requires a database")
	}
	rows, err := s.db.Query(`
		SELECT client_id, name, capabilities_json, created_at, revoked
		FROM automation_clients ORDER BY created_at, client_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list API clients: %w", err)
	}
	defer rows.Close()
	clients := make([]*ClientGrant, 0)
	for rows.Next() {
		grant, err := scanClientGrant(rows)
		if err != nil {
			return nil, err
		}
		clients = append(clients, grant)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read API clients: %w", err)
	}
	return clients, nil
}

type clientGrantScanner interface {
	Scan(dest ...any) error
}

func scanClientGrant(scanner clientGrantScanner) (*ClientGrant, error) {
	var grant ClientGrant
	var capabilityJSON, createdAt string
	var revoked int
	if err := scanner.Scan(&grant.ClientID, &grant.Name, &capabilityJSON, &createdAt, &revoked); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrClientNotFound
		}
		return nil, fmt.Errorf("read API client grant: %w", err)
	}
	if err := json.Unmarshal([]byte(capabilityJSON), &grant.Capabilities); err != nil {
		return nil, fmt.Errorf("decode API client capabilities: %w", err)
	}
	parsedCreatedAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse API client creation time: %w", err)
	}
	grant.CreatedAt = parsedCreatedAt.UTC()
	grant.Revoked = revoked != 0
	return &grant, nil
}
