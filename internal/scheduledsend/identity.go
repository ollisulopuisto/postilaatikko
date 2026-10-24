package scheduledsend

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/zalando/go-keyring"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/box"
)

const keyringService = "Postilaatikko.SendLater.v1"

// Identity owns a NaCl box private key. Its fields are unexported to prevent
// accidental JSON serialization into a relay request, status response, or log.
type Identity struct {
	public  PublicKey
	private [32]byte
}

// GenerateIdentity makes a fresh in-memory identity for a client or helper.
func GenerateIdentity() (*Identity, error) {
	public, private, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate scheduled-send identity: %w", err)
	}
	return &Identity{public: PublicKey(*public), private: *private}, nil
}

// PublicKey returns the public half used in a verified pairing exchange.
func (i *Identity) PublicKey() PublicKey { return i.public }

// CreateIdentity initializes an OS-keyring identity during serialized local
// setup. It refuses to replace an existing identity and has no file fallback.
func CreateIdentity(name string) (*Identity, error) {
	if err := validIdentityName(name); err != nil {
		return nil, err
	}
	_, err := keyring.Get(keyringService, name)
	if err == nil {
		return nil, errors.New("scheduled-send identity already exists")
	}
	if !errors.Is(err, keyring.ErrNotFound) {
		return nil, fmt.Errorf("read scheduled-send keyring: %w", err)
	}
	identity, err := GenerateIdentity()
	if err != nil {
		return nil, err
	}
	if err := keyring.Set(keyringService, name, base64.StdEncoding.EncodeToString(identity.private[:])); err != nil {
		return nil, fmt.Errorf("store scheduled-send identity in OS keyring: %w", err)
	}
	return identity, nil
}

// LoadIdentity unlocks an existing identity from the OS keyring. Missing or
// locked keys are errors; callers must never generate a replacement silently.
func LoadIdentity(name string) (*Identity, error) {
	if err := validIdentityName(name); err != nil {
		return nil, err
	}
	encoded, err := keyring.Get(keyringService, name)
	if err != nil {
		return nil, fmt.Errorf("load scheduled-send identity: %w", err)
	}
	private, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(private) != 32 {
		return nil, errors.New("invalid scheduled-send keyring identity")
	}
	defer clear(private)
	public, err := curve25519.X25519(private, curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("derive scheduled-send public key: %w", err)
	}
	var identity Identity
	copy(identity.private[:], private)
	copy(identity.public[:], public)
	return &identity, nil
}

func validIdentityName(name string) error {
	if strings.TrimSpace(name) == "" || len(name) > 256 {
		return errors.New("scheduled-send identity name is required and must be at most 256 bytes")
	}
	return nil
}
