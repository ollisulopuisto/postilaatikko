//go:build !darwin || !cgo

package macoscontacts

import "errors"

type platformBackend struct{}

func (platformBackend) Status() string { return "unavailable" }
func (platformBackend) Request() error { return errors.New("macOS Contacts requires macOS and cgo") }
func (platformBackend) Search(string, int) (Result, error) {
	return Result{}, errors.New("macOS Contacts is unavailable")
}
func (platformBackend) Open(string) error { return errors.New("macOS Contacts is unavailable") }
