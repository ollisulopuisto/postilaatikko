// Package syncowner provides advisory process ownership locks for a local
// mailbox data directory.
package syncowner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrOwned reports that another process currently owns the requested lock.
var ErrOwned = errors.New("mailbox resource is owned by another process")

// Lock is an advisory process lock held until Release or process exit.
type Lock struct{ file *os.File }

// TryAcquire attempts to acquire an exclusive ownership lock without waiting.
func TryAcquire(path string) (*Lock, error) {
	return tryAcquire(path, false)
}

// TryAcquireShared attempts a shared lock that prevents schema migration while
// allowing other database readers and the helper service to remain open.
func TryAcquireShared(path string) (*Lock, error) {
	return tryAcquire(path, true)
}

func tryAcquire(path string, shared bool) (*Lock, error) {
	if path == "" {
		return nil, errors.New("mail ownership lock path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create mail sync lock directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open mail sync lock: %w", err)
	}
	locked, err := tryLockFile(file, shared)
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

// Acquire waits for an exclusive lock until the owner releases it or ctx is canceled.
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

// DowngradeToShared changes an exclusive ownership lock into a shared lock.
func (l *Lock) DowngradeToShared() error {
	if l == nil || l.file == nil {
		return errors.New("ownership lock is not held")
	}
	if err := downgradeLockFile(l.file); err != nil {
		return fmt.Errorf("downgrade ownership lock: %w", err)
	}
	return nil
}
