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
	"github.com/hkdb/aerion/internal/alwayson"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/logging"
	"github.com/hkdb/aerion/internal/scheduledsend"
	"github.com/hkdb/aerion/internal/servicebootstrap"
)

type allowFlags []string

type helperAgentStatus struct {
	Supported bool `json:"supported"`
	Installed bool `json:"installed"`
	Running   bool `json:"running"`
}

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
		return errors.New("usage: postilaatikko-helper bootstrap|init|public-key|accounts|pair|revoke|serve|install-agent|agent-status|start-agent|stop-agent|uninstall-agent|schedule|submit|status|cancel|list|resolve|api-client [flags]")
	}
	command := args[0]
	dataDefault, err := defaultDataDirectory()
	if err != nil {
		return err
	}
	if command == "api-client" {
		if err := logging.Init(logging.Config{Level: "warn", Console: true}); err != nil {
			return err
		}
		return runAPIClientCommand(args[1:], dataDefault)
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
	apiListen := flags.String("api-listen", "127.0.0.1:8029", "authenticated mailbox API bind address; non-loopback requires API TLS and Host allowlist")
	apiCert := flags.String("api-tls-cert", "", "authenticated mailbox API TLS certificate")
	apiKey := flags.String("api-tls-key", "", "authenticated mailbox API TLS private-key file")
	withSync := flags.Bool("sync", false, "run bounded background mailbox header sync")
	withoutSync := flags.Bool("no-sync", false, "install/run without mailbox sync (Send Later only)")
	gmailPubSubProject := flags.String("gmail-pubsub-project", "", "Google Cloud project ID for the Gmail Pub/Sub subscription")
	gmailPubSubTopic := flags.String("gmail-pubsub-topic", "", "fully qualified Gmail watch topic, e.g. projects/PROJECT/topics/gmail")
	gmailPubSubSubscription := flags.String("gmail-pubsub-subscription", "", "fully qualified Pub/Sub pull subscription resource name")
	gmailPubSubCredentials := flags.String("gmail-pubsub-credentials", "", "optional Google Cloud ADC service-account credentials JSON path")
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
	var apiAllowedHosts allowFlags
	flags.Var(&apiAllowedHosts, "api-allowed-host", "exact Host name accepted by the authenticated mailbox API; repeat for each name")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *withSync && *withoutSync {
		return errors.New("--sync and --no-sync cannot be used together")
	}
	effectiveSync := *withSync
	if command == "install-agent" && !*withoutSync {
		effectiveSync = true
	}
	pushConfig := alwayson.GmailPushConfig{ProjectID: *gmailPubSubProject, Topic: *gmailPubSubTopic, Subscription: *gmailPubSubSubscription, CredentialsFile: *gmailPubSubCredentials}
	pushConfigured := pushConfig.ProjectID != "" || pushConfig.Topic != "" || pushConfig.Subscription != "" || pushConfig.CredentialsFile != ""
	if pushConfigured {
		if !effectiveSync {
			return errors.New("Gmail Pub/Sub requires mailbox sync")
		}
		if err := pushConfig.Validate(); err != nil {
			return err
		}
	}
	if err := logging.Init(logging.Config{Level: "warn", Console: true}); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if command == "agent-status" {
		status, err := getAgentStatus(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(status)
	}
	if command == "start-agent" {
		return startAgent(ctx)
	}
	if command == "stop-agent" {
		return stopAgent(ctx)
	}
	if command == "uninstall-agent" {
		return uninstallAgent(ctx)
	}
	if command == "bootstrap" {
		return runBootstrap(*data)
	}
	switch command {
	case "accounts", "pair", "install-agent":
		if err := prepareExistingServiceDatabase(*data, true); err != nil {
			return err
		}
	case "serve":
		if err := prepareExistingServiceDatabase(*data, false); err != nil {
			return err
		}
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
		syncRuntime, err := alwayson.OpenSyncRuntime(*data, transport.IMAPClientConfig)
		if err != nil {
			return err
		}
		if err := syncRuntime.SetGmailTokenSource(transport.GmailTokenSource); err != nil {
			_ = syncRuntime.Close()
			return err
		}
		if err := syncRuntime.SetGmailTriageLabeler(transport.SetGmailTriageLabel); err != nil {
			_ = syncRuntime.Close()
			return err
		}
		if err := syncRuntime.SetAccountCredentialLifecycle(transport.DeleteAccountCredentials, transport.ValidateSharedMailbox, transport.CopySharedMailboxCredentials); err != nil {
			_ = syncRuntime.Close()
			return err
		}
		if err := syncRuntime.SetAccountPasswordMutationHandler(transport.ApplyAccountPasswordMutation, transport.AccountMutationFingerprint); err != nil {
			_ = syncRuntime.Close()
			return err
		}
		apiConfig := apiListenerConfig{address: *apiListen, cert: *apiCert, key: *apiKey, allowedHost: apiAllowedHosts}
		if pushConfigured {
			if err := syncRuntime.ConfigureGmailPush(pushConfig); err != nil {
				_ = syncRuntime.Close()
				return err
			}
		}
		return serve(ctx, store, transport, syncRuntime, *listen, *cert, *key, effectiveSync, apiConfig)
	case "install-agent":
		transport, err := scheduledsend.OpenSMTPTransport(*data)
		if err != nil {
			return err
		}
		transport.Close()
		apiConfig := apiListenerConfig{address: *apiListen, cert: *apiCert, key: *apiKey, allowedHost: apiAllowedHosts}
		return installAgent(ctx, *identityName, *data, *queue, *listen, *cert, *key, effectiveSync, apiConfig, pushConfig)
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
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appData, "Aerion"), nil
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

func serve(ctx context.Context, store *scheduledsend.Store, transport scheduledsend.Transport, syncRuntime *alwayson.SyncRuntime, address, cert, key string, withSync bool, apiConfig apiListenerConfig) (result error) {
	if syncRuntime == nil {
		return errors.New("mailbox runtime is required to serve the authenticated API")
	}
	defer func() { result = errors.Join(result, syncRuntime.Close()) }()
	if err := validateListener(address, cert, key); err != nil {
		return err
	}
	if err := apiConfig.validate(); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	apiServer, err := syncRuntime.NewPersistentMailboxAPIServer(apiConfig.serverConfig())
	if err != nil {
		return errors.Join(err, listener.Close())
	}
	if err := apiServer.Start(); err != nil {
		return errors.Join(fmt.Errorf("start authenticated mailbox API: %w", err), listener.Close())
	}
	server := &http.Server{Handler: helperHandler(store, withSync), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	workerContext, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := syncRuntime.StartTriageMaintenance(workerContext); err != nil {
		return err
	}
	worker := &scheduledsend.Worker{Store: store, Transport: transport}
	if withSync {
		syncRuntime.Start(workerContext)
	}
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
	shutdownErr := errors.Join(server.Shutdown(shutdownContext), apiServer.Stop(shutdownContext))
	if !workerStopped {
		select {
		case <-workerError:
		case <-time.After(35 * time.Second):
			return errors.Join(shutdownErr, errors.New("helper shutdown interrupted delivery; inspect its durable outcome before retrying"))
		}
	}
	if errors.Is(result, context.Canceled) || errors.Is(result, http.ErrServerClosed) {
		result = nil
	}
	return errors.Join(result, shutdownErr)
}

// helperHandler exposes only a minimal process-health signal alongside the
// authenticated encrypted command endpoint. It does not disclose account or
// queue state.
func helperHandler(store *scheduledsend.Store, syncEnabled bool) http.Handler {
	serviceName := "send-later"
	if syncEnabled {
		serviceName = "mail-service"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Status           string `json:"status"`
			Service          string `json:"service"`
			ProtocolVersion  int    `json:"protocolVersion"`
			APIVersion       string `json:"apiVersion"`
			SchemaVersion    int    `json:"schemaVersion"`
			MaxSchemaVersion int    `json:"maxSchemaVersion"`
		}{
			Status:           "ok",
			Service:          serviceName,
			ProtocolVersion:  servicebootstrap.ProtocolVersion,
			APIVersion:       v1.Version,
			SchemaVersion:    database.MaxSchemaVersion(),
			MaxSchemaVersion: database.MaxSchemaVersion(),
		})
	})
	mux.Handle("/v1/commands", store.Handler())
	return mux
}
