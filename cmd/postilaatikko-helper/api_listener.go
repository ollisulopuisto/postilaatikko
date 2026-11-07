package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/hkdb/aerion/internal/automation"
)

type apiListenerConfig struct {
	address     string
	cert        string
	key         string
	allowedHost []string
}

func (c apiListenerConfig) validate() error {
	host, _, err := net.SplitHostPort(c.address)
	if err != nil {
		return fmt.Errorf("invalid --api-listen address: %w", err)
	}
	remote := !isLoopbackHost(host)
	if (c.cert == "") != (c.key == "") {
		return errors.New("--api-tls-cert and --api-tls-key must be provided together")
	}
	if remote && c.cert == "" {
		return errors.New("non-loopback --api-listen requires --api-tls-cert and --api-tls-key")
	}
	if remote && len(c.allowedHost) == 0 {
		return errors.New("non-loopback --api-listen requires at least one --api-allowed-host")
	}
	for _, allowedHost := range c.allowedHost {
		if !validAllowedHost(allowedHost) {
			return fmt.Errorf("invalid API Host allowlist entry %q", allowedHost)
		}
	}
	if c.cert == "" {
		return nil
	}
	if _, err := tls.LoadX509KeyPair(c.cert, c.key); err != nil {
		return fmt.Errorf("load API TLS identity: %w", err)
	}
	return nil
}

func (c apiListenerConfig) serverConfig() automation.ServerConfig {
	return automation.ServerConfig{
		Addr:         c.address,
		TLSCertFile:  c.cert,
		TLSKeyFile:   c.key,
		AllowedHosts: append([]string(nil), c.allowedHost...),
	}
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func validAllowedHost(host string) bool {
	host = strings.TrimSpace(host)
	return host != "" && !strings.ContainsAny(host, "* /\\")
}
