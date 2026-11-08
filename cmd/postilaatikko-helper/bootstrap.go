package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/servicebootstrap"
	"github.com/hkdb/aerion/internal/syncowner"
)

func runBootstrap(dataDirectory string) error {
	status, err := ensureServiceDatabase(dataDirectory, true)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(status)
}

func prepareExistingServiceDatabase(dataDirectory string, manageService bool) error {
	info, err := os.Stat(filepath.Join(dataDirectory, "aerion.db"))
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("configure mail accounts in Postilaatikko before starting the local mail service")
	}
	_, err = ensureServiceDatabase(dataDirectory, manageService)
	return err
}

func ensureServiceDatabase(dataDirectory string, manageService bool) (status servicebootstrap.Status, resultErr error) {
	if strings.TrimSpace(dataDirectory) == "" {
		return servicebootstrap.Status{}, errors.New("bootstrap requires a data directory")
	}
	if err := os.MkdirAll(dataDirectory, 0700); err != nil {
		return servicebootstrap.Status{}, fmt.Errorf("create mailbox data directory: %w", err)
	}
	db, err := database.Open(filepath.Join(dataDirectory, "aerion.db"))
	if err != nil {
		return servicebootstrap.Status{}, fmt.Errorf("open service database: %w", err)
	}
	var schemaOwner *syncowner.Lock
	defer func() {
		if err := db.Close(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close service database: %w", err))
		}
		if schemaOwner != nil {
			resultErr = errors.Join(resultErr, schemaOwner.Release())
		}
	}()

	current, err := db.CurrentSchemaVersion()
	if err != nil {
		return servicebootstrap.Status{}, err
	}
	if current < database.MaxSchemaVersion() {
		restartAgent := false
		if manageService {
			restartAgent, err = pauseServiceForMigration()
		} else {
			err = assertNoServiceOnDefaultPort()
		}
		if err != nil {
			return servicebootstrap.Status{}, err
		}
		owner, migrationErr := acquireSchemaOwnership(db, dataDirectory)
		if migrationErr != nil {
			if owner != nil {
				migrationErr = errors.Join(migrationErr, owner.Release())
			}
			if restartAgent {
				migrationErr = errors.Join(migrationErr, startAgent(context.Background()))
			}
			return servicebootstrap.Status{}, migrationErr
		}
		schemaOwner = owner
		if restartAgent {
			if err := startAgent(context.Background()); err != nil {
				return servicebootstrap.Status{}, err
			}
		}
	} else if current > database.MaxSchemaVersion() {
		return servicebootstrap.Status{}, &database.ErrSchemaTooNew{DBVersion: current, BuildVersion: database.MaxSchemaVersion()}
	} else {
		owner, err := acquireSchemaOwnership(db, dataDirectory)
		if err != nil {
			return servicebootstrap.Status{}, err
		}
		schemaOwner = owner
	}
	maxKnown := database.MaxSchemaVersion()
	current, err = db.CurrentSchemaVersion()
	if err != nil {
		return servicebootstrap.Status{}, err
	}
	if current > maxKnown {
		return servicebootstrap.Status{}, &database.ErrSchemaTooNew{DBVersion: current, BuildVersion: maxKnown}
	}
	if current != maxKnown {
		return servicebootstrap.Status{}, fmt.Errorf("service database schema handshake failed: current %d, helper supports %d", current, maxKnown)
	}
	return servicebootstrap.Status{
		ProtocolVersion:  servicebootstrap.ProtocolVersion,
		APIVersion:       v1.Version,
		SchemaVersion:    current,
		MaxSchemaVersion: maxKnown,
	}, nil
}

func pauseServiceForMigration() (bool, error) {
	if runtime.GOOS == "darwin" {
		status, err := getAgentStatus(context.Background())
		if err != nil {
			return false, fmt.Errorf("inspect helper service before database upgrade: %w", err)
		}
		if status.Running {
			if err := stopAgent(context.Background()); err != nil {
				return false, fmt.Errorf("stop helper service before database upgrade: %w", err)
			}
			return true, nil
		}
	}
	if err := assertNoServiceOnDefaultPort(); err != nil {
		return false, err
	}
	return false, nil
}

func assertNoServiceOnDefaultPort() error {
	connection, err := net.DialTimeout("tcp", "127.0.0.1:8028", 200*time.Millisecond)
	if err == nil {
		_ = connection.Close()
		return errors.New("a mailbox helper is already listening on 127.0.0.1:8028; stop it before upgrading the service database")
	}
	var operationError *net.OpError
	if !errors.As(err, &operationError) || !errors.Is(operationError.Err, syscall.ECONNREFUSED) {
		return fmt.Errorf("check for an existing mailbox service before database upgrade: %w", err)
	}
	return nil
}

func acquireSchemaOwnership(db *database.DB, dataDirectory string) (*syncowner.Lock, error) {
	current, err := db.CurrentSchemaVersion()
	if err != nil {
		return nil, err
	}
	maxKnown := database.MaxSchemaVersion()
	if current > maxKnown {
		return nil, &database.ErrSchemaTooNew{DBVersion: current, BuildVersion: maxKnown}
	}
	lockPath := filepath.Join(dataDirectory, "mail-schema.owner.lock")
	shared := current == maxKnown
	var owner *syncowner.Lock
	if shared {
		owner, err = syncowner.TryAcquireShared(lockPath)
	} else {
		owner, err = syncowner.TryAcquire(lockPath)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot prepare the service database while another process owns it; stop the running mailbox service and retry: %w", err)
	}
	if shared {
		return owner, nil
	}
	current, err = db.CurrentSchemaVersion()
	if err != nil {
		_ = owner.Release()
		return nil, err
	}
	if current > maxKnown {
		_ = owner.Release()
		return nil, &database.ErrSchemaTooNew{DBVersion: current, BuildVersion: maxKnown}
	}
	if current == maxKnown {
		if err := owner.DowngradeToShared(); err != nil {
			_ = owner.Release()
			return nil, err
		}
		return owner, nil
	}
	if err := db.Migrate(); err != nil {
		_ = owner.Release()
		return nil, fmt.Errorf("migrate service database: %w", err)
	}
	if err := owner.DowngradeToShared(); err != nil {
		_ = owner.Release()
		return nil, err
	}
	return owner, nil
}
