// Package syncowner coordinates the one process allowed to run mailbox sync
// for a local data directory.
package syncowner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrOwned reports that another process currently owns mailbox sync.
var ErrOwned = errors.New("mail sync is owned by another process")

// Lock is an advisory process lock held until Release or process exit.
type Lock struct{ file *os.File }

// TryAcquire attempts to own mailbox sync without waiting.
func TryAcquire(path string) (*Lock, error) {
	if path == "" {
		return nil, errors.New("mail sync lock path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create mail sync lock directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open mail sync lock: %w", err)
	}
	locked, err := tryLockFile(file)
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("acquire mail sync lock: %w", err)
	}
	if !locked {
		_ = file.Close()
		return nil, ErrOwned
	}
	return &Lock{file: file}, nil
}

// Acquire waits until the owner releases the lock or ctx is canceled.
func Acquire(ctx context.Context, path string) (*Lock, error) {
	if ctx == nil {
		return nil, errors.New("mail sync lock requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		lock, err := TryAcquire(path)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, ErrOwned) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// Release relinquishes the process lock and closes its descriptor.
func (l *Lock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	file := l.file
	l.file = nil
	unlockErr := unlockFile(file)
	closeErr := file.Close()
	if unlockErr != nil {
		return fmt.Errorf("release mail sync lock: %w", unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close mail sync lock: %w", closeErr)
	}
	return nil
}
