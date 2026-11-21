package message

// DeduplicateThreadMessages selects one account-scoped copy per RFC Message-ID,
// keeping its first chronological position. Folder/scope membership takes
// priority, then a live server target, then cached content. Ties retain the first
// copy. Messages without an RFC ID stay distinct. A nil matched map means no
// additional scope ranking.
func DeduplicateThreadMessages(messages []*Message, folderID string, matched map[string]bool) []*Message {
	type copyKey struct{ account, rfcID, fallbackID string }
	indices := make(map[copyKey]int, len(messages))
	result := make([]*Message, 0, len(messages))
	better := func(candidate, existing *Message) bool {
		candidateInFolder := folderID != "" && candidate.FolderID == folderID
		existingInFolder := folderID != "" && existing.FolderID == folderID
		if candidateInFolder != existingInFolder {
			return candidateInFolder
		}
		if matched != nil && matched[candidate.ID] != matched[existing.ID] {
			return matched[candidate.ID]
		}
		// A readable snapshot must not hide a live target for reader actions.
		candidateLive := candidate.UID > 0 && int32(candidate.UID) > 0
		existingLive := existing.UID > 0 && int32(existing.UID) > 0
		if candidateLive != existingLive {
			return candidateLive
		}
		if candidate.BodyFetched != existing.BodyFetched {
			return candidate.BodyFetched
		}
		return false
	}
	for _, msg := range messages {
		key := copyKey{account: msg.AccountID, rfcID: normalizeMessageID(msg.MessageID)}
		if key.rfcID == "" {
			key.fallbackID = msg.ID
		}
		if index, exists := indices[key]; exists {
			if better(msg, result[index]) {
				result[index] = msg
			}
		} else {
			indices[key] = len(result)
			result = append(result, msg)
		}
	}
	// Reader content may come from a downloaded copy while flags and mutations
	// keep the selected live identity. Never persist this presentation overlay.
	for i, selected := range result {
		if selected.BodyFetched || selected.UID == 0 || int32(selected.UID) < 0 {
			continue
		}
		rfc := normalizeMessageID(selected.MessageID)
		if rfc == "" {
			continue
		}
		for _, cached := range messages {
			if int32(cached.UID) < 0 && cached.AccountID == selected.AccountID && normalizeMessageID(cached.MessageID) == rfc && cached.BodyFetched && (cached.BodyText != "" || cached.BodyHTML != "" || cached.HasSMIME || cached.HasPGP) {
				overlay := *selected
				overlay.BodyText = cached.BodyText
				overlay.BodyHTML = cached.BodyHTML
				overlay.BodyFetched = true
				overlay.CachedBodySourceID = cached.ID
				overlay.HasAttachments = cached.HasAttachments || selected.HasAttachments
				overlay.HasSMIME, overlay.SMIMEEncrypted = cached.HasSMIME, cached.SMIMEEncrypted
				overlay.SMIMEStatus, overlay.SMIMESignerEmail, overlay.SMIMESignerSubject = cached.SMIMEStatus, cached.SMIMESignerEmail, cached.SMIMESignerSubject
				overlay.HasPGP, overlay.PGPEncrypted = cached.HasPGP, cached.PGPEncrypted
				overlay.PGPStatus, overlay.PGPSignerEmail, overlay.PGPSignerKeyID = cached.PGPStatus, cached.PGPSignerEmail, cached.PGPSignerKeyID
				result[i] = &overlay
				break
			}
		}
	}
	return result
}
