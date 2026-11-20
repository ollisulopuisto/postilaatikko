package gmail

import (
	"context"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
)

// ForwardingAddress is a destination already configured on this Gmail account.
type ForwardingAddress struct {
	Email              string `json:"forwardingEmail"`
	VerificationStatus string `json:"verificationStatus"`
}

// ValidForwardingAddress refuses display names, lists and control characters.
func ValidForwardingAddress(value string) bool {
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Address == value && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\r\n\x00") && len(value) <= 320
}

// ListForwardingAddresses only reads existing addresses. It never verifies or adds one.
func (c *Client) ListForwardingAddresses(ctx context.Context) ([]ForwardingAddress, error) {
	var reply struct {
		Addresses []ForwardingAddress `json:"forwardingAddresses"`
	}
	if err := c.do(ctx, http.MethodGet, "/settings/forwardingAddresses", nil, nil, &reply); err != nil {
		return nil, err
	}
	if len(reply.Addresses) > 1000 {
		return nil, fmt.Errorf("Gmail forwarding catalog exceeds limit")
	}
	return reply.Addresses, nil
}

// ValidateForwardingDestination accepts only an exact verified address on this account.
func ValidateForwardingDestination(address string, catalog []ForwardingAddress) error {
	if address == "" {
		return nil
	}
	if !ValidForwardingAddress(address) {
		return fmt.Errorf("choose a valid existing Gmail forwarding address")
	}
	for _, entry := range catalog {
		if entry.Email == address && entry.VerificationStatus == "accepted" {
			return nil
		}
	}
	return fmt.Errorf("forwarding address is not verified on this Gmail account; review again")
}
