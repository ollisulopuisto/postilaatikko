// Package undo provides an in-memory undo system for email actions
package undo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/hkdb/aerion/internal/logging"
)

// Command represents an undoable action
type Command interface {
	// Execute performs the action (already done, just for interface completeness)
	Execute() error
	// Undo reverses the action
	Undo() error
	// Description returns a human-readable description
	Description() string
	// CreatedAt returns when the command was created
	CreatedAt() time.Time
}

// BaseCommand provides common fields for commands
type BaseCommand struct {
	description string
	createdAt   time.Time
}

// NewBaseCommand creates a new BaseCommand with the given description
func NewBaseCommand(description string) BaseCommand {
	return BaseCommand{
		description: description,
		createdAt:   time.Now(),
	}
}

// Description returns the command description
func (c *BaseCommand) Description() string { return c.description }

// CreatedAt returns when the command was created
func (c *BaseCommand) CreatedAt() time.Time { return c.createdAt }

// Stack is an undo stack with an optional durable backing store. Without a
// store it behaves exactly like the original in-memory stack.
//
// maxSize <= 0 means no count cap: retention is then purely time-based. The
// durable mailbox history uses that mode deliberately - the user asked for a
// 24h (later maybe 7-day) history to observe growth, and a count cap would
// silently truncate that history before the time limit.
type Stack struct {
	mu        sync.Mutex
	commands  []entry
	maxSize   int
	timeout   time.Duration // How long commands remain undoable
	store     *OperationStore
	undoCtx   UndoContext
	lastSweep time.Time
}

type entry struct {
	operationID string
	command     Command
	action      string
	messageIDs  []string
	expiresAt   time.Time
}

// OperationClaim is an atomic claim of one complete user action. Commands are
// ordered exactly as they must be undone (newest first).
type OperationClaim struct {
	OperationID string
	Action      string
	MessageIDs  []string
	entries     []entry
}

// Commands returns the commands in undo execution order.
func (c *OperationClaim) Commands() []Command {
	if c == nil {
		return nil
	}
	commands := make([]Command, 0, len(c.entries))
	for _, item := range c.entries {
		commands = append(commands, item.command)
	}
	return commands
}

// OperationMetadata contains safe identifiers used to trace one undoable
// mutation. Message IDs are local database IDs, never RFC822 headers or mail
// content.
type OperationMetadata struct {
	Action     string
	MessageIDs []string
}

func newOperationID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(bytes)
}

// NewStack creates a new undo stack
func NewStack(maxSize int, timeout time.Duration) *Stack {
	return &Stack{
		commands: make([]entry, 0),
		maxSize:  maxSize,
		timeout:  timeout,
	}
}

// Push adds a command to the stack
func (s *Stack) Push(cmd Command) string {
	return s.PushOperation("", cmd)
}

// PushOperation groups one or more commands created by a single user action.
// The returned token can later undo that action without consuming a newer one.
func (s *Stack) PushOperation(operationID string, cmd Command, metadata ...OperationMetadata) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	created := operationID == ""
	if operationID == "" {
		operationID = newOperationID()
	}
	meta := OperationMetadata{Action: cmd.Description()}
	if len(metadata) > 0 {
		meta = metadata[0]
	}

	// Remove expired commands first
	s.cleanExpired()

	// Add new command
	s.commands = append(s.commands, entry{
		operationID: operationID,
		command:     cmd,
		action:      meta.Action,
		messageIDs:  append([]string(nil), meta.MessageIDs...),
		expiresAt:   cmd.CreatedAt().Add(s.timeout),
	})
	log := logging.WithComponent("undo")
	log.Info().
		Str("operationId", operationID).
		Str("action", meta.Action).
		Strs("messageIds", meta.MessageIDs).
		Bool("created", created).
		Msg("UNDO PUSH")

	// Trim if over max size (a non-positive cap means unbounded)
	if s.maxSize > 0 && len(s.commands) > s.maxSize {
		s.commands = s.commands[len(s.commands)-s.maxSize:]
	}
	s.persistLocked(operationID, meta.Action, meta.MessageIDs, cmd)
	return operationID
}

