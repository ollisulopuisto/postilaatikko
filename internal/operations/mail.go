// Package operations defines explicit, account-qualified targets for mail
// mutations. UI surfaces should resolve these targets before calling the
// existing mutation methods, which retain their undo, sync and event behavior.
package operations

import (
	"errors"
	"fmt"
	"strings"

	"github.com/hkdb/aerion/internal/message"
)

type TargetKind string

const (
	TargetMessage         TargetKind = "message"
	TargetMatchedMessages TargetKind = "matched_messages"
	TargetThread          TargetKind = "thread"
)

// MessageRef identifies one local copy of a message. AccountID is required:
// duplicate copies of the same email in two accounts are different targets.
type MessageRef struct {
	AccountID string `json:"accountId"`
	MessageID string `json:"messageId"`
}

// ThreadRef identifies a whole thread within one account.
type ThreadRef struct {
	AccountID string `json:"accountId"`
	ThreadID  string `json:"threadId"`
}

// Target describes exactly what a surface intended to act on. A matched set
// is the displayed subset of a thread (for example, a virtual-folder result);
// a thread target explicitly opts into every locally known message in it.
type Target struct {
	Kind     TargetKind   `json:"kind"`
	Messages []MessageRef `json:"messages,omitempty"`
	Thread   *ThreadRef   `json:"thread,omitempty"`
}

type MessageLookup interface {
	GetByIDs(ids []string) ([]*message.Message, error)
	GetMessageIDsInThread(accountID, threadID string) ([]string, error)
}

// Resolve validates the requested target against the local message store and
// returns only message IDs. Thread IDs are resolved by the store's thread
// query; they are never forwarded to a message-ID mutation API.
func Resolve(target Target, lookup MessageLookup) ([]string, error) {
	if lookup == nil {
		return nil, errors.New("message lookup is unavailable")
	}
	switch target.Kind {
	case TargetMessage, TargetMatchedMessages:
		if target.Thread != nil {
			return nil, errors.New("message target cannot include a thread reference")
		}
		if len(target.Messages) == 0 {
			return nil, errors.New("message target must include at least one message")
		}
		if target.Kind == TargetMessage && len(target.Messages) != 1 {
			return nil, errors.New("single-message target must include exactly one message")
		}
		ids := make([]string, 0, len(target.Messages))
		seen := make(map[string]struct{}, len(target.Messages))
		for _, ref := range target.Messages {
			if strings.TrimSpace(ref.AccountID) == "" || strings.TrimSpace(ref.MessageID) == "" {
				return nil, errors.New("message reference needs accountId and messageId")
			}
			if _, ok := seen[ref.MessageID]; ok {
				return nil, fmt.Errorf("duplicate message reference %q", ref.MessageID)
			}
			seen[ref.MessageID] = struct{}{}
			ids = append(ids, ref.MessageID)
		}
		messages, err := lookup.GetByIDs(ids)
		if err != nil {
			return nil, fmt.Errorf("validate message target: %w", err)
		}
		if len(messages) != len(target.Messages) {
			return nil, errors.New("one or more target messages no longer exist")
		}
		actualAccounts := make(map[string]string, len(messages))
		for _, item := range messages {
			actualAccounts[item.ID] = item.AccountID
		}
		for _, ref := range target.Messages {
			if actualAccounts[ref.MessageID] != ref.AccountID {
				return nil, fmt.Errorf("message %q does not belong to account %q", ref.MessageID, ref.AccountID)
			}
		}
		return ids, nil
	case TargetThread:
		if len(target.Messages) != 0 {
			return nil, errors.New("thread target cannot include message references")
		}
		if target.Thread == nil || strings.TrimSpace(target.Thread.AccountID) == "" || strings.TrimSpace(target.Thread.ThreadID) == "" {
			return nil, errors.New("thread target needs accountId and threadId")
		}
		ids, err := lookup.GetMessageIDsInThread(target.Thread.AccountID, target.Thread.ThreadID)
		if err != nil {
			return nil, fmt.Errorf("resolve thread target: %w", err)
		}
		if len(ids) == 0 {
			return nil, errors.New("target thread no longer exists")
		}
		return ids, nil
	default:
		return nil, fmt.Errorf("unknown mail target kind %q", target.Kind)
	}
}
