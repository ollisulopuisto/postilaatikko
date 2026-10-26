// postilaatikko-helper runs the trusted E2EE Send Later endpoint on an always-on
// computer. Local administration is intentionally separate from HTTP commands.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/hkdb/aerion/internal/logging"
	"github.com/hkdb/aerion/internal/scheduledsend"
)

type allowFlags []string

func (a *allowFlags) String() string { return strings.Join(*a, ",") }
func (a *allowFlags) Set(value string) error {
	*a = append(*a, value)
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: postilaatikko-helper init|public-key|accounts|pair|revoke|serve|install-agent|uninstall-agent|schedule|submit|status|cancel|list|resolve [flags]")
	}
	command := args[0]
	dataDefault, err := defaultDataDirectory()
	if err != nil {
		return err
	}
	identityDefault := "studio"
	switch command {
	case "schedule", "submit", "status", "cancel":
		identityDefault = "client"
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	identityName := flags.String("identity", identityDefault, "local OS-keyring identity name")
	data := flags.String("data", dataDefault, "helper-local Postilaatikko account data directory")
	queue := flags.String("queue", filepath.Join(dataDefault, "send-later-helper"), "dedicated private helper queue directory")
	helperKeyText := flags.String("helper-key", "", "public key verified directly with the helper")
	clientKeyText := flags.String("client-key", "", "client public key verified during local pairing")
	endpoint := flags.String("endpoint", "http://127.0.0.1:8028", "helper HTTPS URL (HTTP is allowed only on loopback)")
	listen := flags.String("listen", "127.0.0.1:8028", "helper bind address; non-loopback requires TLS certificate and key")
	cert := flags.String("tls-cert", "", "TLS server certificate")
	key := flags.String("tls-key", "", "TLS server private-key file")
	account := flags.String("account", "", "helper-local sending account ID")
	from := flags.String("from", "", "sender address configured on the helper account")
	to := flags.String("to", "", "comma-separated envelope recipients, including Bcc")
	due := flags.String("due", "", "delivery time in RFC3339 with timezone, e.g. 2026-10-25T09:00:00+02:00")
	message := flags.String("message", "", "prepared immutable RFC822 .eml input")
	file := flags.String("file", "", "saved encrypted request to submit unchanged")
	out := flags.String("out", "", "new encrypted request file; must not already exist")
	jobID := flags.String("job", "", "scheduled job UUID")
	revision := flags.Int64("revision", 1, "positive monotonic job revision")
	outcome := flags.String("outcome", "", "locally verified unknown outcome: sent or failed")
	var allowed allowFlags
	flags.Var(&allowed, "allow", "pairing permission account-id=sender@example.com; repeat for each account/sender")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if err := logging.Init(logging.Config{Level: "warn", Console: true}); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if command == "uninstall-agent" {
		return uninstallAgent(ctx)
	}
	if command == "init" {
		identity, err := scheduledsend.CreateIdentity(*identityName)
		if err != nil {
			return err
		}
		fmt.Println(identity.PublicKey().String())
		return nil
	}
	if command == "accounts" {
		transport, err := scheduledsend.OpenSMTPTransport(*data)
		if err != nil {
			return err
		}
		defer transport.Close()
		accounts, err := transport.Accounts()
		if err != nil {
			return err
		}
		for _, account := range accounts {
			fmt.Printf("%s\t%s\t%s\n", account.ID, account.Name, account.Email)
		}
		return nil
	}
	identity, err := scheduledsend.LoadIdentity(*identityName)
	if err != nil {
		return err
	}
	if command == "public-key" {
		fmt.Println(identity.PublicKey().String())
		return nil
	}
	switch command {
	case "schedule", "submit", "status", "cancel":
		helperKey, err := scheduledsend.ParsePublicKey(*helperKeyText)
		if err != nil {
			return err
		}
		var encoded []byte
		if command == "submit" {
			encoded, err = readBounded(*file, scheduledsend.MaxEnvelopeBytes)
		}
		if command != "submit" {
			id := *jobID
			if command == "schedule" && id == "" {
				id = uuid.NewString()
			}
			request := scheduledsend.Command{Version: scheduledsend.ProtocolVersion, JobID: id, Revision: *revision, Action: command}
			if command == "schedule" {
				at, err := time.Parse(time.RFC3339, *due)
				if err != nil {
					return errors.New("--due must contain an explicit RFC3339 date, time and timezone")
				}
				raw, err := readBounded(*message, scheduledsend.MaxMessageBytes)
				if err != nil {
					return err
				}
				defer clear(raw)
				request.Delivery = &scheduledsend.Delivery{AccountID: *account, From: *from, To: strings.Split(*to, ","), RFC822: raw, DueAt: at}
			}
			encoded, err = scheduledsend.SealCommand(request, identity, helperKey)
			if err == nil && command != "status" {
				err = persistRequest(*out, encoded)
			}
		}
		if err != nil {
			return err
		}
		status, err := scheduledsend.SendEnvelope(ctx, *endpoint, encoded, identity, helperKey)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(status)
	}
	store, err := scheduledsend.OpenStore(*queue, identity)
	if err != nil {
		return err
	}
	defer store.Close()
	switch command {
	case "pair":
		clientKey, err := scheduledsend.ParsePublicKey(*clientKeyText)
		if err != nil {
			return err
		}
		transport, err := scheduledsend.OpenSMTPTransport(*data)
		if err != nil {
			return err
		}
		defer transport.Close()
		var permissions scheduledsend.Permissions
		for _, permission := range allowed {
			parts := strings.SplitN(permission, "=", 2)
			if len(parts) != 2 {
				return errors.New("--allow must be account-id=sender@example.com")
			}
			if err := transport.ValidateSender(parts[0], parts[1]); err != nil {
				return err
			}
			permissions.Senders = append(permissions.Senders, scheduledsend.SenderPermission{AccountID: parts[0], From: parts[1]})
		}
		return store.PairClient(ctx, clientKey, permissions)
	case "revoke":
		clientKey, err := scheduledsend.ParsePublicKey(*clientKeyText)
		if err != nil {
			return err
		}
		return store.RevokeClient(ctx, clientKey)
	case "list":
		statuses, err := store.ListStatus(ctx, 100, 0)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(statuses)
	case "resolve":
		return store.ResolveUnknown(ctx, *jobID, *revision, scheduledsend.State(*outcome))
	case "serve":
		transport, err := scheduledsend.OpenSMTPTransport(*data)
		if err != nil {
			return err
		}
		defer transport.Close()
		return serve(ctx, store, transport, *listen, *cert, *key)
	case "install-agent":
		transport, err := scheduledsend.OpenSMTPTransport(*data)
		if err != nil {
			return err
		}
		transport.Close()
		return installAgent(ctx, *identityName, *data, *queue, *listen, *cert, *key)
	default:
		return fmt.Errorf("unknown helper command %q", command)
	}
}

