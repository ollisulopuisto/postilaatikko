package scheduledsend

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/mail"
	"strings"

	"golang.org/x/crypto/nacl/box"
)

const maxPreviewBytes = 16 << 10

// Preview contains only enough information to identify a scheduled message.
// Its cached representation is authenticated and encrypted to the client key.
type Preview struct {
	Subject string `json:"subject"`
	From    string `json:"from"`
	To      string `json:"to"`
	ToCount int    `json:"toCount"`
	Kind    string `json:"kind"`
}

func deliveryPreview(delivery *Delivery) (Preview, error) {
	message, err := mail.ReadMessage(bytes.NewReader(delivery.RFC822))
	if err != nil {
		return Preview{}, err
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
	if err != nil {
		subject = message.Header.Get("Subject")
	}
	runes := []rune(subject)
	if len(runes) > 512 {
		subject = string(runes[:512]) + "…"
	}
	return Preview{Subject: subject, From: delivery.From, To: strings.Join(delivery.To[:min(3, len(delivery.To))], ", "), ToCount: len(delivery.To), Kind: "scheduled-preview-v1"}, nil
}

func sealPreview(preview Preview, client *Identity) ([]byte, error) {
	plaintext, err := json.Marshal(preview)
	if err != nil || len(plaintext) > maxPreviewBytes {
		return nil, errors.New("scheduled message preview exceeds size limit")
	}
	defer clear(plaintext)
	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nil, err
	}
	public := client.PublicKey()
	encoded := append([]byte{ProtocolVersion}, nonce[:]...)
	return box.Seal(encoded, plaintext, &nonce, (*[32]byte)(&public), &client.private), nil
}

func openPreview(encoded []byte, client *Identity) (Preview, error) {
	if len(encoded) < 25+box.Overhead || len(encoded) > maxPreviewBytes+25+box.Overhead || encoded[0] != ProtocolVersion {
		return Preview{}, errors.New("invalid encrypted scheduled message preview")
	}
	var nonce [24]byte
	copy(nonce[:], encoded[1:25])
	public := client.PublicKey()
	plaintext, ok := box.Open(nil, encoded[25:], &nonce, (*[32]byte)(&public), &client.private)
	if !ok {
		return Preview{}, errors.New("scheduled message preview authentication failed")
	}
	defer clear(plaintext)
	var preview Preview
	if err := decodeJSON(plaintext, &preview); err != nil || preview.Kind != "scheduled-preview-v1" {
		return Preview{}, errors.New("invalid scheduled message preview")
	}
	return preview, nil
}
