// Package mailmove contains safety rules shared by desktop and headless mail
// service adapters.
package mailmove

import "fmt"

// CopyBeforeCleanup guarantees that source cleanup cannot begin until the
// destination adapter has confirmed every copy. A partial or ambiguous copy
// leaves the source untouched so a caller can safely reconcile it later.
func CopyBeforeCleanup(copyAndConfirm func() error, cleanupSource func() error) error {
	if copyAndConfirm == nil || cleanupSource == nil {
		return fmt.Errorf("cross-account move requires copy and cleanup operations")
	}
	if err := copyAndConfirm(); err != nil {
		return fmt.Errorf("destination copy was not confirmed; source preserved: %w", err)
	}
	return cleanupSource()
}
