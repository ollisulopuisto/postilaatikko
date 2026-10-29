package app

import "github.com/hkdb/aerion/internal/macoscontacts"

// GetMacOSContactsStatus checks authorization without reading or prompting.
func (a *App) GetMacOSContactsStatus() string { return macoscontacts.New().Status() }

// RequestMacOSContactsAccess requests permission after an explicit user action.
func (a *App) RequestMacOSContactsAccess() (string, error) {
	return macoscontacts.New().RequestAccess()
}

// SearchMacOSContacts browses authorized native contacts without storing copies.
func (a *App) SearchMacOSContacts(query string, limit int) (macoscontacts.Result, error) {
	return macoscontacts.New().Search(query, limit)
}

// OpenMacOSContact shows the selected person in Contacts.app for editing there.
func (a *App) OpenMacOSContact(id string) error { return macoscontacts.New().Open(id) }
