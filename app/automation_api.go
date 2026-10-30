package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hkdb/aerion/internal/automation"
	"github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/logging"
	"github.com/hkdb/aerion/internal/settings"
)

// AgentAPIClientInfo describes an authorized local API client without exposing
// its bearer credential.
type AgentAPIClientInfo struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
	CreatedAt    string   `json:"createdAt"`
}

// AgentAPIStatus reports whether the opt-in local API is currently listening.
type AgentAPIStatus struct {
	Enabled bool                 `json:"enabled"`
	Address string               `json:"address,omitempty"`
	Clients []AgentAPIClientInfo `json:"clients"`
}

// AgentAPIClientCredential is returned only when a client is created. Token
// is shown once and is not included by AgentAPIStatus or ListAgentAPIClients.
type AgentAPIClientCredential struct {
	Client AgentAPIClientInfo `json:"client"`
	Token  string             `json:"token"`
}

// initAutomationAPI initializes protected client credentials but does not
// bind the listener unless the user explicitly opted in before this launch.
func (a *App) initAutomationAPI(ctx context.Context) {
	log := logging.WithComponent("app.automation")
	registry, err := automation.NewRegistry(a.settingsStore, a.credStore)
	if err != nil {
		log.Error().Err(err).Msg("Failed to initialize local automation client registry")
		return
	}
	a.automationRegistry = registry
	enabled, err := a.settingsStore.Get(settings.KeyAgentAPIEnabled)
	if err != nil {
		log.Warn().Err(err).Msg("Could not read local automation API setting")
		return
	}
	if enabled != "true" {
		return
	}
	a.automationMu.Lock()
	err = a.startAutomationAPILocked(ctx)
	a.automationMu.Unlock()
	if err != nil {
		log.Error().Err(err).Msg("Could not start the opted-in local automation API")
	}
}

// GetAgentAPIStatus returns local listener state and safe authorized-client metadata.
func (a *App) GetAgentAPIStatus() (AgentAPIStatus, error) {
	if a.automationRegistry == nil {
		return AgentAPIStatus{}, errors.New("local automation API is not initialized")
	}
	a.automationMu.Lock()
	server := a.automationServer
	status := AgentAPIStatus{Enabled: server != nil}
	if server != nil {
		status.Address = server.Address()
	}
	a.automationMu.Unlock()
	for _, client := range a.automationRegistry.ListClients() {
		status.Clients = append(status.Clients, agentAPIClientInfo(client))
	}
	return status, nil
}

// SetAgentAPIEnabled opts in to or stops the local-only automation API.
func (a *App) SetAgentAPIEnabled(enabled bool) error {
	if a.settingsStore == nil || a.automationRegistry == nil {
		return errors.New("local automation API is not initialized")
	}
	a.automationMu.Lock()
	defer a.automationMu.Unlock()
	if enabled {
		if a.automationServer != nil {
			return nil
		}
		if err := a.startAutomationAPILocked(a.ctx); err != nil {
			return err
		}
		if err := a.settingsStore.Set(settings.KeyAgentAPIEnabled, "true"); err != nil {
			_ = a.stopAutomationAPILocked()
			return fmt.Errorf("save local automation API preference: %w", err)
		}
		return nil
	}
	if err := a.settingsStore.Set(settings.KeyAgentAPIEnabled, "false"); err != nil {
		return fmt.Errorf("save local automation API preference: %w", err)
	}
	return a.stopAutomationAPILocked()
}

// CreateAgentAPIClient grants specific capabilities and returns the bearer
// token once. The credential is retained only in the protected credential store.
func (a *App) CreateAgentAPIClient(name string, capabilityNames []string) (AgentAPIClientCredential, error) {
	if a.automationRegistry == nil {
		return AgentAPIClientCredential{}, errors.New("local automation API is not initialized")
	}
	capabilities := make([]v1.Capability, 0, len(capabilityNames))
	for _, name := range capabilityNames {
		capabilities = append(capabilities, v1.Capability(name))
	}
	client, token, err := a.automationRegistry.CreateClient(name, capabilities)
	if err != nil {
		return AgentAPIClientCredential{}, err
	}
	return AgentAPIClientCredential{Client: agentAPIClientInfo(client), Token: token}, nil
}

// ListAgentAPIClients returns authorized client metadata, never bearer tokens.
func (a *App) ListAgentAPIClients() ([]AgentAPIClientInfo, error) {
	if a.automationRegistry == nil {
		return nil, errors.New("local automation API is not initialized")
	}
	clients := a.automationRegistry.ListClients()
	result := make([]AgentAPIClientInfo, 0, len(clients))
	for _, client := range clients {
		result = append(result, agentAPIClientInfo(client))
	}
	return result, nil
}

// RevokeAgentAPIClient invalidates a local client's credential and all grants.
func (a *App) RevokeAgentAPIClient(clientID string) error {
	if a.automationRegistry == nil {
		return errors.New("local automation API is not initialized")
	}
	return a.automationRegistry.RevokeClient(clientID)
}

func (a *App) startAutomationAPILocked(ctx context.Context) error {
	if ctx == nil {
		return errors.New("application context is not ready")
	}
	server, err := automation.NewServer(automation.ServerConfig{
		Registry: a.automationRegistry,
		Logger:   logging.WithComponent("app.automation"),
	})
	if err != nil {
		return err
	}
	if err := server.Start(ctx); err != nil {
		return err
	}
	a.automationServer = server
	return nil
}

func (a *App) stopAutomationAPI() error {
	a.automationMu.Lock()
	defer a.automationMu.Unlock()
	return a.stopAutomationAPILocked()
}

func (a *App) stopAutomationAPILocked() error {
	server := a.automationServer
	a.automationServer = nil
	if server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Stop(ctx)
}

func agentAPIClientInfo(client automation.Client) AgentAPIClientInfo {
	capabilities := make([]string, 0, len(client.Capabilities))
	for _, capability := range client.Capabilities {
		capabilities = append(capabilities, string(capability))
	}
	return AgentAPIClientInfo{
		ID:           client.ID,
		Name:         client.Name,
		Capabilities: capabilities,
		CreatedAt:    client.CreatedAt.UTC().Format(time.RFC3339),
	}
}
