package message

// DeduplicateThreadMessages selects one account-scoped copy per RFC Message-ID,
// keeping its first chronological position. Folder/scope membership takes
// priority, then cached content; ties retain the first copy. Messages without
// an RFC ID stay distinct. A nil matched map means no additional scope ranking.
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
	return result
}
