// Package automation implements the opt-in local API transport and its client
// authorization registry.
package automation

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hkdb/aerion/internal/automation/v1"
)

const (
	registrySetting = "automation_api_clients_v1"
	credentialScope = "automation-api"
	maxClientName   = 80
)

// SettingsStore reads and writes the non-secret client registry metadata.
type SettingsStore interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// SecretStore stores client credentials in the platform credential store.
type SecretStore interface {
	SetExtensionSecret(extension, key, value string) error
	GetExtensionSecret(extension, key string) (string, error)
	DeleteExtensionSecret(extension, key string) error
}

// Client is safe metadata for one explicitly authorized local API client.
// It never contains the client's bearer credential.
type Client struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Capabilities []v1.Capability `json:"capabilities"`
	CreatedAt    time.Time       `json:"createdAt"`
	RevokedAt    *time.Time      `json:"revokedAt,omitempty"`
}

// Registry persists client metadata and protected credentials, and maintains
// the in-memory authorization snapshot used for request checks.
type Registry struct {
	mu       sync.RWMutex
	settings SettingsStore
	secrets  SecretStore
	clients  map[string]registeredClient
}

type registeredClient struct {
	client Client
	digest [sha256.Size]byte
}

type registryFile struct {
	Clients []Client `json:"clients"`
}

// NewRegistry loads client grants and credentials from the app's local stores.
func NewRegistry(settings SettingsStore, secrets SecretStore) (*Registry, error) {
	if settings == nil || secrets == nil {
		return nil, errors.New("automation: settings and credential stores are required")
	}
	r := &Registry{settings: settings, secrets: secrets, clients: make(map[string]registeredClient)}
	raw, err := settings.Get(registrySetting)
	if err != nil {
		return nil, fmt.Errorf("load automation client registry: %w", err)
	}
	if raw == "" {
		return r, nil
	}
	var file registryFile
	if err := json.Unmarshal([]byte(raw), &file); err != nil {
		return nil, fmt.Errorf("decode automation client registry: %w", err)
	}
	for _, client := range file.Clients {
		if client.ID == "" || client.RevokedAt != nil {
			continue
		}
		token, err := secrets.GetExtensionSecret(credentialScope, client.ID)
		if err != nil {
			return nil, fmt.Errorf("load automation client credential %s: %w", client.ID, err)
		}
		if token == "" {
			continue
		}
		r.clients[client.ID] = registeredClient{client: cloneClient(client), digest: sha256.Sum256([]byte(token))}
	}
	return r, nil
}

// CreateClient explicitly authorizes a client and returns its credential once.
func (r *Registry) CreateClient(name string, capabilities []v1.Capability) (Client, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxClientName {
		return Client{}, "", errors.New("automation: client name must contain 1 to 80 characters")
	}
	if len(capabilities) == 0 || len(capabilities) > 16 {
		return Client{}, "", errors.New("automation: grant at least one and at most 16 capabilities")
	}
	unique := make(map[v1.Capability]struct{}, len(capabilities))
	grants := make([]v1.Capability, 0, len(capabilities))
	for _, capability := range capabilities {
		if !capability.Known() {
			return Client{}, "", fmt.Errorf("automation: unknown capability %q", capability)
		}
		if _, exists := unique[capability]; exists {
			continue
		}
		unique[capability] = struct{}{}
		grants = append(grants, capability)
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return Client{}, "", fmt.Errorf("generate automation credential: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)
	client := Client{ID: uuid.NewString(), Name: name, Capabilities: grants, CreatedAt: time.Now().UTC()}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.secrets.SetExtensionSecret(credentialScope, client.ID, token); err != nil {
		return Client{}, "", fmt.Errorf("store automation credential: %w", err)
	}
	r.clients[client.ID] = registeredClient{client: client, digest: sha256.Sum256([]byte(token))}
	if err := r.persistLocked(); err != nil {
		delete(r.clients, client.ID)
		_ = r.secrets.DeleteExtensionSecret(credentialScope, client.ID)
		return Client{}, "", err
	}
	return cloneClient(client), token, nil
}

// ListClients returns authorized client metadata without credentials.
func (r *Registry) ListClients() []Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	clients := make([]Client, 0, len(r.clients))
	for _, registered := range r.clients {
		clients = append(clients, cloneClient(registered.client))
	}
	return clients
}

// RevokeClient immediately removes a grant and invalidates its credential.
func (r *Registry) RevokeClient(clientID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	registered, exists := r.clients[clientID]
	if !exists {
		return fmt.Errorf("automation: client %q not found", clientID)
	}
	delete(r.clients, clientID)
	if err := r.persistLocked(); err != nil {
		r.clients[clientID] = registered
		return err
	}
	if err := r.secrets.DeleteExtensionSecret(credentialScope, clientID); err != nil {
		return fmt.Errorf("revoke automation credential: %w", err)
	}
	return nil
}

// Authenticate validates a bearer token using a constant-time digest check.
func (r *Registry) Authenticate(token string) (Client, bool) {
	if token == "" {
		return Client{}, false
	}
	digest := sha256.Sum256([]byte(token))
	r.mu.RLock()
	defer r.mu.RUnlock()
	var match *Client
	for _, registered := range r.clients {
		if subtle.ConstantTimeCompare(digest[:], registered.digest[:]) == 1 {
			client := cloneClient(registered.client)
			match = &client
		}
	}
	if match == nil {
		return Client{}, false
	}
	return *match, true
}

func (r *Registry) persistLocked() error {
	file := registryFile{Clients: make([]Client, 0, len(r.clients))}
	for _, registered := range r.clients {
		file.Clients = append(file.Clients, cloneClient(registered.client))
	}
	raw, err := json.Marshal(file)
	if err != nil {
		return fmt.Errorf("encode automation client registry: %w", err)
	}
	if err := r.settings.Set(registrySetting, string(raw)); err != nil {
		return fmt.Errorf("persist automation client registry: %w", err)
	}
	return nil
}

func cloneClient(client Client) Client {
	client.Capabilities = append([]v1.Capability(nil), client.Capabilities...)
	if client.RevokedAt != nil {
		revokedAt := *client.RevokedAt
		client.RevokedAt = &revokedAt
	}
	return client
}
