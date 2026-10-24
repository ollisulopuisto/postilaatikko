package settings

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// KeyQuickReplies holds the JSON array of reusable plain-text replies. They are
// local to this installation and are not synced with any account.
const KeyQuickReplies = "quick_replies"

// QuickReply is a named, reusable plain-text reply snippet for the composer.
type QuickReply struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Body string `json:"body"`
}

// ListQuickReplies returns the saved quick replies in creation order. An
// unreadable stored value is logged and treated as empty so the composer keeps
// working.
func (s *Store) ListQuickReplies() ([]QuickReply, error) {
	value, err := s.Get(KeyQuickReplies)
	if err != nil {
		return nil, err
	}
	replies := []QuickReply{}
	if value == "" {
		return replies, nil
	}
	if err := json.Unmarshal([]byte(value), &replies); err != nil {
		s.log.Warn().Err(err).Msg("Failed to parse quick replies, using empty list")
		return []QuickReply{}, nil
	}
	return replies, nil
}

// SaveQuickReply creates a quick reply (empty ID) or updates an existing one
// and returns the stored value. Name and body must not be blank.
func (s *Store) SaveQuickReply(reply QuickReply) (QuickReply, error) {
	reply.Name = strings.TrimSpace(reply.Name)
	if reply.Name == "" {
		return QuickReply{}, fmt.Errorf("quick reply name is required")
	}
	if strings.TrimSpace(reply.Body) == "" {
		return QuickReply{}, fmt.Errorf("quick reply body is required")
	}

	replies, err := s.ListQuickReplies()
	if err != nil {
		return QuickReply{}, err
	}

	if reply.ID == "" {
		reply.ID = uuid.NewString()
		replies = append(replies, reply)
		return reply, s.writeQuickReplies(replies)
	}

	for i := range replies {
		if replies[i].ID != reply.ID {
			continue
		}
		replies[i] = reply
		return reply, s.writeQuickReplies(replies)
	}
	return QuickReply{}, fmt.Errorf("quick reply not found: %s", reply.ID)
}

// DeleteQuickReply removes a quick reply by ID (no-op if absent).
func (s *Store) DeleteQuickReply(id string) error {
	replies, err := s.ListQuickReplies()
	if err != nil {
		return err
	}
	kept := make([]QuickReply, 0, len(replies))
	for _, r := range replies {
		if r.ID != id {
			kept = append(kept, r)
		}
	}
	return s.writeQuickReplies(kept)
}

func (s *Store) writeQuickReplies(replies []QuickReply) error {
	data, err := json.Marshal(replies)
	if err != nil {
		return fmt.Errorf("failed to marshal quick replies: %w", err)
	}
	return s.Set(KeyQuickReplies, string(data))
}
