package settings

import (
	"encoding/json"
	"fmt"
	"net/mail"
	"strings"

	"github.com/google/uuid"
)

const KeySmartNotifications = "smart_notifications"

// NotificationRule can notify or mute mail matching one sender or domain.
type NotificationRule struct {
	ID     string `json:"id"`
	Scope  string `json:"scope"` // sender | domain
	Match  string `json:"match"`
	Action string `json:"action"` // notify | mute
}

// SmartNotificationSettings contains local notification controls.
type SmartNotificationSettings struct {
	Enabled bool               `json:"enabled"`
	Rules   []NotificationRule `json:"rules"`
}

// GetSmartNotificationSettings returns local notification settings. Alerts
// remain enabled by default to preserve the app's existing behavior.
func (s *Store) GetSmartNotificationSettings() (SmartNotificationSettings, error) {
	value, err := s.Get(KeySmartNotifications)
	if err != nil {
		return SmartNotificationSettings{}, err
	}
	if value == "" {
		return SmartNotificationSettings{Enabled: true, Rules: []NotificationRule{}}, nil
	}
	var prefs SmartNotificationSettings
	if err := json.Unmarshal([]byte(value), &prefs); err != nil {
		s.log.Warn().Err(err).Msg("Failed to parse smart notification settings; using defaults")
		return SmartNotificationSettings{Enabled: true, Rules: []NotificationRule{}}, nil
	}
	if prefs.Rules == nil {
		prefs.Rules = []NotificationRule{}
	}
	return prefs, nil
}

// SaveSmartNotificationSettings validates and stores local notification controls.
func (s *Store) SaveSmartNotificationSettings(prefs SmartNotificationSettings) error {
	seen := make(map[string]struct{}, len(prefs.Rules))
	for i := range prefs.Rules {
		rule := &prefs.Rules[i]
		if rule.Scope != "sender" && rule.Scope != "domain" {
			return fmt.Errorf("notification rule scope must be sender or domain")
		}
		if rule.Action != "notify" && rule.Action != "mute" {
			return fmt.Errorf("notification rule action must be notify or mute")
		}
		normalized, err := normalizeNotificationMatch(rule.Scope, rule.Match)
		if err != nil {
			return err
		}
		rule.Match = normalized
		key := rule.Scope + ":" + normalized
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate notification rule for %s", normalized)
		}
		seen[key] = struct{}{}
		if rule.ID == "" {
			rule.ID = uuid.NewString()
		}
	}
	data, err := json.Marshal(prefs)
	if err != nil {
		return fmt.Errorf("failed to marshal smart notification settings: %w", err)
	}
	return s.Set(KeySmartNotifications, string(data))
}

func normalizeNotificationMatch(scope, value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if scope == "sender" {
		address, err := mail.ParseAddress(value)
		if err != nil || address.Address != value || strings.Count(value, "@") != 1 {
			return "", fmt.Errorf("notification sender must be an email address")
		}
		return value, nil
	}
	value = strings.TrimPrefix(value, "@")
	address, err := mail.ParseAddress("postilaatikko@" + value)
	if err != nil || address.Address != "postilaatikko@"+value || strings.ContainsAny(value, "@/ ") {
		return "", fmt.Errorf("notification domain must be a domain name")
	}
	return value, nil
}

// ShouldNotify reports whether a message from sender should produce an alert.
// A sender-specific rule takes precedence over a domain rule. Unmatched senders
// remain eligible so adding a rule never silences unrelated mail.
func (prefs SmartNotificationSettings) ShouldNotify(sender string) bool {
	if !prefs.Enabled {
		return false
	}
	address, err := mail.ParseAddress(strings.TrimSpace(sender))
	if err != nil || address.Address == "" {
		return true
	}
	email := strings.ToLower(address.Address)
	domain := email[strings.LastIndexByte(email, '@')+1:]
	var domainAction string
	for _, rule := range prefs.Rules {
		if rule.Scope == "sender" && strings.EqualFold(rule.Match, email) {
			return rule.Action != "mute"
		}
		if rule.Scope == "domain" && strings.EqualFold(rule.Match, domain) {
			domainAction = rule.Action
		}
	}
	return domainAction != "mute"
}
