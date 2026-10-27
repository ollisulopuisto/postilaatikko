package folder

import "database/sql"

func (s *Store) GmailLabel(accountID, folderID string) (string, error) {
	var label string
	err := s.db.QueryRow(`SELECT label_id FROM gmail_folder_refs WHERE account_id=? AND folder_id=?`, accountID, folderID).Scan(&label)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return label, err
}
func (s *Store) SetGmailLabel(accountID, folderID, labelID string) error {
	_, err := s.db.Exec(`INSERT INTO gmail_folder_refs(account_id,folder_id,label_id) VALUES(?,?,?) ON CONFLICT(folder_id) DO UPDATE SET label_id=excluded.label_id`, accountID, folderID, labelID)
	return err
}
func (s *Store) GmailSyncCursor(accountID string) (history, page, backfill string, err error) {
	err = s.db.QueryRow(`SELECT history_id,history_page,backfill_page FROM gmail_sync_state WHERE account_id=?`, accountID).Scan(&history, &page, &backfill)
	if err == sql.ErrNoRows {
		err = nil
	}
	return
}
func (s *Store) SetGmailSyncCursor(accountID, history, page, backfill string) error {
	_, err := s.db.Exec(`INSERT INTO gmail_sync_state(account_id,history_id,history_page,backfill_page) VALUES(?,?,?,?) ON CONFLICT(account_id) DO UPDATE SET history_id=excluded.history_id,history_page=excluded.history_page,backfill_page=excluded.backfill_page`, accountID, history, page, backfill)
	return err
}
func (s *Store) MarkSynced(folderID string) error {
	_, err := s.db.Exec(`UPDATE folders SET last_sync=CURRENT_TIMESTAMP WHERE id=?`, folderID)
	return err
}
