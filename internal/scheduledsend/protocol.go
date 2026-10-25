// Package scheduledsend implements the encrypted queue and worker used by a
// trusted Send Later helper. It does not expose a network listener.
package scheduledsend

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/box"
)

const (
	// ProtocolVersion identifies the authenticated command format.
	ProtocolVersion = 1
	// MaxMessageBytes bounds MIME payloads until streaming attachments are added.
	MaxMessageBytes = 10 << 20
	// MaxEnvelopeBytes bounds the encoded, potentially untrusted submission.
	MaxEnvelopeBytes   = 24 << 20
	maxCiphertextBytes = 16 << 20
)

// PublicKey identifies a paired client or helper using a NaCl box public key.
type PublicKey [32]byte

// String encodes a public key for a local pairing exchange.
func (k PublicKey) String() string { return base64.RawURLEncoding.EncodeToString(k[:]) }

// ParsePublicKey decodes a public key obtained through a verified pairing flow.
func ParsePublicKey(value string) (PublicKey, error) {
	var key PublicKey
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != len(key) {
		return key, errors.New("invalid scheduled-send public key")
	}
	copy(key[:], decoded)
	if err := validatePublicKey(key); err != nil {
		return key, err
	}
	return key, nil
}

func validatePublicKey(key PublicKey) error {
	// X25519 rejects low-order public points. The fixed scalar is only used
	// for public-key validation; it is never used to encrypt a command.
	if _, err := curve25519.X25519(make([]byte, 32), key[:]); err != nil {
		return errors.New("invalid scheduled-send public key")
	}
	return nil
}

// Delivery is an immutable prepared message. AccountID names an explicitly
// authorized account on the helper; credentials never belong in this payload.
type Delivery struct {
	AccountID string    `json:"accountId"`
	From      string    `json:"from"`
	To        []string  `json:"to"` // Includes envelope Bcc recipients.
	RFC822    []byte    `json:"rfc822"`
	DueAt     time.Time `json:"dueAt"`
}

// Command carries a monotonic revision. A cancel tombstone prevents an older,
// delayed schedule command from resurrecting a cancelled message.
type Command struct {
	Version  int       `json:"version"`
	JobID    string    `json:"jobId"`
	Revision int64     `json:"revision"`
	Action   string    `json:"action"` // schedule, cancel, or status
	Delivery *Delivery `json:"delivery,omitempty"`
}

