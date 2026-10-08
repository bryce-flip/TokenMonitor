package rpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"TOkenMonitor/internal/config"
)

const testUSDTContract = "0xdAC17F958D2ee523a2206206994597C13D831ec7"

func testConfig() *config.Config {
	return &config.Config{
		ChainID:            1,
		ConfirmationBlocks: 20,
		Tokens: []config.Token{
			{Symbol: "USDT", Issuer: "Tether", Contract: testUSDTContract, Decimals: 6},
		},
	}
}

// cannedRPC answers every JSON-RPC POST with fixed eth_chainId /
// eth_blockNumber results, an empty eth_getLogs result, and optional
// injected eth_getLogs failures: a permanent one (getLogsFailed) or an
// initial run of throttled responses (getLogs429s) after which it succeeds.
type cannedRPC struct {
	chainID       string
	head          string
	getLogsFailed bool
	getLogs429s   int32        // throttle the first N eth_getLogs with -32005
	getLogsCalls  atomic.Int32 // total eth_getLogs attempts served
}

func (c *cannedRPC) start(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		resp := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		switch req.Method {
		case "eth_chainId":
			resp["result"] = c.chainID
		case "eth_blockNumber":
			resp["result"] = c.head
		case "eth_getLogs":
			n := c.getLogsCalls.Add(1)
			if c.getLogsFailed {
				resp["error"] = map[string]any{"code": -32602, "message": "Archive requests require a personal token"}
				break
			}
			if n <= c.getLogs429s {
				resp["error"] = map[string]any{"code": -32005, "message": "Too Many Requests"}
				break
			}
			resp["result"] = []any{}
		default:
			resp["result"] = nil
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestProbeRejectsWrongChain proves D-02's network guard: a provider on chain
// id 2 aborts before any ingest.
func TestProbeRejectsWrongChain(t *testing.T) {
	canned := cannedRPC{chainID: "0x2", head: "0x3e8"}
	srv := canned.start(t)
	c, err := New(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	err = c.Probe(context.Background(), testConfig(), 100, 200)
	if err == nil {
		t.Fatal("expected probe failure on chain id 2")
	}
	if !strings.Contains(err.Error(), "chain id 2") {
		t.Errorf("error %q does not name the actual chain id", err.Error())
	}
}

// TestProbeAcceptsConfirmedRangeAndFetchReturnsZeroRows proves the D-02 happy
// path offline: chain id 1, an in-range to-block, and a working (empty)
// sample eth_getLogs pass the probe; the fetch path then returns zero rows
// without error.
func TestProbeAcceptsConfirmedRangeAndFetchReturnsZeroRows(t *testing.T) {
	canned := cannedRPC{chainID: "0x1", head: "0x3e8"} // head 1000, conf 20
	srv := canned.start(t)
	c, err := New(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	cfg := testConfig()
	if err := c.Probe(context.Background(), cfg, 100, 980); err != nil {
		t.Fatalf("probe at exactly head-confirmation_blocks: %v", err)
	}
	logs, err := c.FetchLogs(context.Background(), cfg.Tokens[0], nil, 100, 980)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("expected zero logs from empty canned response, got %d", len(logs))
	}
}

// TestProbeConfirmationBoundaryExactness pins the confirmation policy edge:
// to == head-confirmation_blocks is accepted; to one block past it is
// rejected with an error naming the policy.
func TestProbeConfirmationBoundaryExactness(t *testing.T) {
	canned := cannedRPC{chainID: "0x1", head: "0x3e8"} // head 1000
	srv := canned.start(t)
	c, err := New(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	cfg := testConfig() // confirmation_blocks 20 -> eligible through 980

	if err := c.Probe(context.Background(), cfg, 100, 980); err != nil {
		t.Fatalf("to == head-confirmation_blocks must be accepted: %v", err)
	}
	err = c.Probe(context.Background(), cfg, 100, 981)
	if err == nil {
		t.Fatal("to one block past head-confirmation_blocks must be rejected")
	}
	if !strings.Contains(err.Error(), "confirmation_blocks") {
		t.Errorf("error %q does not name confirmation_blocks", err.Error())
	}
}

// TestProbeRejectsProviderWithoutHistoricalLogs aborts when the sample
// eth_getLogs at the from-block fails (D-02: history-limited providers).
func TestProbeRejectsProviderWithoutHistoricalLogs(t *testing.T) {
	canned := cannedRPC{chainID: "0x1", head: "0x3e8", getLogsFailed: true}
	srv := canned.start(t)
	c, err := New(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	err = c.Probe(context.Background(), testConfig(), 100, 200)
	if err == nil {
		t.Fatal("expected probe failure when sample eth_getLogs errors")
	}
	if !strings.Contains(err.Error(), "sample eth_getLogs") {
		t.Errorf("error %q does not name the sample eth_getLogs probe", err.Error())
	}
}

// TestFetchLogsRetriesRateLimits proves requirements.md §24 behavior
// (free-tier providers throttle bursty requests; observed live against the
// operator provider as HTTP 429 / JSON-RPC -32005): a throttled eth_getLogs
// is retried with bounded backoff instead of failing the bounded run.
func TestFetchLogsRetriesRateLimits(t *testing.T) {
	orig := rateLimitBackoffs
	rateLimitBackoffs = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	t.Cleanup(func() { rateLimitBackoffs = orig })

	canned := cannedRPC{chainID: "0x1", head: "0x3e8", getLogs429s: 2}
	srv := canned.start(t)
	c, err := New(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	logs, err := c.FetchLogs(context.Background(), testConfig().Tokens[0], nil, 100, 980)
	if err != nil {
		t.Fatalf("fetch must survive two throttled responses via retry: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("expected zero logs from the recovered canned response, got %d", len(logs))
	}
	if got := canned.getLogsCalls.Load(); got != 3 {
		t.Fatalf("eth_getLogs attempts = %d, want 3 (two throttled, one recovered)", got)
	}
}
