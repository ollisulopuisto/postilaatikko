package message

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ReadVerifiedDownload returns only unchanged bounded regular files in the
// managed cache. The stored digest comes from verified original bytes.
func (s *AttachmentStore) ReadVerifiedDownload(root, id string) ([]byte, bool) {
	const limit = 50 * 1024 * 1024
	var path string
	var digest []byte
	var size int64
	err := s.db.QueryRow(`SELECT i.local_path,i.sha256,i.byte_size FROM attachment_download_integrity i JOIN attachments a ON a.id=i.attachment_id WHERE i.attachment_id=? AND a.local_path=i.local_path`, id).Scan(&path, &digest, &size)
	if err != nil || root == "" || path == "" || size < 0 || size > limit || len(digest) != sha256.Size {
		return nil, false
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, false
	}
	canonicalPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, false
	}
	rel, err := filepath.Rel(canonicalRoot, canonicalPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return nil, false
	}
	info, err := os.Stat(canonicalPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() != size {
		return nil, false
	}
	f, err := os.Open(canonicalPath)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != size {
		return nil, false
	}
	content, err := io.ReadAll(io.LimitReader(f, size+1))
	if err != nil || int64(len(content)) != size {
		return nil, false
	}
	got := sha256.Sum256(content)
	if !bytes.Equal(got[:], digest) {
		return nil, false
	}
	return content, true
}

// SetVerifiedDownload records the editable location and original integrity
// together, so a failed write cannot authorize an unrelated cached path.
func (s *AttachmentStore) SetVerifiedDownload(id, path string, content []byte) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.Exec(`UPDATE attachments SET local_path=? WHERE id=?`, path, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("attachment unavailable: %w", sql.ErrNoRows)
	}
	digest := sha256.Sum256(content)
	_, err = tx.Exec(`INSERT INTO attachment_download_integrity(attachment_id,local_path,sha256,byte_size) VALUES(?,?,?,?) ON CONFLICT(attachment_id) DO UPDATE SET local_path=excluded.local_path,sha256=excluded.sha256,byte_size=excluded.byte_size`, id, path, digest[:], len(content))
	if err != nil {
		return err
	}
	return tx.Commit()
}
