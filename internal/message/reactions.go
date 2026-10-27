package message

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hkdb/aerion/internal/email/reactions"
)

// EnrichReactions loads cached MIME metadata in bounded batches; legacy stored
// bodies can use exact provider notification templates without network fetches.
func (s *Store) EnrichReactions(messages []*Message) error {
	for start := 0; start < len(messages); start += 200 {
		end := min(start+200, len(messages))
		args := []any{}
		byID := map[string]*Message{}
		for _, m := range messages[start:end] {
			if m != nil {
				args = append(args, m.ID)
				byID[m.ID] = m
			}
		}
		if len(args) == 0 {
			continue
		}
		rows, err := s.db.Query("SELECT message_id,metadata FROM message_reactions WHERE message_id IN ("+strings.TrimRight(strings.Repeat("?,", len(args)), ",")+")", args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id, data string
			if err := rows.Scan(&id, &data); err != nil {
				rows.Close()
				return err
			}
			var info reactions.Info
			if json.Unmarshal([]byte(data), &info) != nil {
				info = reactions.Info{Invalid: true, Provider: "cache"}
			}
			byID[id].Reaction = &info
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, m := range byID {
			if m.Reaction != nil || !m.BodyFetched || m.HasSMIME || m.HasPGP || m.HasAttachments {
				continue
			}
			target := m.InReplyTo
			if strings.TrimSpace(target) == "" {
				var refs []string
				if json.Unmarshal([]byte(m.References), &refs) == nil && len(refs) > 0 {
					target = refs[len(refs)-1]
				}
			}
			m.Reaction = reactions.FromNotification(m.FromName, target, m.BodyText, m.BodyHTML)
		}
	}
	return nil
}

func reactionMetadata(info *reactions.Info) (string, error) {
	data, err := json.Marshal(info)
	if err != nil {
		return "", fmt.Errorf("encode reaction: %w", err)
	}
	return string(data), nil
}
