package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"

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
// injected eth_getLogs failures: a permanent one (getLogsFailed), an initial
// run of throttled responses (getLogs429s, message throttleMsg — default
// "Too Many Requests"), and a result-cap mode (capSpan) answering -32005
// with capMsg whenever the requested span exceeds capSpan, so halving
// observably shrinks the span (D-05 proofs).
type cannedRPC struct {
	chainID             string
	head                string
	getLogsFailed       bool
	getLogs429s         int32  // throttle the first N eth_getLogs with -32005
	throttleMsg         string // throttle error message; empty = "Too Many Requests"
	capSpan             uint64 // 0 = off; otherwise cap any eth_getLogs span > capSpan
	capMsg              string // cap error message; empty = "query returned more than 10000 results"
	getLogsCalls        atomic.Int32
	blockByNumberFailed bool // answer eth_getBlockByNumber with an error (tag-probe false path)

	mu    sync.Mutex
	spans [][2]uint64 // (fromBlock, toBlock) of every eth_getLogs served
	tags  []string    // first param of every eth_getBlockByNumber served
}

// hexU64 parses a 0x-prefixed JSON-RPC quantity.
func hexU64(s string) uint64 {
	n, err := strconv.ParseUint(strings.TrimPrefix(s, "0x"), 16, 64)
	if err != nil {
		return 0
	}
	return n
}

func (c *cannedRPC) recordSpan(from, to uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.spans = append(c.spans, [2]uint64{from, to})
}

// ranges returns every (fromBlock, toBlock) eth_getLogs pair served, in order.
func (c *cannedRPC) ranges() [][2]uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([][2]uint64(nil), c.spans...)
}

// blockTags returns the first param of every eth_getBlockByNumber served.
func (c *cannedRPC) blockTags() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.tags...)
}