// AttachStore binds the durable mailbox undo history and immediately
// rehydrates every operation still inside the retention window, so Ctrl+Z
// works across restarts. Expired rows are swept first. A nil store disables
// persistence. Rehydration failures skip only the affected row.
func (s *Stack) AttachStore(store *OperationStore, undoCtx UndoContext) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store = store
	s.undoCtx = undoCtx
	if store == nil {
		return
	}
	log := logging.WithComponent("undo")
	if swept := store.Sweep(time.Now(), s.timeout); swept > 0 {
		log.Info().Int("swept", swept).Msg("UNDO STORE SWEEP")
	}
	rows := store.loadActive(time.Now(), s.timeout, s.maxSize)
	restored := 0
	for _, row := range rows {
		cmd, err := rehydrateCommand(row.Type, row.Payload, context.Background(), undoCtx)
		if err != nil {
			log.Warn().Err(err).Str("operationId", row.OperationID).Msg("UNDO STORE REHYDRATE SKIP")
			continue
		}
		s.commands = append(s.commands, entry{
			operationID: row.OperationID,
			command:     cmd,
			action:      row.Action,
			messageIDs:  append([]string(nil), row.MessageIDs...),
			expiresAt:   row.CreatedAt.Add(s.timeout),
		})
		restored++
	}
	log.Info().Int("restored", restored).Int("stored", store.Count()).Msg("UNDO STORE ATTACHED")
}

// persistLocked persists one freshly pushed command and occasionally sweeps
// expired rows. Caller must hold s.mu.
func (s *Stack) persistLocked(operationID, action string, messageIDs []string, cmd Command) {
	if s.store == nil {
		return
	}
	s.store.persistCommand(operationID, action, messageIDs, cmd)
	if time.Since(s.lastSweep) > 10*time.Minute {
		s.lastSweep = time.Now()
		s.store.Sweep(time.Now(), s.timeout)
	}
}

// PopLatestOperation atomically claims every command belonging to the newest
// user action. Unlike Pop, it preserves the operation ID needed by global Undo
// and handles grouped multi-folder actions as one LIFO item.
func (s *Stack) PopLatestOperation() *OperationClaim {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanExpired()
	if len(s.commands) == 0 {
		log := logging.WithComponent("undo")
		log.Warn().Msg("UNDO POP MISS")
		return nil
	}

	operationID := s.commands[len(s.commands)-1].operationID
	claim := &OperationClaim{OperationID: operationID}
	remaining := make([]entry, 0, len(s.commands))
	for i := len(s.commands) - 1; i >= 0; i-- {
		item := s.commands[i]
		if item.operationID == operationID {
			claim.entries = append(claim.entries, item)
			if claim.Action == "" {
				claim.Action = item.action
				claim.MessageIDs = append([]string(nil), item.messageIDs...)
			}
			continue
		}
		remaining = append(remaining, item)
	}
	for left, right := 0, len(remaining)-1; left < right; left, right = left+1, right-1 {
		remaining[left], remaining[right] = remaining[right], remaining[left]
	}
	s.commands = remaining

	log := logging.WithComponent("undo")
	log.Info().
		Str("operationId", claim.OperationID).
		Str("action", claim.Action).
		Strs("messageIds", claim.MessageIDs).
		Int("commands", len(claim.entries)).
		Msg("UNDO CLAIM")
	if s.store != nil {
		s.store.removeOperation(claim.OperationID)
	}
	return claim
}

// RestoreOperation requeues the failed command and any commands that had not
// yet run. Commands already undone remain consumed. A restored operation gets
// a fresh retry window so an error at the edge of the normal TTL cannot turn
// the next immediate Ctrl+Z into a misleading MISS.
func (s *Stack) RestoreOperation(claim *OperationClaim, failedCommand int) {
	if claim == nil || failedCommand < 0 || failedCommand >= len(claim.entries) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanExpired()
	retryUntil := time.Now().Add(s.timeout)
	for i := len(claim.entries) - 1; i >= failedCommand; i-- {
		item := claim.entries[i]
		item.expiresAt = retryUntil
		s.commands = append(s.commands, item)
		s.persistLocked(item.operationID, item.action, item.messageIDs, item.command)
	}
	if s.maxSize > 0 && len(s.commands) > s.maxSize {
		s.commands = s.commands[len(s.commands)-s.maxSize:]
	}
}

