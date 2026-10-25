package certificate

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// BuildTLSConfig returns a tls.Config that verifies certificates against
// the system CA pool first, then the trusted certificate store, and returns
// a CertificateError if the certificate is still untrusted.
func BuildTLSConfig(host string, store *Store) *tls.Config {
	return buildTrustVerifyingTLSConfig(host, store)
}

func buildTrustVerifyingTLSConfig(host string, store *Store) *tls.Config {
	serverHost := host
	trustedHost, hostErr := normalizeHost(host)
	if hostErr == nil {
		serverHost = trustedHost
	}

	verifyingTLSConfig := &tls.Config{
		ServerName:         serverHost,
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("no certificates presented")
			}

			// Parse the leaf certificate
			cert, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return fmt.Errorf("failed to parse certificate: %w", err)
			}

			// Try system CA verification first.
			systemErr := verifyWithSystemCAs(cert, serverHost, rawCerts)
			if systemErr == nil {
				return nil // System CAs trust this cert
			}

			// Only after normal validation fails may a host-bound exception apply.
			fingerprint := Fingerprint(rawCerts[0])
			if hostErr == nil && store != nil && store.IsTrusted(trustedHost, fingerprint) {
				now := time.Now()
				if !now.Before(cert.NotBefore) && !now.After(cert.NotAfter) {
					return nil // We trust this cert and it has not expired
				}
				if now.After(cert.NotAfter) {
					systemErr = fmt.Errorf("certificate has expired")
				} else {
					systemErr = fmt.Errorf("certificate is not yet valid")
				}
			}

			// Not trusted - return structured error
			info := ExtractCertInfo(rawCerts[0], systemErr)
			return &Error{
				Info:   info,
				Reason: info.ErrorReason,
			}
		},
	}
	return verifyingTLSConfig
}

// BuildTLSConfigDynamic is the host-agnostic variant of BuildTLSConfig: it
// verifies each connection against the server name negotiated for THAT
// connection (read from tls.ConnectionState in VerifyConnection), so a single
// *tls.Config can back a transport that talks to many hosts. The DAV clients
// reuse one shared transport across all CardDAV/CalDAV sources (and the auth
// broker hands out an account-level client before the DAV host is known), so a
// fixed-host config like BuildTLSConfig won't do. Same trust logic: system CA
// first, then the trusted-cert fingerprint store, else a structured *Error.
func BuildTLSConfigDynamic(store *Store) *tls.Config {
	return buildDynamicTrustVerifyingTLSConfig(store)
}

func buildDynamicTrustVerifyingTLSConfig(store *Store) *tls.Config {
	verifyingTLSConfig := &tls.Config{
		InsecureSkipVerify: true, // real verification happens in VerifyConnection
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return fmt.Errorf("no certificates presented")
			}
			leaf := cs.PeerCertificates[0]

			host, hostErr := normalizeHost(cs.ServerName)
			verifyHost := cs.ServerName
			if hostErr == nil {
				verifyHost = host
			}

			systemErr := verifyParsedWithSystemCAs(cs.PeerCertificates, verifyHost)
			if systemErr == nil {
				return nil
			}

			fingerprint := Fingerprint(leaf.Raw)
			if hostErr == nil && store != nil && store.IsTrusted(host, fingerprint) {
				now := time.Now()
				if !now.Before(leaf.NotBefore) && !now.After(leaf.NotAfter) {
					return nil
				}
				if now.After(leaf.NotAfter) {
					systemErr = fmt.Errorf("certificate has expired")
				} else {
					systemErr = fmt.Errorf("certificate is not yet valid")
				}
			}

			info := ExtractCertInfo(leaf.Raw, systemErr)
			return &Error{Info: info, Reason: info.ErrorReason}
		},
	}
	return verifyingTLSConfig
}

// verifyParsedWithSystemCAs verifies an already-parsed certificate chain against
// the system CA pool for the given host. Parsed-cert analog of
// verifyWithSystemCAs, used by the VerifyConnection-based dynamic config.
func verifyParsedWithSystemCAs(chain []*x509.Certificate, host string) error {
	roots, err := x509.SystemCertPool()
	if err != nil {
		return fmt.Errorf("failed to load system cert pool: %w", err)
	}
	intermediates := x509.NewCertPool()
	for _, c := range chain[1:] {
		intermediates.AddCert(c)
	}
	_, err = chain[0].Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		DNSName:       host,
	})
	return err
}

// verifyWithSystemCAs attempts to verify the certificate chain using system CAs
func verifyWithSystemCAs(cert *x509.Certificate, host string, rawCerts [][]byte) error {
	roots, err := x509.SystemCertPool()
	if err != nil {
		return fmt.Errorf("failed to load system cert pool: %w", err)
	}

	// Build intermediates pool from the rest of the chain
	intermediates := x509.NewCertPool()
	for _, rawCert := range rawCerts[1:] {
		intermediateCert, err := x509.ParseCertificate(rawCert)
		if err != nil {
			continue
		}
		intermediates.AddCert(intermediateCert)
	}

	_, err = cert.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		DNSName:       host,
	})
	return err
}

// Fingerprint returns the SHA-256 fingerprint of a DER-encoded certificate
func Fingerprint(derCert []byte) string {
	hash := sha256.Sum256(derCert)
	return hex.EncodeToString(hash[:])
}

// FormatFingerprint formats a hex fingerprint with colon separators for display
func FormatFingerprint(fp string) string {
	var parts []string
	for i := 0; i < len(fp); i += 2 {
		end := i + 2
		if end > len(fp) {
			end = len(fp)
		}
		parts = append(parts, strings.ToUpper(fp[i:end]))
	}
	return strings.Join(parts, ":")
}

// ExtractCertInfo parses a DER-encoded certificate into display-friendly info
func ExtractCertInfo(rawCert []byte, verifyErr error) *CertificateInfo {
	cert, err := x509.ParseCertificate(rawCert)
	if err != nil {
		return &CertificateInfo{
			Fingerprint: Fingerprint(rawCert),
			ErrorReason: "failed to parse certificate",
		}
	}

	info := &CertificateInfo{
		Subject:     formatDN(cert.Subject.CommonName, cert.Subject.Organization),
		Issuer:      formatDN(cert.Issuer.CommonName, cert.Issuer.Organization),
		Fingerprint: Fingerprint(rawCert),
		NotBefore:   cert.NotBefore.Format(time.RFC3339),
		NotAfter:    cert.NotAfter.Format(time.RFC3339),
		DNSNames:    cert.DNSNames,
		IsExpired:   time.Now().After(cert.NotAfter),
		ErrorReason: classifyError(verifyErr),
	}

	return info
}

// formatDN formats a distinguished name for display
func formatDN(cn string, org []string) string {
	if cn == "" && len(org) == 0 {
		return "(unknown)"
	}
	if cn != "" && len(org) > 0 {
		return fmt.Sprintf("%s (%s)", cn, org[0])
	}
	if cn != "" {
		return cn
	}
	return org[0]
}

// classifyError returns a human-readable reason for the verification failure
func classifyError(err error) string {
	if err == nil {
		return "unknown error"
	}

	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "signed by unknown authority"):
		return "self-signed or unknown certificate authority"
	case strings.Contains(errStr, "certificate has expired"):
		return "certificate has expired"
	case strings.Contains(errStr, "not valid"):
		return "certificate is not yet valid"
	case strings.Contains(errStr, "doesn't contain any IP SANs") ||
		strings.Contains(errStr, "cannot validate certificate"):
		return "certificate name mismatch"
	default:
		return errStr
	}
}
