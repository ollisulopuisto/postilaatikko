// postilaatikko-apibench measures helper mailbox API latency, request counts,
// transferred bytes, and process CPU/RSS against a synthetic mailbox fixture.
//
// It exists for PERF-SC-001 baselines: run it before and after a candidate
// optimization to get comparable p50/p95 numbers. The fixture is synthetic,
// so results quantify local SQL/HTTP/serialization overhead only; provider
// latency is absent by construction and desktop paint/WebKit costs are not
// covered (see docs/performance/).
//
// Example:
//
//	go run ./cmd/postilaatikko-apibench -messages 20000 -iterations 15 \
//	  -report /tmp/baseline.json
//
// Add -rtt 40ms to simulate a higher-latency link between desktop and helper.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ollisulopuisto/postilaatikko/internal/alwayson"
	"github.com/ollisulopuisto/postilaatikko/internal/automation"
	"github.com/ollisulopuisto/postilaatikko/internal/automation/client"
	v1 "github.com/ollisulopuisto/postilaatikko/internal/automation/v1"
	"github.com/ollisulopuisto/postilaatikko/internal/database"
	"github.com/ollisulopuisto/postilaatikko/internal/imap"
	"github.com/ollisulopuisto/postilaatikko/internal/message"
	"github.com/ollisulopuisto/postilaatikko/internal/views"
)

type fixtureConfig struct {
	dataDir    string
	accounts   int
	messages   int
	threadSize int
}

type fixture struct {
	cfg        fixtureConfig
	accountIDs []string
	inboxIDs   map[string]string
	threadIDs  map[string][]string
}

// seedFixture builds a synthetic mailbox database. Subjects repeat a small
// vocabulary so search results are realistic; every nineteenth message is an
// "invoice" so the search scenario matches about five percent of the mail
// in both the Inbox and Sent folders.
func seedFixture(cfg fixtureConfig) (*fixture, *database.DB, error) {
	db, err := database.Open(filepath.Join(cfg.dataDir, "aerion.db"))
	if err != nil {
		return nil, nil, fmt.Errorf("open fixture database: %w", err)
	}
	if err := db.Migrate(); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("migrate fixture database: %w", err)
	}

	fx := &fixture{cfg: cfg, inboxIDs: map[string]string{}, threadIDs: map[string][]string{}}
	store := message.NewStore(db)
	base := time.Now().Add(-400 * 24 * time.Hour).UTC()
	uid := uint32(1)

	perAccount := cfg.messages / cfg.accounts
	for a := 0; a < cfg.accounts; a++ {
		accountID := fmt.Sprintf("acc-%d", a+1)
		fx.accountIDs = append(fx.accountIDs, accountID)
		if _, err := db.Exec(
			`INSERT INTO accounts (id, name, email, imap_host, smtp_host, username) VALUES (?, ?, ?, ?, ?, ?)`,
			accountID, fmt.Sprintf("Bench %d", a+1), fmt.Sprintf("bench%d@example.com", a+1),
			"imap.example.com", "smtp.example.com", "bench"); err != nil {
			_ = db.Close()
			return nil, nil, fmt.Errorf("seed account: %w", err)
		}
		for _, f := range []struct{ id, name, path, kind string }{
			{"inbox", "Inbox", "INBOX", "inbox"},
			{"sent", "Sent", "Sent", "sent"},
			{"archive", "Archive", "Archive", "archive"},
			{"drafts", "Drafts", "Drafts", "drafts"},
			{"trash", "Trash", "Trash", "trash"},
		} {
			folderID := accountID + "-" + f.id
			if _, err := db.Exec(
				`INSERT INTO folders (id, account_id, name, path, folder_type) VALUES (?, ?, ?, ?, ?)`,
				folderID, accountID, f.name, f.path, f.kind); err != nil {
				_ = db.Close()
				return nil, nil, fmt.Errorf("seed folder: %w", err)
			}
			if f.id == "inbox" {
				fx.inboxIDs[accountID] = folderID
			}
		}

		for i := 0; i < perAccount; i++ {
			threadNo := i / cfg.threadSize
			threadID := fmt.Sprintf("thread-%d", threadNo)
			if i%cfg.threadSize == 0 {
				fx.threadIDs[accountID] = append(fx.threadIDs[accountID], threadID)
			}
			folderID := fx.inboxIDs[accountID]
			if i%10 == 9 {
				folderID = accountID + "-sent"
			}
			subject := fmt.Sprintf("Status update %d", i)
			// Every 19th message is an invoice: prime cadence so invoices
			// land in Sent too (every 10th message goes to Sent), not only
			// in the Inbox.
			if i%19 == 0 {
				subject = fmt.Sprintf("Invoice #%d for services", i)
			}
			m := &message.Message{
				ID:          fmt.Sprintf("%s-msg-%d", accountID, i),
				AccountID:   accountID,
				FolderID:    folderID,
				UID:         uid,
				MessageID:   fmt.Sprintf("<msg-%d-%d@bench.local>", a, i),
				ThreadID:    threadID,
				Subject:     subject,
				FromName:    fmt.Sprintf("Sender %d", i%97),
				FromEmail:   fmt.Sprintf("sender%d@example.net", i%97),
				ToList:      fmt.Sprintf(`["bench%d@example.com"]`, a+1),
				Date:        base.Add(time.Duration(i) * 30 * time.Minute),
				Snippet:     fmt.Sprintf("Snippet preview text for message %d in the benchmark fixture.", i),
				IsRead:      i%7 != 0,
				Size:        4096 + i%8192,
				BodyFetched: false,
			}
			uid++
			if err := store.Create(m); err != nil {
				_ = db.Close()
				return nil, nil, fmt.Errorf("seed message %d: %w", i, err)
			}
		}
	}
	return fx, db, nil
}