func persistRequest(path string, encoded []byte) error {
	if path == "" {
		return errors.New("--out is required: persist encrypted request bytes before submission so retries are idempotent")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err := file.Write(encoded); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		directory, err := os.Open(filepath.Dir(path))
		if err != nil {
			return err
		}
		defer directory.Close()
		return directory.Sync()
	}
	return nil
}

func readBounded(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("input exceeds scheduled-send size limit or cannot be read")
	}
	return data, nil
}

func defaultDataDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Aerion"), nil
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Aerion"), nil
	default:
		if data := os.Getenv("XDG_DATA_HOME"); data != "" {
			return filepath.Join(data, "aerion"), nil
		}
		return filepath.Join(home, ".local", "share", "aerion"), nil
	}
}

func validateListener(address, cert, key string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	loopback := strings.EqualFold(host, "localhost")
	if ip := net.ParseIP(host); ip != nil {
		loopback = ip.IsLoopback()
	}
	if (!loopback || cert != "" || key != "") && (cert == "" || key == "") {
		return errors.New("non-loopback helper listeners require both --tls-cert and --tls-key")
	}
	if cert != "" {
		if _, err := tls.LoadX509KeyPair(cert, key); err != nil {
			return fmt.Errorf("load helper TLS identity: %w", err)
		}
	}
	return nil
}

func serve(ctx context.Context, store *scheduledsend.Store, transport scheduledsend.Transport, address, cert, key string) error {
	if err := validateListener(address, cert, key); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: store.Handler(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	workerContext, cancel := context.WithCancel(ctx)
	defer cancel()
	worker := &scheduledsend.Worker{Store: store, Transport: transport}
	workerError := make(chan error, 1)
	serverError := make(chan error, 1)
	go func() { workerError <- worker.Run(workerContext) }()
	go func() {
		if cert != "" {
			serverError <- server.ServeTLS(listener, cert, key)
			return
		}
		serverError <- server.Serve(listener)
	}()
	var result error
	workerStopped := false
	select {
	case <-ctx.Done():
	case result = <-workerError:
		workerStopped = true
	case result = <-serverError:
	}
	cancel()
	shutdownContext, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	_ = server.Shutdown(shutdownContext)
	if !workerStopped {
		select {
		case <-workerError:
		case <-time.After(35 * time.Second):
			return errors.New("helper shutdown interrupted delivery; inspect its durable outcome before retrying")
		}
	}
	if errors.Is(result, context.Canceled) || errors.Is(result, http.ErrServerClosed) {
		return nil
	}
	return result
}
