//go:build linux

package scheduledsend

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	devicecrypto "github.com/ollisulopuisto/postilaatikko/internal/crypto"
	"golang.org/x/crypto/curve25519"
)

// FileIdentityStore is an explicit Linux-only alternative to Secret Service.
// It never reads the keyring, migrates an identity or replaces a missing key.
// Existing software device encryption is not protection against host compromise.
type FileIdentityStore struct{ Directory string }

func (s FileIdentityStore) paths(name string) (string, string, error) {
	if err := validIdentityName(name); err != nil {
		return "", "", err
	}
	if !filepath.IsAbs(s.Directory) {
		return "", "", errors.New("identity directory must be absolute")
	}
	if err := os.MkdirAll(s.Directory, 0700); err != nil {
		return "", "", err
	}
	root, err := os.Lstat(s.Directory)
	if err != nil || !root.IsDir() || root.Mode()&os.ModeSymlink != 0 || root.Mode().Perm()&0077 != 0 {
		return "", "", errors.New("profile directory must be private and not a symlink")
	}
	dir := filepath.Join(s.Directory, "send-later-identities")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", "", err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return "", "", errors.New("identity directory must be a private real directory (0700)")
	}
	sum := sha256.Sum256([]byte(name))
	return dir, filepath.Join(dir, fmt.Sprintf("%x.identity", sum)), nil
}

// Create refuses every existing identity file, including a corrupt one. A
// process lock plus atomic hard-link publication prevents accidental replacement.
func (s FileIdentityStore) Create(name string) (*Identity, error) {
	dir, path, err := s.paths(name)
	if err != nil {
		return nil, err
	}
	lock, err := devicecrypto.LockCredentialMigration(dir)
	if err != nil {
		return nil, errors.New("could not lock identity store")
	}
	defer lock.Close()
	if _, err := os.Lstat(path); err == nil {
		return nil, errors.New("encrypted identity already exists")
	} else if !os.IsNotExist(err) {
		return nil, errors.New("could not inspect identity")
	}
	if _, err := os.Lstat(filepath.Join(dir, "device.key")); os.IsNotExist(err) {
		entries, err := filepath.Glob(filepath.Join(dir, "*.identity"))
		if err != nil || len(entries) > 0 {
			return nil, errors.New("identity encryption key missing; refusing replacement")
		}
	}
	enc, _, version, err := devicecrypto.NewEncryptor(dir)
	if err != nil || version == devicecrypto.KeyVersionLegacy {
		return nil, errors.New("identity encryption key unavailable or unsupported")
	}
	identity, err := GenerateIdentity()
	if err != nil {
		return nil, err
	}
	ciphertext, err := enc.Encrypt("postilaatikko-send-later-v1:" + name + ":" + base64.StdEncoding.EncodeToString(identity.private[:]))
	if err != nil {
		return nil, errors.New("could not encrypt identity")
	}
	tmp, err := os.CreateTemp(dir, ".identity-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err = tmp.WriteString(ciphertext); err != nil {
		return nil, errors.New("could not write encrypted identity")
	}
	if err = tmp.Sync(); err != nil {
		return nil, err
	}
	if err = tmp.Close(); err != nil {
		return nil, err
	}
	if err = os.Link(tmp.Name(), path); err != nil {
		return nil, errors.New("identity was not published; no existing key replaced")
	}
	d, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer d.Close()
	if err = d.Sync(); err != nil {
		return nil, err
	}
	return identity, nil
}

// Load requires the original private files and validates the identity's name
// binding. Corruption or loss is an error, never a reason to generate a key.
func (s FileIdentityStore) Load(name string) (*Identity, error) {
	dir, path, err := s.paths(name)
	if err != nil {
		return nil, err
	}
	lock, err := devicecrypto.LockCredentialMigration(dir)
	if err != nil {
		return nil, errors.New("could not lock identity store")
	}
	defer lock.Close()
	for _, p := range []string{path, filepath.Join(dir, "device.key")} {
		info, err := os.Lstat(p)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return nil, errors.New("identity and encryption key must exist as private regular files")
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("encrypted identity unavailable")
	}
	defer f.Close()
	before, err := os.Lstat(path)
	if err != nil {
		return nil, errors.New("identity unavailable")
	}
	opened, err := f.Stat()
	if err != nil || !before.Mode().IsRegular() || !os.SameFile(before, opened) {
		return nil, errors.New("identity changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil || len(data) > 8192 {
		return nil, errors.New("invalid encrypted identity")
	}
	enc, _, version, err := devicecrypto.NewEncryptor(dir)
	if err != nil || version == devicecrypto.KeyVersionLegacy {
		return nil, errors.New("identity encryption key unavailable or unsupported")
	}
	plaintext, err := enc.Decrypt(string(data))
	if err != nil {
		return nil, errors.New("could not decrypt identity")
	}
	prefix := "postilaatikko-send-later-v1:" + name + ":"
	if len(plaintext) < len(prefix) || plaintext[:len(prefix)] != prefix {
		return nil, errors.New("identity name binding mismatch")
	}
	private, err := base64.StdEncoding.DecodeString(plaintext[len(prefix):])
	if err != nil || len(private) != 32 {
		return nil, errors.New("invalid encrypted private key")
	}
	defer clear(private)
	public, err := curve25519.X25519(private, curve25519.Basepoint)
	if err != nil {
		return nil, errors.New("invalid identity")
	}
	var identity Identity
	copy(identity.private[:], private)
	copy(identity.public[:], public)
	return &identity, nil
}
