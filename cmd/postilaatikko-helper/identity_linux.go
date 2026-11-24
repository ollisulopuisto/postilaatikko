//go:build linux

package main

import "github.com/ollisulopuisto/postilaatikko/internal/scheduledsend"

func createHelperIdentity(backend, data, name string) (*scheduledsend.Identity, error) {
	if backend == "encrypted-file" {
		return (scheduledsend.FileIdentityStore{Directory: data}).Create(name)
	}
	return scheduledsend.CreateIdentity(name)
}
func loadHelperIdentity(backend, data, name string) (*scheduledsend.Identity, error) {
	if backend == "encrypted-file" {
		return (scheduledsend.FileIdentityStore{Directory: data}).Load(name)
	}
	return scheduledsend.LoadIdentity(name)
}
