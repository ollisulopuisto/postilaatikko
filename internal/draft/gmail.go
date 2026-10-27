package draft

import "database/sql"

func (s *Store) GmailID(localID string) (string, error) {
	var id string
	err := s.db.QueryRow(`SELECT provider_id FROM gmail_draft_refs WHERE draft_id=?`, localID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}
func (s *Store) SetGmailID(localID, id string) error {
	_, err := s.db.Exec(`INSERT INTO gmail_draft_refs(draft_id,provider_id) VALUES(?,?) ON CONFLICT(draft_id) DO UPDATE SET provider_id=excluded.provider_id`, localID, id)
	return err
}
func (s *Store) GetByGmailID(accountID, providerID string) (*Draft, error) {
	var id string
	err := s.db.QueryRow(`SELECT d.id FROM drafts d JOIN gmail_draft_refs r ON r.draft_id=d.id WHERE d.account_id=? AND r.provider_id=? ORDER BY d.updated_at DESC LIMIT 1`, accountID, providerID).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.Get(id)
}
