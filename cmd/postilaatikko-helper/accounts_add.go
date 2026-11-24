package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/ollisulopuisto/postilaatikko/internal/account"
	"github.com/ollisulopuisto/postilaatikko/internal/accountsetup"
	"github.com/ollisulopuisto/postilaatikko/internal/credentials"
	"github.com/ollisulopuisto/postilaatikko/internal/logging"
	"github.com/ollisulopuisto/postilaatikko/internal/oauth2"
	"golang.org/x/term"
)

func runAccountsAdd(args []string, dataDefault string, input *os.File, out, errOut io.Writer) error {
	flags := flag.NewFlagSet("accounts add", flag.ContinueOnError)
	flags.SetOutput(errOut)
	dir := flags.String("data", dataDefault, "profile to configure; stop app/helper first")
	provider := flags.String("provider", "google", "google, microsoft or imap")
	name := flags.String("name", "", "account name (defaults to email)")
	display := flags.String("display-name", "", "sender display name (defaults to email)")
	email := flags.String("email", "", "email address, required for IMAP; optional OAuth identity check")
	username := flags.String("username", "", "IMAP username (defaults to email)")
	incoming := flags.String("imap-host", "", "IMAP server")
	incomingPort := flags.Int("imap-port", 993, "IMAP TLS port")
	outgoing := flags.String("smtp-host", "", "SMTP server")
	outgoingPort := flags.Int("smtp-port", 587, "SMTP STARTTLS port")
	receiveOnly := flags.Bool("receive-only", false, "disable outgoing mail")
	smtpUser := flags.String("smtp-username", "", "separate SMTP username; prompts for a separate password")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected account setup arguments")
	}
	if *provider != "google" && *provider != "microsoft" && *provider != "imap" {
		return errors.New("provider must be google, microsoft or imap")
	}
	if err := logging.Init(logging.Config{Level: "disabled"}); err != nil {
		return err
	}
	store, err := accountsetup.Open(*dir)
	if err != nil {
		return err
	}
	defer store.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	ctx, timeout := context.WithTimeout(ctx, 5*time.Minute)
	defer timeout()
	config := account.AccountConfig{Name: *name, DisplayName: *display, Email: strings.TrimSpace(*email), Username: *username, IMAPHost: *incoming, IMAPPort: *incomingPort, IMAPSecurity: account.SecurityTLS, SMTPHost: *outgoing, SMTPPort: *outgoingPort, SMTPSecurity: account.SecurityStartTLS, SMTPUsername: *smtpUser, NoOutgoingServer: *receiveOnly, SyncInterval: 30}
	var tokens *credentials.OAuthTokens
	password, smtpPassword := "", ""
	if *provider == "imap" {
		if config.Username == "" {
			config.Username = config.Email
		}
		if config.Name == "" {
			config.Name = config.Email
		}
		if config.DisplayName == "" {
			config.DisplayName = config.Email
		}
		if err := config.Validate(); err != nil {
			return errors.New("IMAP setup needs email, incoming host and outgoing host (or --receive-only)")
		}
		if *incomingPort < 1 || *incomingPort > 65535 || *outgoingPort < 1 || *outgoingPort > 65535 {
			return errors.New("invalid mail server port")
		}
		password, err = readAccountPassword(input, errOut, "Incoming password: ")
		if err != nil {
			return err
		}
		if config.SMTPUsername != "" && !config.NoOutgoingServer {
			smtpPassword, err = readAccountPassword(input, errOut, "Outgoing password: ")
			if err != nil {
				return err
			}
		}
	} else {
		manager := oauth2.NewManager()
		defer manager.CancelAuthFlow()
		cfg, err := store.Provider(*provider)
		if err != nil {
			return errors.New("OAuth provider is unavailable")
		}
		link, err := manager.StartAuthFlowWithProvider(ctx, &cfg)
		if err != nil {
			return errors.New("could not start desktop OAuth consent")
		}
		parsed, _ := url.Parse(link)
		redirect, _ := url.Parse(parsed.Query().Get("redirect_uri"))
		fmt.Fprintf(out, "Keep this command running. On another computer first forward port %s to this host's 127.0.0.1:%s over SSH. Then open the URL below in that computer's browser. Do not paste callback URLs or tokens into chat.\n%s\n", redirect.Port(), redirect.Port(), link)
		pending, identity, err := manager.WaitForSetupCallback(ctx)
		if err != nil {
			return err
		}
		defer pending.Clear()
		if config.Email != "" && !strings.EqualFold(config.Email, identity) {
			return errors.New("signed-in identity does not match --email; no account added")
		}
		config.Email, config.Username = identity, identity
		config.AuthType = account.AuthOAuth2
		config.IMAPHost, config.SMTPHost = "imap.gmail.com", "smtp.gmail.com"
		if *provider == "microsoft" {
			config.IMAPHost, config.SMTPHost = "outlook.office365.com", "smtp.office365.com"
		}
		config.IMAPPort, config.SMTPPort = 993, 587
		if config.Name == "" {
			config.Name = identity
		}
		if config.DisplayName == "" {
			config.DisplayName = identity
		}
		tokens = &credentials.OAuthTokens{Provider: *provider, AccessToken: pending.AccessToken, RefreshToken: pending.RefreshToken, ExpiresAt: time.Now().Add(time.Duration(pending.ExpiresIn) * time.Second), Scopes: oauth2.GrantedScopes(pending, cfg.Scopes)}
		defer func() { tokens.AccessToken = ""; tokens.RefreshToken = "" }()
	}
	acc, err := store.Create(config, password, smtpPassword, tokens)
	password, smtpPassword = "", ""
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Account added: %s\t%s\nNo mail sent and no sync started. Restart the helper to load it.\n", acc.ID, acc.Email)
	return nil
}

func readAccountPassword(in *os.File, out io.Writer, prompt string) (string, error) {
	if !term.IsTerminal(int(in.Fd())) {
		return "", errors.New("password entry requires a terminal; no password flags, pipes or files are accepted")
	}
	fmt.Fprint(out, prompt)
	value, err := term.ReadPassword(int(in.Fd()))
	fmt.Fprintln(out)
	defer func() {
		for i := range value {
			value[i] = 0
		}
	}()
	if err != nil || len(value) == 0 {
		return "", errors.New("password entry cancelled or empty")
	}
	return string(value), nil
}
