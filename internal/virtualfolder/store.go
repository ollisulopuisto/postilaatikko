package virtualfolder

import (
	"database/sql"
	"fmt"
	"strings"
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

// ensureTables confirms the migration-created virtual-folder schema is present.
func (s *Store) ensureTables() error {
	var tableCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='virtual_folder_members'`).Scan(&tableCount); err != nil {
		return err
	}
	if tableCount == 0 {
		return fmt.Errorf("virtual folder schema is missing; run database migrations")
	}
	return nil
}

// Create inserts a new virtual folder and its folder members
func (s *Store) Create(vf *VirtualFolder) error {
	if vf == nil {
		return fmt.Errorf("virtual folder cannot be nil")
	}
	vf.Name = strings.TrimSpace(vf.Name)
	if vf.Name == "" {
		return fmt.Errorf("virtual folder name cannot be empty")
	}
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
	defer func() { _ = tx.Rollback() }() // no-op after a successful Commit

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

// UnreadCounts returns unread conversation counts for every virtual folder in one query.
func (s *Store) UnreadCounts() (map[string]int, error) {
	rows, err := s.db.Query(`
		SELECT v.id,
			COUNT(DISTINCT CASE WHEN m.id IS NOT NULL
				THEN COALESCE(m.thread_id, m.id) || ':' || a.id END)
		FROM virtual_folders v
		LEFT JOIN virtual_folder_members vm ON vm.virtual_folder_id = v.id
		LEFT JOIN folders f ON f.id = vm.folder_id
		LEFT JOIN accounts a ON a.id = f.account_id AND a.enabled = 1
		LEFT JOIN messages m ON m.folder_id = f.id AND m.is_read = 0 AND a.id IS NOT NULL
		GROUP BY v.id
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query virtual folder unread counts: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var id string
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			return nil, fmt.Errorf("failed to scan virtual folder unread count: %w", err)
		}
		counts[id] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read virtual folder unread counts: %w", err)
	}
	return counts, nil
}

// Reorder assigns each virtual folder its position in the supplied ID list.
func (s *Store) Reorder(ids []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin virtual folder reorder: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after a successful Commit
	for index, id := range ids {
		result, err := tx.Exec(`UPDATE virtual_folders SET order_index = ?, updated_at = ? WHERE id = ?`, index, time.Now().UTC(), id)
		if err != nil {
			return fmt.Errorf("failed to reorder virtual folder %s: %w", id, err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to confirm virtual folder reorder: %w", err)
		}
		if rows == 0 {
			return fmt.Errorf("virtual folder not found: %s", id)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit virtual folder reorder: %w", err)
	}
	return nil
}

// Update modifies an existing virtual folder and its folder members
func (s *Store) Update(vf *VirtualFolder) error {
	if vf == nil {
		return fmt.Errorf("virtual folder cannot be nil")
	}
	vf.Name = strings.TrimSpace(vf.Name)
	if vf.ID == "" {
		return fmt.Errorf("virtual folder ID cannot be empty")
	}
	if vf.Name == "" {
		return fmt.Errorf("virtual folder name cannot be empty")
	}
	vf.UpdatedAt = time.Now().UTC()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after a successful Commit

	result, err := tx.Exec(`
		UPDATE virtual_folders
		SET name = ?, order_index = ?, updated_at = ?
		WHERE id = ?
	`, vf.Name, vf.OrderIndex, vf.UpdatedAt, vf.ID)
	if err != nil {
		return fmt.Errorf("failed to update virtual folder: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to confirm virtual folder update: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("virtual folder not found: %s", vf.ID)
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

// Delete removes a virtual folder and its members.
func (s *Store) Delete(id string) error {
	if id == "" {
		return fmt.Errorf("virtual folder ID cannot be empty")
	}
	_, err := s.db.Exec(`DELETE FROM virtual_folders WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete virtual folder: %w", err)
	}
	return nil
}
