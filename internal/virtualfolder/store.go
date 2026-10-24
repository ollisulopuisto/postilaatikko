package virtualfolder

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/logging"
	"github.com/rs/zerolog"
)

// Store provides persistence operations for virtual folders
type Store struct {
	db  *database.DB
	log zerolog.Logger
}

// NewStore creates a new virtual folder store
func NewStore(db *database.DB) *Store {
	s := &Store{
		db:  db,
		log: logging.WithComponent("virtualfolder-store"),
	}
	if err := s.ensureTables(); err != nil {
		// Table creation is idempotent; a failure here is fatal because every
		// virtual folder operation depends on the schema. Surface it at startup
		// rather than failing with a confusing "no such table" on first use.
		panic(fmt.Errorf("virtualfolder store: ensure tables: %w", err))
	}
	return s
}

// ensureTables creates the virtual_folders schema if it is not already present.
// This is intentionally lazy (not a numbered migration) so it does not disturb
// the existing migration sequence; see the contact store's ensureTable for the
// same pattern.
func (s *Store) ensureTables() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS virtual_folders (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			order_index INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);
		CREATE TABLE IF NOT EXISTS virtual_folder_members (
			virtual_folder_id TEXT NOT NULL,
			folder_id TEXT NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
			PRIMARY KEY (virtual_folder_id, folder_id)
		);
	`)
	return err
}

// Create inserts a new virtual folder and its folder members
func (s *Store) Create(vf *VirtualFolder) error {
	if vf.ID == "" {
		vf.ID = uuid.New().String()
	}

	now := time.Now().UTC()
	if vf.CreatedAt.IsZero() {
		vf.CreatedAt = now
	}
	vf.UpdatedAt = now

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert virtual folder
	_, err = tx.Exec(`
		INSERT INTO virtual_folders (id, name, order_index, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, vf.ID, vf.Name, vf.OrderIndex, vf.CreatedAt, vf.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert virtual folder: %w", err)
	}

	// Insert members
	for _, folderID := range vf.FolderIDs {
		_, err = tx.Exec(`
			INSERT INTO virtual_folder_members (virtual_folder_id, folder_id)
			VALUES (?, ?)
		`, vf.ID, folderID)
		if err != nil {
			return fmt.Errorf("failed to insert virtual folder member %s: %w", folderID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Get returns a single virtual folder by its ID
func (s *Store) Get(id string) (*VirtualFolder, error) {
	vf := &VirtualFolder{}

	err := s.db.QueryRow(`
		SELECT id, name, order_index, created_at, updated_at
		FROM virtual_folders
		WHERE id = ?
	`, id).Scan(&vf.ID, &vf.Name, &vf.OrderIndex, &vf.CreatedAt, &vf.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get virtual folder: %w", err)
	}

	// Get members
	rows, err := s.db.Query(`
		SELECT folder_id
		FROM virtual_folder_members
		WHERE virtual_folder_id = ?
	`, id)
	if err != nil {
		return nil, fmt.Errorf("failed to query virtual folder members: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var folderID string
		if err := rows.Scan(&folderID); err != nil {
			return nil, fmt.Errorf("failed to scan folder_id: %w", err)
		}
		vf.FolderIDs = append(vf.FolderIDs, folderID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
    if vf.FolderIDs == nil {
        vf.FolderIDs = []string{}
    }

	return vf, nil
}

// List returns all virtual folders ordered by order_index
func (s *Store) List() ([]*VirtualFolder, error) {
	rows, err := s.db.Query(`
		SELECT v.id, v.name, v.order_index, v.created_at, v.updated_at, m.folder_id
		FROM virtual_folders v
		LEFT JOIN virtual_folder_members m ON v.id = m.virtual_folder_id
		ORDER BY v.order_index ASC, v.name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list virtual folders: %w", err)
	}
	defer rows.Close()

	vfMap := make(map[string]*VirtualFolder)
	var orderedList []*VirtualFolder

	for rows.Next() {
		var id, name string
		var orderIndex int
		var createdAt, updatedAt time.Time
		var folderID sql.NullString

		if err := rows.Scan(&id, &name, &orderIndex, &createdAt, &updatedAt, &folderID); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		vf, exists := vfMap[id]
		if !exists {
			vf = &VirtualFolder{
				ID:         id,
				Name:       name,
				OrderIndex: orderIndex,
				CreatedAt:  createdAt,
				UpdatedAt:  updatedAt,
				FolderIDs:  []string{},
			}
			vfMap[id] = vf
			orderedList = append(orderedList, vf)
		}

		if folderID.Valid {
			vf.FolderIDs = append(vf.FolderIDs, folderID.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return orderedList, nil
}

// Update modifies an existing virtual folder and its folder members
func (s *Store) Update(vf *VirtualFolder) error {
	vf.UpdatedAt = time.Now().UTC()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		UPDATE virtual_folders
		SET name = ?, order_index = ?, updated_at = ?
		WHERE id = ?
	`, vf.Name, vf.OrderIndex, vf.UpdatedAt, vf.ID)
	if err != nil {
		return fmt.Errorf("failed to update virtual folder: %w", err)
	}

	// Recreate members: delete existing and insert new
	_, err = tx.Exec(`DELETE FROM virtual_folder_members WHERE virtual_folder_id = ?`, vf.ID)
	if err != nil {
		return fmt.Errorf("failed to clear virtual folder members: %w", err)
	}

	for _, folderID := range vf.FolderIDs {
		_, err = tx.Exec(`
			INSERT INTO virtual_folder_members (virtual_folder_id, folder_id)
			VALUES (?, ?)
		`, vf.ID, folderID)
		if err != nil {
			return fmt.Errorf("failed to insert virtual folder member %s: %w", folderID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Delete removes a virtual folder (members are cascaded)
func (s *Store) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM virtual_folders WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete virtual folder: %w", err)
	}
	return nil
}
