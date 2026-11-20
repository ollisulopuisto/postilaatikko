package mailfilter

import (
	"context"
	"fmt"
	"reflect"
	"strings"
)

// RuleTarget is an execution owner, not a second copy of a rule.
type RuleTarget string

const (
	TargetLocal  RuleTarget = "local"
	TargetHelper RuleTarget = "helper"
)

// TargetRuleStore is the bounded configuration surface used for verified saves.
type TargetRuleStore interface {
	List(context.Context) ([]*Rule, error)
	Create(context.Context, *Rule) error
	Update(context.Context, *Rule) error
}

// SaveVerifiedRule writes only to the explicitly selected owner and reads back
// its complete rule. Unknown outcomes are returned as errors, never success.
// The preflight comparison detects already-visible edits, not a transactional
// compare-and-swap. Concurrent writes after that read are not prevented.
func SaveVerifiedRule(ctx context.Context, store TargetRuleStore, rule *Rule, expected *Rule) error {
	if store == nil || rule == nil || rule.ID == "" {
		return fmt.Errorf("rule target and rule ID are required")
	}
	before, err := store.List(ctx)
	if err != nil {
		return fmt.Errorf("read rule target before save: %w", err)
	}
	var existing *Rule
	for _, item := range before {
		if item != nil && item.ID == rule.ID {
			existing = item
			break
		}
	}

	// Work on a copy: helper validation normalizes criteria on another host.
	normalized := normalizedRuleConfiguration(rule)
	rule = &normalized
	if expected == nil && rule.OrderIndex == 0 {
		rule.OrderIndex = len(before)
		if existing != nil {
			rule.OrderIndex = existing.OrderIndex
		}
	}
	// A retry after an uncertain write can verify the desired state without
	// writing again. Different existing content remains a hard conflict.
	if existing != nil && SameRuleConfiguration(existing, rule) {
		return nil
	}
	if expected == nil && existing != nil {
		return fmt.Errorf("rule already exists on target; refresh before saving")
	}
	if expected != nil && (existing == nil || !SameRuleConfiguration(existing, expected)) {
		return fmt.Errorf("rule changed on target; refresh before saving")
	}

	if expected == nil {
		err = store.Create(ctx, rule)
	}
	if expected != nil {
		err = store.Update(ctx, rule)
	}
	if err != nil {
		return fmt.Errorf("rule save outcome needs verification: %w", err)
	}
	after, err := store.List(ctx)
	if err != nil {
		return fmt.Errorf("rule saved but read-back failed: %w", err)
	}
	for _, item := range after {
		if item != nil && item.ID == rule.ID && SameRuleConfiguration(item, rule) {
			return nil
		}
	}
	return fmt.Errorf("rule save could not be verified on target")
}

// SameRuleConfiguration compares effective configuration while ignoring server
// timestamps. Slice order remains significant, including rule scope/order.
func SameRuleConfiguration(a, b *Rule) bool {
	if a == nil || b == nil {
		return a == b
	}
	left, right := normalizedRuleConfiguration(a), normalizedRuleConfiguration(b)
	left.CreatedAt, left.UpdatedAt = right.CreatedAt, right.UpdatedAt
	return reflect.DeepEqual(left, right)
}

func normalizedRuleConfiguration(rule *Rule) Rule {
	r := *rule
	r.Name = strings.TrimSpace(r.Name)
	r.FromContains = cleanPatterns(r.FromContains)
	r.ToContains = cleanPatterns(r.ToContains)
	r.SubjectContains = cleanPatterns(r.SubjectContains)
	r.ListIDContains = cleanPatterns(r.ListIDContains)
	for _, values := range []*[]string{&r.AccountIDs, &r.FolderIDs} {
		if len(*values) == 0 {
			*values = nil
		}
	}
	return r
}
