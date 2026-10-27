package app

// GetReaderIdentityEmails includes inactive/historical identities for display
// matching. It does not change which accounts/identities may compose or send.
func (a *App) GetReaderIdentityEmails() ([]string, error) {
	accounts, err := a.accountStore.List()
	if err != nil {
		return nil, err
	}
	var emails []string
	for _, account := range accounts {
		emails = append(emails, account.Email)
		identities, err := a.accountStore.GetIdentities(account.ID)
		if err != nil {
			return nil, err
		}
		for _, identity := range identities {
			emails = append(emails, identity.Email)
		}
	}
	return emails, nil
}