// meterTransport counts requests and response bytes and can inject per-request
// latency to simulate a higher-RTT desktop/helper link.
type meterTransport struct {
	base  http.RoundTripper
	delay time.Duration

	requests      atomic.Int64
	requestBytes  atomic.Int64
	responseBytes atomic.Int64
}

func (t *meterTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.delay > 0 {
		time.Sleep(t.delay)
	}
	t.requests.Add(1)
	if req.ContentLength > 0 {
		t.requestBytes.Add(req.ContentLength)
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	resp.Body = &countingReadCloser{rc: resp.Body, counter: &t.responseBytes}
	return resp, nil
}

type countingReadCloser struct {
	rc      io.ReadCloser
	counter *atomic.Int64
}

func (c *countingReadCloser) Read(p []byte) (int, error) {
	n, err := c.rc.Read(p)
	if n > 0 {
		c.counter.Add(int64(n))
	}
	return n, err
}

func (c *countingReadCloser) Close() error { return c.rc.Close() }

type scenarioStats struct {
	Name          string  `json:"name"`
	Iterations    int     `json:"iterations"`
	MinMS         float64 `json:"min_ms"`
	P50MS         float64 `json:"p50_ms"`
	P95MS         float64 `json:"p95_ms"`
	MaxMS         float64 `json:"max_ms"`
	MeanMS        float64 `json:"mean_ms"`
	Requests      int64   `json:"requests"`
	RequestBytes  int64   `json:"request_bytes"`
	ResponseBytes int64   `json:"response_bytes"`
}

type report struct {
	Tool        string `json:"tool"`
	StartedAt   string `json:"started_at"`
	GitRevision string `json:"git_revision,omitempty"`
	GitModified bool   `json:"git_modified,omitempty"`
	Fixture     struct {
		Accounts   int `json:"accounts"`
		Messages   int `json:"messages"`
		ThreadSize int `json:"thread_size"`
	} `json:"fixture"`
	InjectedRTT string          `json:"injected_rtt"`
	Scenarios   []scenarioStats `json:"scenarios"`
	Idle        idleStats       `json:"idle"`
}

type idleStats struct {
	DurationS  float64 `json:"duration_s"`
	CPUPercent float64 `json:"cpu_percent"`
	RSSKBStart int64   `json:"rss_kb_start"`
	RSSKBEnd   int64   `json:"rss_kb_end"`
	Note       string  `json:"note,omitempty"`
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p*float64(len(sorted)-1) + 0.5)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func summarize(name string, samples []float64, reqBefore, reqB, respB [2]int64) scenarioStats {
	sorted := append([]float64(nil), samples...)
	sort.Float64s(sorted)
	var sum float64
	for _, s := range sorted {
		sum += s
	}
	stats := scenarioStats{
		Name:          name,
		Iterations:    len(samples),
		Requests:      reqBefore[1] - reqBefore[0],
		RequestBytes:  reqB[1] - reqB[0],
		ResponseBytes: respB[1] - respB[0],
	}
	if len(sorted) > 0 {
		stats.MinMS = sorted[0]
		stats.MaxMS = sorted[len(sorted)-1]
		stats.P50MS = percentile(sorted, 0.50)
		stats.P95MS = percentile(sorted, 0.95)
		stats.MeanMS = sum / float64(len(sorted))
	}
	return stats
}

// procSample holds process CPU and RSS read from /proc (Linux only).
type procSample struct {
	cpuJiffies int64
	rssKB      int64
	ok         bool
}

func readProc() procSample {
	if runtime.GOOS != "linux" {
		return procSample{}
	}
	var sample procSample
	if data, err := os.ReadFile("/proc/self/stat"); err == nil {
		// Fields after comm (which may contain spaces): state is field 3,
		// utime 14, stime 15.
		if idx := strings.LastIndexByte(string(data), ')'); idx >= 0 {
			fields := strings.Fields(string(data)[idx+1:])
			if len(fields) >= 13 {
				utime, utimeErr := strconv.ParseInt(fields[11], 10, 64)
				stime, stimeErr := strconv.ParseInt(fields[12], 10, 64)
				if utimeErr == nil && stimeErr == nil {
					sample.cpuJiffies = utime + stime
					sample.ok = true
				}
			}
		}
	}
	if data, err := os.ReadFile("/proc/self/status"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if rest, ok := strings.CutPrefix(line, "VmRSS:"); ok {
				fields := strings.Fields(rest)
				if len(fields) > 0 {
					if rss, err := strconv.ParseInt(fields[0], 10, 64); err == nil {
						sample.rssKB = rss
					}
				}
			}
		}
	}
	return sample
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "apibench:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("apibench", flag.ContinueOnError)
	dataDir := flags.String("data", "", "fixture directory (default: a temporary directory)")
	accounts := flags.Int("accounts", 2, "number of fixture accounts")
	messages := flags.Int("messages", 20000, "total fixture messages across accounts")
	threadSize := flags.Int("thread-size", 4, "average messages per thread")
	iterations := flags.Int("iterations", 15, "measured iterations per scenario")
	warmup := flags.Int("warmup", 2, "warmup iterations per scenario")
	pageSize := flags.Int("page-size", 50, "conversation page size")
	rtt := flags.Duration("rtt", 0, "extra latency injected per API request")
	idleDuration := flags.Duration("idle", 3*time.Second, "idle CPU/RSS sampling window")
	reportPath := flags.String("report", "", "write the JSON report to this path")
	keep := flags.Bool("keep", false, "keep the fixture directory after the run")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *accounts < 1 || *messages < 100 || *threadSize < 1 {
		return errors.New("need at least 1 account, 100 messages and thread size 1")
	}

	cleanup := false
	if *dataDir == "" {
		dir, err := os.MkdirTemp("", "apibench-fixture-*")
		if err != nil {
			return err
		}
		*dataDir = dir
		cleanup = !*keep
	}
	if cleanup {
		defer os.RemoveAll(*dataDir)
	}

	fmt.Fprintf(os.Stderr, "Seeding %d messages into %d account(s) at %s ...\n", *messages, *accounts, *dataDir)
	seedStart := time.Now()
	fx, db, err := seedFixture(fixtureConfig{dataDir: *dataDir, accounts: *accounts, messages: *messages, threadSize: *threadSize})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Fixture ready in %s.\n", time.Since(seedStart).Round(time.Millisecond))

	tokenStore := automation.NewSQLiteTokenStore(db)
	grant, err := tokenStore.RegisterClient("apibench", []v1.Capability{
		v1.CapabilityMailRead,
		v1.CapabilityMailBodyFetch,
	})
	if err != nil {
		return fmt.Errorf("register benchmark client: %w", err)
	}

	noCredentials := func(context.Context, string) (*imap.ClientConfig, error) {
		return nil, errors.New("apibench fixture has no provider credentials")
	}
	rt, err := alwayson.OpenSyncRuntime(*dataDir, noCredentials)
	if err != nil {
		return fmt.Errorf("open sync runtime: %w", err)
	}
	defer func() { _ = rt.Close() }()

	apiServer, err := rt.NewMailboxAPIServer(automation.ServerConfig{
		Addr:       "127.0.0.1:0",
		TokenStore: tokenStore,
	})
	if err != nil {
		return fmt.Errorf("compose API server: %w", err)
	}
	if err := apiServer.Start(); err != nil {
		return fmt.Errorf("start API server: %w", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = apiServer.Stop(ctx)
	}()

	meter := &meterTransport{delay: *rtt}
	api := client.New(client.Config{
		Endpoint:  apiServer.URL(),
		Token:     grant.Token,
		Transport: meter,
	})

	ctx := context.Background()
	firstAccount := fx.accountIDs[0]
	firstInbox := fx.inboxIDs[firstAccount]
	// Deep paging must land on a populated conversation page. The cursor is
	// an offset into grouped conversations, not messages, so derive it from
	// the fixture's inbox conversation count (90% of messages, grouped by
	// threadSize).
	perAccount := *messages / *accounts
	inboxConversations := (perAccount * 9 / 10) / *threadSize
	deepOffset := inboxConversations / 2
	threadCursor := 0

	folderScope := views.Scope{Kind: views.ScopeFolder, AccountID: firstAccount, FolderID: firstInbox}

	scenarios := []struct {
		name string
		run  func(context.Context) error
	}{
		{
			name: "startup_waterfall",
			run: func(ctx context.Context) error {
				if _, err := api.GetCapabilities(ctx); err != nil {
					return err
				}
				if _, err := api.GetAccounts(ctx); err != nil {
					return err
				}
				if _, err := api.GetFolders(ctx, firstAccount); err != nil {
					return err
				}
				if _, err := api.GetViews(ctx); err != nil {
					return err
				}
				if _, err := api.VirtualFolderUnreadCounts(ctx); err != nil {
					return err
				}
				_, err := api.ListConversations(ctx, v1.ConversationListRequest{
					Scope: folderScope,
					Page:  v1.PageRequest{Limit: *pageSize},
				})
				return err
			},
		},
		{
			// PERF-SC-003: the same first-paint sections through the
			// mailbox snapshot plus the same capability discovery as the waterfall.
			// Older helpers without the snapshot fail this measurement explicitly.
			name: "startup_bootstrap",
			run: func(ctx context.Context) error {
				if _, err := api.GetCapabilities(ctx); err != nil {
					return err
				}
				resp, err := api.BootstrapWorkspace(ctx, v1.WorkspaceBootstrapRequest{
					FoldersAccountID: firstAccount,
					InitialPage: &v1.ConversationListRequest{
						Scope: folderScope,
						Page:  v1.PageRequest{Limit: *pageSize},
					},
				})
				if client.IsHTTPNotFound(err) {
					return errors.New("helper does not expose /v1/workspace/bootstrap")
				}
				if err != nil {
					return err
				}
				if len(resp.Errors) != 0 {
					return fmt.Errorf("bootstrap section errors: %v", resp.Errors)
				}
				if resp.InitialPage == nil {
					return errors.New("bootstrap returned no initial page")
				}
				return nil
			},
		},
		{
			name: "folder_page_first",
			run: func(ctx context.Context) error {
				page, err := api.ListConversations(ctx, v1.ConversationListRequest{
					Scope: folderScope,
					Page:  v1.PageRequest{Limit: *pageSize},
				})
				if err != nil {
					return err
				}
				if len(page.Items) == 0 {
					return errors.New("first page is empty; the scenario would measure nothing")
				}
				return nil
			},
		},
		{
			name: "folder_page_deep_offset",
			run: func(ctx context.Context) error {
				page, err := api.ListConversations(ctx, v1.ConversationListRequest{
					Scope: folderScope,
					Page:  v1.PageRequest{Limit: *pageSize, Cursor: fmt.Sprintf("%d", deepOffset)},
				})
				if err != nil {
					return err
				}
				if len(page.Items) == 0 {
					return fmt.Errorf("deep page at offset %d is empty; the scenario would measure nothing", deepOffset)
				}
				return nil
			},
		},
		{
			name: "folder_count",
			run: func(ctx context.Context) error {
				_, err := api.CountConversations(ctx, v1.ConversationCountRequest{Scope: folderScope})
				return err
			},
		},
		{
			name: "unified_inbox_page",
			run: func(ctx context.Context) error {
				page, err := api.ListConversations(ctx, v1.ConversationListRequest{
					Scope: views.Scope{Kind: views.ScopeAllInboxes},
					Page:  v1.PageRequest{Limit: *pageSize},
				})
				if err != nil {
					return err
				}
				if len(page.Items) == 0 {
					return errors.New("unified inbox page is empty; the scenario would measure nothing")
				}
				return nil
			},
		},
		{
			name: "conversation_open",
			run: func(ctx context.Context) error {
				threads := fx.threadIDs[firstAccount]
				if len(threads) == 0 {
					return errors.New("fixture has no threads")
				}
				threadID := threads[threadCursor%len(threads)]
				threadCursor++
				view, err := api.ReadThread(ctx, v1.ThreadReadRequest{
					Ref:   views.ThreadRef{AccountID: firstAccount, ThreadID: threadID},
					Scope: folderScope,
				})
				if err != nil {
					return err
				}
				if len(view.Messages) == 0 {
					return errors.New("thread view has no messages; the scenario would measure nothing")
				}
				return nil
			},
		},
		{
			// Indexed header search over the local cache. The online
			// /v1/search path is deliberately not measured: with a synthetic
			// fixture it degenerates into provider credential failures,
			// which say nothing about local search cost.
			name: "search_invoice",
			run: func(ctx context.Context) error {
				page, err := api.SearchUnifiedFolder(ctx, v1.ConversationSearchRequest{
					// Unified-folder search supports system folders; the
					// fixture's Sent folder carries the same 5% invoice
					// minority as the Inbox.
					Scope: views.Scope{Kind: views.ScopeUnifiedFolder, FolderType: "sent"},
					Query: "invoice",
					Limit: *pageSize,
				})
				if err != nil {
					return err
				}
				if len(page.Results) == 0 {
					return errors.New("search returned no results; the scenario would measure nothing")
				}
				return nil
			},
		},
		{
			name: "concurrent_identical_reads_x8",
			run: func(ctx context.Context) error {
				var wg sync.WaitGroup
				errs := make(chan error, 8)
				for i := 0; i < 8; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						_, err := api.ListConversations(ctx, v1.ConversationListRequest{
							Scope: folderScope,
							Page:  v1.PageRequest{Limit: *pageSize},
						})
						errs <- err
					}()
				}
				wg.Wait()
				close(errs)
				for err := range errs {
					if err != nil {
						return err
					}
				}
				return nil
			},
		},
	}

	rep := report{Tool: "postilaatikko-apibench", StartedAt: time.Now().UTC().Format(time.RFC3339)}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				rep.GitRevision = setting.Value
			case "vcs.modified":
				rep.GitModified = setting.Value == "true"
			}
		}
	}
	rep.Fixture.Accounts = *accounts
	rep.Fixture.Messages = *messages
	rep.Fixture.ThreadSize = *threadSize
	rep.InjectedRTT = rtt.String()

	for _, scenario := range scenarios {
		for i := 0; i < *warmup; i++ {
			if err := scenario.run(ctx); err != nil {
				return fmt.Errorf("scenario %s warmup: %w", scenario.name, err)
			}
		}
		reqBefore := meter.requests.Load()
		reqBBefore := meter.requestBytes.Load()
		respBBefore := meter.responseBytes.Load()
		samples := make([]float64, 0, *iterations)
		for i := 0; i < *iterations; i++ {
			start := time.Now()
			if err := scenario.run(ctx); err != nil {
				return fmt.Errorf("scenario %s iteration %d: %w", scenario.name, i, err)
			}
			samples = append(samples, float64(time.Since(start).Microseconds())/1000.0)
		}
		stats := summarize(scenario.name, samples,
			[2]int64{reqBefore, meter.requests.Load()},
			[2]int64{reqBBefore, meter.requestBytes.Load()},
			[2]int64{respBBefore, meter.responseBytes.Load()})
		rep.Scenarios = append(rep.Scenarios, stats)
		fmt.Fprintf(os.Stderr, "%-28s p50 %8.2f ms  p95 %8.2f ms  requests %d\n",
			stats.Name, stats.P50MS, stats.P95MS, stats.Requests)
	}

	// Idle CPU/RSS: no traffic during the sampling window.
	idleStart := readProc()
	idleWall := time.Now()
	time.Sleep(*idleDuration)
	idleEnd := readProc()
	rep.Idle = idleStats{
		DurationS:  idleDuration.Seconds(),
		RSSKBStart: idleStart.rssKB,
		RSSKBEnd:   idleEnd.rssKB,
	}
	if idleStart.ok && idleEnd.ok {
		const clockTicksPerSecond = 100
		cpuSeconds := float64(idleEnd.cpuJiffies-idleStart.cpuJiffies) / clockTicksPerSecond
		rep.Idle.CPUPercent = 100 * cpuSeconds / time.Since(idleWall).Seconds()
	} else {
		rep.Idle.Note = "CPU sampling requires Linux /proc; RSS may be zero on other platforms"
	}
	fmt.Fprintf(os.Stderr, "idle: cpu %.2f%%  rss %d -> %d kB\n", rep.Idle.CPUPercent, rep.Idle.RSSKBStart, rep.Idle.RSSKBEnd)

	if *reportPath != "" {
		data, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(*reportPath, append(data, '\n'), 0o644); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Report written to %s\n", *reportPath)
	}
	return nil
}
