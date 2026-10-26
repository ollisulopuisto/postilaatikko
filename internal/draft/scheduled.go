package draft

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
)

// ErrDraftNotFound prevents a concurrent cleanup from silently discarding a
// later editor save. Callers can preserve that input as a new draft.
var ErrDraftNotFound = errors.New("draft was removed while saving")

// Fingerprint hashes user-editable content, including encrypted bodies and
// attachments, while ignoring timestamps and background IMAP sync state.
func Fingerprint(d *Draft) string {
	hash := sha256.New()
	write := func(value []byte) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write(value)
	}
	for _, value := range []string{"Postilaatikko-draft-v1", d.ID, d.AccountID, d.ToList, d.CcList, d.BccList, d.Subject,
		d.BodyHTML, d.BodyText, d.InReplyToID, d.ReplyType, d.ReferencesList, d.IdentityID} {
		write([]byte(value))
	}
	for _, value := range []bool{d.SignMessage, d.Encrypted, d.PGPSignMessage, d.PGPEncrypted} {
		flag := byte(0)
		if value {
			flag = 1
		}
		write([]byte{flag})
	}
	write(d.EncryptedBody)
	write(d.PGPEncryptedBody)
	write(d.AttachmentsData)
	return hex.EncodeToString(hash.Sum(nil))
}

// DeleteUnchanged removes the exact version previously checked by cleanup.
// Any intervening edit/sync update preserves the row for a later decision.
func (s *Store) DeleteUnchanged(ctx context.Context, d *Draft) (bool, error) {
	if d.StorageVersion == "" {
		return false, nil
	}
	result, err := s.db.ExecContext(ctx, "DELETE FROM drafts WHERE id = ? AND quote(updated_at) = ?", d.ID, d.StorageVersion)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}
