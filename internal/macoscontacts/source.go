// Package macoscontacts reads Apple's Contacts database without writing to it.
package macoscontacts

import (
	"errors"
	"strings"
)

// Person contains only the fields needed for browsing and recipient completion.
type Person struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Emails []string `json:"emails"`
}

// Result describes a bounded read. Truncated means more records may match.
type Result struct {
	People    []Person `json:"people"`
	Truncated bool     `json:"truncated"`
}

type backend interface {
	Status() string
	Request() error
	Search(string, int) (Result, error)
	Open(string) error
}

// Source enforces the permission boundary before invoking platform operations.
type Source struct{ backend backend }

// New creates a read-only native source. It never prompts for permission.
func New() *Source { return &Source{backend: platformBackend{}} }

// Status checks permission without reading contacts or prompting.
func (s *Source) Status() string { return s.backend.Status() }

func canRead(status string) bool { return status == "authorized" || status == "limited" }

// RequestAccess prompts only when the user has not made a permission decision.
func (s *Source) RequestAccess() (string, error) {
	status := s.Status()
	if canRead(status) {
		return status, nil
	}
	if status != "not-determined" {
		return status, errors.New("Contacts access is unavailable; check macOS Privacy & Security settings")
	}
	if err := s.backend.Request(); err != nil {
		return s.Status(), err
	}
	status = s.Status()
	if !canRead(status) {
		return status, errors.New("Contacts access was not granted")
	}
	return status, nil
}

// Search reads at most 100 matching people, only after permission was granted.
func (s *Source) Search(query string, limit int) (Result, error) {
	if !canRead(s.Status()) {
		return Result{}, errors.New("Contacts access is not authorized")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	query = strings.TrimSpace(query)
	if len(query) > 512 || strings.ContainsRune(query, 0) {
		return Result{}, errors.New("invalid Contacts search")
	}
	return s.backend.Search(query, limit)
}

// Open shows a selected person in Contacts.app; changes belong to that app.
func (s *Source) Open(id string) error {
	if !canRead(s.Status()) {
		return errors.New("Contacts access is not authorized")
	}
	if strings.TrimSpace(id) == "" || len(id) > 1024 || strings.ContainsRune(id, 0) {
		return errors.New("invalid Contacts identifier")
	}
	return s.backend.Open(id)
}