// Envelope contains only routing public keys, a random nonce, and ciphertext.
// Message content, recipients, account selection and due time are encrypted.
type Envelope struct {
	Version    int    `json:"version"`
	HelperKey  string `json:"helperKey"`
	ClientKey  string `json:"clientKey"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

// SealCommand authenticates a command as the client and encrypts it to the
// helper. The helper public key must already have been verified during pairing.
func SealCommand(command Command, client *Identity, helper PublicKey) ([]byte, error) {
	if client == nil || helper == (PublicKey{}) {
		return nil, errors.New("client identity and helper key are required")
	}
	if err := validatePublicKey(helper); err != nil {
		return nil, err
	}
	if err := validatePublicKey(client.PublicKey()); err != nil {
		return nil, err
	}
	if err := command.validate(); err != nil {
		return nil, err
	}
	plaintext, err := json.Marshal(command)
	if err != nil {
		return nil, fmt.Errorf("encode scheduled command: %w", err)
	}
	defer clear(plaintext)
	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nil, fmt.Errorf("generate scheduled-send nonce: %w", err)
	}
	ciphertext := box.Seal(nil, plaintext, &nonce, (*[32]byte)(&helper), &client.private)
	if len(ciphertext) > maxCiphertextBytes {
		return nil, errors.New("scheduled command exceeds size limit")
	}
	return json.Marshal(Envelope{ProtocolVersion, helper.String(), client.PublicKey().String(), nonce[:], ciphertext})
}

// OpenCommand decrypts and authenticates an envelope. The caller must separately
// verify that ClientKey is paired and permitted to use the selected account.
func OpenCommand(encoded []byte, helper *Identity) (Command, PublicKey, error) {
	envelope, err := decodeEnvelope(encoded)
	if err != nil {
		return Command{}, PublicKey{}, err
	}
	return openEnvelope(envelope, helper)
}

func openEnvelope(envelope Envelope, helper *Identity) (Command, PublicKey, error) {
	var command Command
	var clientKey PublicKey
	if helper == nil {
		return command, clientKey, errors.New("helper identity is required")
	}
	if envelope.HelperKey != helper.PublicKey().String() {
		return command, clientKey, errors.New("envelope is addressed to another helper")
	}
	clientKey, err := ParsePublicKey(envelope.ClientKey)
	if err != nil {
		return command, clientKey, err
	}
	var nonce [24]byte
	copy(nonce[:], envelope.Nonce)
	plaintext, ok := box.Open(nil, envelope.Ciphertext, &nonce, (*[32]byte)(&clientKey), &helper.private)
	if !ok {
		return command, clientKey, errors.New("scheduled command authentication failed")
	}
	defer clear(plaintext)
	if err := decodeJSON(plaintext, &command); err != nil {
		if command.Delivery != nil {
			clear(command.Delivery.RFC822)
		}
		return Command{}, clientKey, fmt.Errorf("decode scheduled command: %w", err)
	}
	if err := command.validate(); err != nil {
		if command.Delivery != nil {
			clear(command.Delivery.RFC822)
		}
		return Command{}, clientKey, err
	}
	return command, clientKey, nil
}

func decodeEnvelope(encoded []byte) (Envelope, error) {
	var envelope Envelope
	if len(encoded) == 0 || len(encoded) > MaxEnvelopeBytes {
		return envelope, errors.New("invalid scheduled envelope size")
	}
	if err := decodeJSON(encoded, &envelope); err != nil {
		return envelope, fmt.Errorf("decode scheduled envelope: %w", err)
	}
	if envelope.Version != ProtocolVersion || len(envelope.Nonce) != 24 ||
		len(envelope.Ciphertext) < box.Overhead || len(envelope.Ciphertext) > maxCiphertextBytes {
		return envelope, errors.New("unsupported or invalid scheduled envelope")
	}
	return envelope, nil
}

func decodeJSON(encoded []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("trailing data in scheduled payload")
	}
	return nil
}

func (c Command) validate() error {
	if c.Version != ProtocolVersion || c.Revision <= 0 {
		return errors.New("invalid scheduled command version or revision")
	}
	if _, err := uuid.Parse(c.JobID); err != nil {
		return errors.New("scheduled job requires a UUID")
	}
	switch c.Action {
	case "cancel", "status":
		if c.Delivery != nil {
			return errors.New("cancel command must not contain a delivery")
		}
		return nil
	case "schedule":
		if c.Delivery == nil {
			return errors.New("schedule command requires a delivery")
		}
		return c.Delivery.validate()
	default:
		return errors.New("unsupported scheduled command action")
	}
}

func (d Delivery) validate() error {
	if d.AccountID == "" || len(d.AccountID) > 256 || d.DueAt.IsZero() || len(d.To) == 0 || len(d.To) > 1000 {
		return errors.New("scheduled delivery requires an account, due time, and recipients")
	}
	if len(d.RFC822) == 0 || len(d.RFC822) > MaxMessageBytes {
		return errors.New("scheduled message exceeds MIME size limit")
	}
	for _, address := range append([]string{d.From}, d.To...) {
		parsed, err := mail.ParseAddress(address)
		if err != nil || parsed.Address != address || strings.ContainsAny(address, "\r\n") {
			return errors.New("scheduled delivery requires bare valid envelope addresses")
		}
	}
	message, err := mail.ReadMessage(bytes.NewReader(d.RFC822))
	if err != nil {
		return fmt.Errorf("invalid prepared scheduled message: %w", err)
	}
	from, err := mail.ParseAddress(message.Header.Get("From"))
	if err != nil || !strings.EqualFold(from.Address, d.From) || len(message.Header["From"]) != 1 {
		return errors.New("prepared message From differs from authorized envelope sender")
	}
	messageID := message.Header.Get("Message-Id")
	if len(message.Header["Message-Id"]) != 1 || !strings.HasPrefix(messageID, "<") || !strings.HasSuffix(messageID, ">") || !strings.Contains(messageID, "@") || strings.ContainsAny(messageID, "\r\n \t") {
		return errors.New("prepared message requires one stable Message-ID")
	}
	if len(message.Header["Bcc"]) != 0 {
		return errors.New("prepared message must keep Bcc in envelope recipients only")
	}
	return nil
}