func (c *cannedRPC) start(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage   `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
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
		case "eth_getBlockByNumber":
			var tag string
			if len(req.Params) > 0 && json.Unmarshal(req.Params[0], &tag) == nil {
				c.mu.Lock()
				c.tags = append(c.tags, tag)
				c.mu.Unlock()
			}
			if c.blockByNumberFailed {
				resp["error"] = map[string]any{"code": -32601, "message": "method not supported"}
				break
			}
			// A null result makes ethclient.HeaderByNumber return a not-found
			// error, so serve a real (RLP-consistent) header object.
			b, err := json.Marshal(&types.Header{
				UncleHash:   types.EmptyUncleHash,
				TxHash:      types.EmptyRootHash,
				ReceiptHash: types.EmptyRootHash,
				Difficulty:  big.NewInt(0),
				Number:      big.NewInt(1),
				GasLimit:    30_000_000,
				Time:        1,
			})
			if err != nil {
				http.Error(w, "marshal header", http.StatusInternalServerError)
				return
			}
			resp["result"] = json.RawMessage(b)
		case "eth_getLogs":
			var q struct {
				FromBlock string `json:"fromBlock"`
				ToBlock   string `json:"toBlock"`
			}
			if len(req.Params) == 0 || json.Unmarshal(req.Params[0], &q) != nil {
				resp["error"] = map[string]any{"code": -32602, "message": "bad params"}
				break
			}
			from, to := hexU64(q.FromBlock), hexU64(q.ToBlock)
			c.recordSpan(from, to)
			n := c.getLogsCalls.Add(1)
			if c.getLogsFailed {
				resp["error"] = map[string]any{"code": -32602, "message": "Archive requests require a personal token"}
				break
			}
			if n <= c.getLogs429s {
				msg := c.throttleMsg
				if msg == "" {
					msg = "Too Many Requests"
				}
				resp["error"] = map[string]any{"code": -32005, "message": msg}
				break
			}
			if c.capSpan != 0 && to-from+1 > c.capSpan {
				msg := c.capMsg
				if msg == "" {
					msg = "query returned more than 10000 results"
				}
				resp["error"] = map[string]any{"code": -32005, "message": msg}
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

// spansOf maps recorded ranges to their block spans.
func spansOf(ranges [][2]uint64) []uint64 {
	out := make([]uint64, len(ranges))
	for i, r := range ranges {
		out[i] = r[1] - r[0] + 1
	}
	return out
}

// assertExactTiling verifies the accepted sub-ranges (those within capSpan of
// capThreshold) cover [from, to] exactly once each: no gap, no overlap.
func assertExactTiling(t *testing.T, ranges [][2]uint64, capThreshold, from, to uint64) {
	t.Helper()
	var accepted [][2]uint64
	for _, r := range ranges {
		if r[1]-r[0]+1 <= capThreshold {
			accepted = append(accepted, r)
		}
	}
	if len(accepted) == 0 {
		t.Fatalf("no accepted sub-ranges recorded in %+v", ranges)
	}
	cur := from
	for _, r := range accepted {
		if r[0] != cur {
			t.Fatalf("accepted sub-range %+v does not start at %d (gap or overlap in %+v)", r, cur, accepted)
		}
		cur = r[1] + 1
	}
	if cur != to+1 {
		t.Fatalf("accepted sub-ranges cover through %d, want exactly %d", cur-1, to)
	}
}

// TestResultCapHalvesRangeAndSucceeds proves D-05's deterministic halving:
// a -32005 "query returned more than 10000 results" on a 1000-block fetch
// halves the sub-range and retries the SAME starting block immediately
// (spans descend 1000 -> 500 -> 250 with no backoff pacing), and the
// accepted sub-ranges tile [100, 1099] exactly — no gap, no overlap.
func TestResultCapHalvesRangeAndSucceeds(t *testing.T) {
	canned := &cannedRPC{chainID: "0x1", head: "0x3e8", capSpan: 200}
	srv := canned.start(t)
	c, err := New(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	logs, err := c.FetchLogsResilient(context.Background(), testConfig().Tokens[0], nil, 100, 1099, 1000, 10)
	if err != nil {
		t.Fatalf("resilient fetch must succeed after halving: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("expected zero logs from canned server, got %d", len(logs))
	}
	ranges := canned.ranges()
	spans := spansOf(ranges)
	if len(spans) < 4 || spans[0] != 1000 || spans[1] != 500 || spans[2] != 250 {
		t.Fatalf("recorded spans = %v, want the head to descend 1000 -> 500 -> 250 (then succeed below the cap)", spans)
	}
	for i := 1; i <= 2; i++ {
		if ranges[i][0] != ranges[0][0] {
			t.Fatalf("halving retried a different start: %+v vs %+v", ranges[i], ranges[0])
		}
	}
	assertExactTiling(t, ranges, 200, 100, 1099)
}

// TestQueryTimeoutTreatedAsResultCap proves the second -32005 cap shape
// ("query timeout exceeded") follows the halving path, not backoff: the
// attempt count matches the halving schedule exactly (3 capped attempts +
// 8 accepted sub-ranges = 11) and no span ever repeats at the throttled
// size — a same-range backoff retry would re-log the identical span.
func TestQueryTimeoutTreatedAsResultCap(t *testing.T) {
	canned := &cannedRPC{chainID: "0x1", head: "0x3e8", capSpan: 200, capMsg: "query timeout exceeded"}
	srv := canned.start(t)
	c, err := New(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	logs, err := c.FetchLogsResilient(context.Background(), testConfig().Tokens[0], nil, 100, 1099, 1000, 10)
	if err != nil {
		t.Fatalf("timeout-shaped cap must halve to success, not exhaust backoff: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("expected zero logs, got %d", len(logs))
	}
	ranges := canned.ranges()
	spans := spansOf(ranges)
	want := []uint64{1000, 500, 250, 125, 125, 125, 125, 125, 125, 125, 125}
	if len(spans) != len(want) {
		t.Fatalf("eth_getLogs attempts = %d (spans %v), want %d (spans %v): halving schedule, not backoff pacing", len(spans), spans, len(want), want)
	}
	for i, s := range want {
		if spans[i] != s {
			t.Fatalf("span schedule = %v, want %v (same-range backoff retries never repeat a shrinking schedule)", spans, want)
		}
	}
	assertExactTiling(t, ranges, 200, 100, 1099)
}

// TestThrottleBacksOffSameRange proves the throttle classification keeps
// the SAME range on every attempt (SYNC-02: retried, not skipped): two
// -32005 "Too Many Requests" responses then success, with bounded backoff
// (shortened for the test) between attempts — three identical requests.
func TestThrottleBacksOffSameRange(t *testing.T) {
	orig := rateLimitBackoffs
	rateLimitBackoffs = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	t.Cleanup(func() { rateLimitBackoffs = orig })

	canned := &cannedRPC{chainID: "0x1", head: "0x3e8", getLogs429s: 2}
	srv := canned.start(t)
	c, err := New(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	logs, err := c.FetchLogsResilient(context.Background(), testConfig().Tokens[0], nil, 100, 980, 1000, 10)
	if err != nil {
		t.Fatalf("throttled fetch must survive bounded backoff: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("expected zero logs, got %d", len(logs))
	}
	if got := canned.getLogsCalls.Load(); got != 3 {
		t.Fatalf("eth_getLogs attempts = %d, want 3 (two throttled, one recovered)", got)
	}
	for _, r := range canned.ranges() {
		if r[0] != 100 || r[1] != 980 {
			t.Fatalf("throttle retried a different range: %+v, want [100 980] on every attempt", r)
		}
	}
}

// TestFloorTripFailsClosed proves the SYNC-02 boundary: a result cap that
// persists at window_floor fails the run with a named error identifying the
// floor and the blocked sub-range — the fetch never silently skips ahead.
func TestFloorTripFailsClosed(t *testing.T) {
	canned := &cannedRPC{chainID: "0x1", head: "0x3e8", capSpan: 1} // every span caps
	srv := canned.start(t)
	c, err := New(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	logs, err := c.FetchLogsResilient(context.Background(), testConfig().Tokens[0], nil, 100, 1099, 1000, 10)
	if err == nil {
		t.Fatal("persistent cap at the floor must fail closed, not succeed")
	}
	if logs != nil {
		t.Fatalf("failed fetch reported %d logs, want nil (no partial advance)", len(logs))
	}
	msg := err.Error()
	if !strings.Contains(msg, "window_floor 10") {
		t.Errorf("error %q does not name window_floor 10", msg)
	}
	if !strings.Contains(msg, "blocks 100-106") {
		t.Errorf("error %q does not identify the blocked sub-range 100-106", msg)
	}
	// Halving descended all the way: 1000, 500, 250, 125, 62, 31, 15, 7.
	if spans := spansOf(canned.ranges()); len(spans) != 8 || spans[7] != 7 {
		t.Fatalf("recorded spans = %v, want the halving descent to end at span 7", spans)
	}
}

// TestRateLimitExceededMessageIsThrottle proves the live-observed
// eth.merkle.io shape (-32005 "Rate limit exceeded") routes to backoff, not
// halving: every retry carries the identical range and the call recovers.
func TestRateLimitExceededMessageIsThrottle(t *testing.T) {
	orig := rateLimitBackoffs
	rateLimitBackoffs = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	t.Cleanup(func() { rateLimitBackoffs = orig })

	canned := &cannedRPC{chainID: "0x1", head: "0x3e8", getLogs429s: 2, throttleMsg: "Rate limit exceeded"}
	srv := canned.start(t)
	c, err := New(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	logs, err := c.FetchLogsResilient(context.Background(), testConfig().Tokens[0], nil, 100, 980, 1000, 10)
	if err != nil {
		t.Fatalf("rate-limit-shaped throttle must recover via backoff: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("expected zero logs, got %d", len(logs))
	}
	if got := canned.getLogsCalls.Load(); got != 3 {
		t.Fatalf("eth_getLogs attempts = %d, want 3", got)
	}
	for _, r := range canned.ranges() {
		if r[0] != 100 || r[1] != 980 {
			t.Fatalf("throttle retried a different range: %+v, want [100 980] (halving would have shrunk it)", r)
		}
	}
}

// assertRedacted fails when err's text carries the endpoint's credentials:
// it must name the host (URL text survived, redacted) while carrying neither
// the userinfo pair nor the full credentialed URL.
func assertRedacted(t *testing.T, err error, credURL, userinfo string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "127.0.0.1") {
		t.Errorf("error %q does not name the host", msg)
	}
	if strings.Contains(msg, userinfo) {
		t.Errorf("error %q leaks the userinfo pair %q", msg, userinfo)
	}
	if strings.Contains(msg, credURL) {
		t.Errorf("error %q leaks the full credentialed URL", msg)
	}
	if strings.Contains(msg, "xxxxx@") || strings.Contains(msg, "secretpass") {
		t.Errorf("error %q leaks a masked-or-raw password form", msg)
	}
}

// TestWrappedErrorsRedactCredentials closes AR-01 / T-02-03: dialing through
// a userinfo-bearing URL and forcing transport failures on eth_getLogs and
// eth_getBlockByNumber yields %w-wrapped *url.Error chains — every returned
// error must render the endpoint as scheme://host only (net/http alone would
// keep "user:xxxxx@host" in the text).
func TestWrappedErrorsRedactCredentials(t *testing.T) {
	canned := &cannedRPC{chainID: "0x1", head: "0x3e8"}
	srv := canned.start(t)
	const userinfo = "user:secretpass@"
	credURL := strings.Replace(srv.URL, "http://", "http://"+userinfo, 1)
	c, err := New(context.Background(), credURL)
	if err != nil {
		assertRedacted(t, err, credURL, userinfo)
		t.Fatalf("dial through userinfo URL errored: %v", err)
	}
	srv.Close() // force connection-refused transport errors wrapping *url.Error

	_, err = c.FetchLogs(context.Background(), testConfig().Tokens[0], nil, 100, 200)
	assertRedacted(t, err, credURL, userinfo)
	_, err = c.Header(context.Background(), 100)
	assertRedacted(t, err, credURL, userinfo)
	var _ = c.SupportsHeadTag(context.Background()) // false path must not leak either
	// The scrub itself is deterministic on both leak shapes: the raw URL and
	// net/http's masked "user:xxxxx@" form.
	for _, leak := range []string{
		`post failed: Get "` + credURL + `": dial tcp: connection refused`,
		`post failed: Get "` + strings.Replace(credURL, "secretpass", "xxxxx", 1) + `": dial tcp: connection refused`,
	} {
		scrubbed := scrubURL(credURL, errors.New(leak))
		if scrubbed.Error() == leak {
			t.Errorf("scrubURL left %q untouched", leak)
		}
		if !strings.Contains(scrubbed.Error(), "http://127.0.0.1") {
			t.Errorf("scrubbed error %q lost the host rendering", scrubbed.Error())
		}
	}
	// The SIGINT contract survives redaction: a canceled transport error
	// still satisfies errors.Is(err, context.Canceled) after scrubbing.
	canceled := scrubURL(credURL, fmt.Errorf(`get: %w`, context.Canceled))
	if !errors.Is(canceled, context.Canceled) {
		t.Errorf("scrubbed cancellation lost its errors.Is contract: %v", canceled)
	}
	if errors.Is(scrubURL(credURL, errors.New("boom")), context.Canceled) {
		t.Error("non-canceled error must not masquerade as context.Canceled")
	}
}

// TestSupportsHeadTagTrueAndFalse proves the A1 probe: a provider answering
// eth_getBlockByNumber for the "finalized" tag (params carry the tag string)
// reports true; one erroring the method reports false without failing.
func TestSupportsHeadTagTrueAndFalse(t *testing.T) {
	canned := &cannedRPC{chainID: "0x1", head: "0x3e8"}
	srv := canned.start(t)
	c, err := New(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if !c.SupportsHeadTag(context.Background()) {
		t.Fatal("provider serving eth_getBlockByNumber must report tag support")
	}
	tags := canned.blockTags()
	if len(tags) == 0 || tags[len(tags)-1] != "finalized" {
		t.Fatalf("eth_getBlockByNumber params = %v, want the \"finalized\" tag string", tags)
	}

	canned2 := &cannedRPC{chainID: "0x1", head: "0x3e8", blockByNumberFailed: true}
	srv2 := canned2.start(t)
	c2, err := New(context.Background(), srv2.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if c2.SupportsHeadTag(context.Background()) {
		t.Fatal("provider erroring eth_getBlockByNumber must report no tag support")
	}
}

// TestCloseIsSafe proves IN-08's client half: Close on a dialed client
// returns without panic and a double Close is tolerated.
func TestCloseIsSafe(t *testing.T) {
	canned := &cannedRPC{chainID: "0x1", head: "0x3e8"}
	srv := canned.start(t)
	c, err := New(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	c.Close()
	c.Close() // tolerated
}