// Pop removes and returns the most recent undoable command
// Returns nil if no commands are available or all are expired
func (s *Stack) Pop() Command {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cleanExpired()

	if len(s.commands) == 0 {
		log := logging.WithComponent("undo")
		log.Warn().Msg("UNDO POP MISS")
		return nil
	}

	item := s.commands[len(s.commands)-1]
	s.commands = s.commands[:len(s.commands)-1]
	log := logging.WithComponent("undo")
	log.Info().
		Str("operationId", item.operationID).
		Str("action", item.action).
		Strs("messageIds", item.messageIDs).
		Msg("UNDO POP")
	if s.store != nil {
		s.store.removeOperation(item.operationID)
	}
	return item.command
}

// PopOperation atomically claims all commands from an operation in reverse
// execution order. It deliberately does not require global LIFO ordering.
func (s *Stack) PopOperation(operationID string) []Command {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanExpired()
	var commands []Command
	var action string
	var messageIDs []string
	remaining := make([]entry, 0, len(s.commands))
	for i := len(s.commands) - 1; i >= 0; i-- {
		item := s.commands[i]
		if item.operationID == operationID {
			commands = append(commands, item.command)
			if action == "" {
				action = item.action
				messageIDs = append([]string(nil), item.messageIDs...)
			}
			continue
		}
		remaining = append(remaining, item)
	}
	for left, right := 0, len(remaining)-1; left < right; left, right = left+1, right-1 {
		remaining[left], remaining[right] = remaining[right], remaining[left]
	}
	s.commands = remaining
	log := logging.WithComponent("undo")
	if len(commands) == 0 {
		log.Warn().Str("operationId", operationID).Msg("UNDO MISS")
		return commands
	}
	log.Info().
		Str("operationId", operationID).
		Str("action", action).
		Strs("messageIds", messageIDs).
		Int("commands", len(commands)).
		Msg("UNDO CLAIM")
	if s.store != nil {
		s.store.removeOperation(operationID)
	}
	return commands
}

// ContainsOperation reports whether an operation is still available without
// claiming it. It is used by mutation diagnostics and integration tests.
func (s *Stack) ContainsOperation(operationID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanExpired()
	for _, item := range s.commands {
		if item.operationID == operationID {
			return true
		}
	}
	return false
}

// DiscardOperation invalidates every command belonging to an action that was
// optimistically accepted locally but could not begin remotely.
func (s *Stack) DiscardOperation(operationID string) int {
	if operationID == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanExpired()
	removed := 0
	remaining := make([]entry, 0, len(s.commands))
	for _, item := range s.commands {
		if item.operationID == operationID {
			removed++
			continue
		}
		remaining = append(remaining, item)
	}
	s.commands = remaining
	if removed > 0 && s.store != nil {
		s.store.removeOperation(operationID)
	}
	return removed
}

// Peek returns the most recent command without removing it
func (s *Stack) Peek() Command {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cleanExpired()

	if len(s.commands) == 0 {
		return nil
	}
	return s.commands[len(s.commands)-1].command
}

// Clear removes all commands from the stack
func (s *Stack) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commands = make([]entry, 0)
	if s.store != nil {
		s.store.removeAll()
	}
}

// cleanExpired removes commands older than timeout (must be called with lock held)
func (s *Stack) cleanExpired() {
	now := time.Now()
	i := 0
	for ; i < len(s.commands); i++ {
		if s.commands[i].expiresAt.After(now) {
			break
		}
	}
	if i > 0 {
		s.commands = s.commands[i:]
	}
}

// CanUndo returns true if there's a command that can be undone
func (s *Stack) CanUndo() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanExpired()
	return len(s.commands) > 0
}

// Size returns the current number of commands in the stack
func (s *Stack) Size() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanExpired()
	return len(s.commands)
}
