//go:build !linux

package main

import (
	"errors"
	"github.com/ollisulopuisto/postilaatikko/internal/scheduledsend"
)

func createHelperIdentity(backend, data, name string) (*scheduledsend.Identity, error) {
	if backend != "keyring" {
		return nil, errors.New("encrypted-file identities are supported only on Linux")
	}
	return scheduledsend.CreateIdentity(name)
}
func loadHelperIdentity(backend, data, name string) (*scheduledsend.Identity, error) {
	if backend != "keyring" {
		return nil, errors.New("encrypted-file identities are supported only on Linux")
	}
	return scheduledsend.LoadIdentity(name)
}
