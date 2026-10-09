// Offline suite for the continuous sync loop: a canned JSON-RPC server
// (local copy of the internal/rpc cannedRPC pattern, blessed by
// 02-PATTERNS.md) plus an in-memory indexer.EventStore fake prove the SYNC-01/03/04
// behaviors without any network or ClickHouse dependency.
package indexer_test

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
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"TOkenMonitor/internal/config"
	"TOkenMonitor/internal/indexer"
	"TOkenMonitor/internal/rpc"
	"TOkenMonitor/internal/storage"
)

// The seams are satisfied structurally: *rpc.Client is a indexer.ChainReader and
// *storage.Store is an indexer.EventStore (sync.go cannot import either package; the
// test binary can — no cycle).
var (
	_ indexer.ChainReader = (*rpc.Client)(nil)
	_ indexer.EventStore  = (*storage.Store)(nil)
)

const syncUSDTContract = "0xdAC17F958D2ee523a2206206994597C13D831ec7"

func syncUSDTToken() config.Token {
	return config.Token{Symbol: "USDT", Issuer: "Tether", Contract: syncUSDTContract, Decimals: 6}
}

// syncWord32 renders v as the exact 32-byte big-endian ABI word it occupies
// in non-indexed event data (local copy of decoder_test.go's word32 — the
// external test package cannot see same-package helpers).
func syncWord32(v *big.Int) []byte {
	b := make([]byte, 32)
	v.FillBytes(b)
	return b
}

func syncConfig() *config.Config {
	return &config.Config{
		ChainID:            1,
		ConfirmationBlocks: 20,
		HeadPolicy:         "finalized",
		WindowBlocks:       1000,
		WindowFloor:        100,
		PollSeconds:        1,
		Tokens:             []config.Token{syncUSDTToken()},
	}
}

// Header hashes are RLP-derived on unmarshal (go-ethereum v1.17.7 has no
// stored hash field), so the canned chain derives its per-height hash from a
// deterministic header — exactly how a real provider's headers behave. A
// timeOverride at height N yields a different canonical hash at N.
func cannonHeader(n, ts uint64) *types.Header {
	return &types.Header{
		UncleHash:   types.EmptyUncleHash,
		TxHash:      types.EmptyRootHash,
		ReceiptHash: types.EmptyRootHash,
		Difficulty:  big.NewInt(0),
		Number:      big.NewInt(int64(n)),
		GasLimit:    30_000_000,
		Time:        ts,
	}
}

func cannonTime(n uint64, timeOverride map[uint64]uint64) uint64 {
	if ts, ok := timeOverride[n]; ok {
		return ts
	}
	return 1700000000 + n
}

func cannonHash(n uint64, timeOverride map[uint64]uint64) string {
	return cannonHeader(n, cannonTime(n, timeOverride)).Hash().Hex()
}

// cannonRPC answers the three methods indexer.RunSync needs: a finalized head
// (eth_getBlockByNumber "finalized"), per-height headers, and a fixed log
// set filtered to the requested eth_getLogs range.
type cannonRPC struct {
	head         uint64
	logs         []types.Log
	timeOverride map[uint64]uint64

	mu          sync.Mutex
	getLogsFrom []uint64 // FromBlock of every eth_getLogs served
}

func hexNum(s string) uint64 {
	n, err := strconv.ParseUint(strings.TrimPrefix(s, "0x"), 16, 64)
	if err != nil {
		return 0
	}
	return n
}

func (c *cannonRPC) start(t *testing.T) *httptest.Server {
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
		case "eth_blockNumber":
			resp["result"] = "0x" + strconv.FormatUint(c.head, 16)
		case "eth_getBlockByNumber":
			var tag string
			if len(req.Params) == 0 || json.Unmarshal(req.Params[0], &tag) != nil {
				resp["error"] = map[string]any{"code": -32602, "message": "bad params"}
				break
			}
			n := c.head
			if tag != "finalized" && tag != "safe" {
				n = hexNum(tag)
			}
			b, err := json.Marshal(cannonHeader(n, cannonTime(n, c.timeOverride)))
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
			from, to := hexNum(q.FromBlock), hexNum(q.ToBlock)
			c.mu.Lock()
			c.getLogsFrom = append(c.getLogsFrom, from)
			c.mu.Unlock()
			out := []types.Log{}
			for _, lg := range c.logs {
				if lg.BlockNumber >= from && lg.BlockNumber <= to {
					out = append(out, lg)
				}
			}
			resp["result"] = out
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

func (c *cannonRPC) fromBlocks() []uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]uint64(nil), c.getLogsFrom...)
}

// usdtIssueLog builds a decodable USDT Issue log whose block hash matches the
// canned header at bn (so the WR-01 assertion passes).
func usdtIssueLog(bn uint64, logIndex uint32, amount uint64) types.Log {
	return types.Log{
		Address:     common.HexToAddress(syncUSDTContract),
		Topics:      []common.Hash{indexer.IssueTopic},
		Data:        syncWord32(new(big.Int).SetUint64(amount)),
		BlockNumber: bn,
		BlockHash:   common.HexToHash(cannonHash(bn, nil)),
		TxHash:      common.HexToHash(fmt.Sprintf("0x%064x", 0x900000+bn*16+uint64(logIndex))),
		Index:       uint(logIndex),
	}
}

// fakeEventStore records every InsertEvents call and holds checkpoint state.
// failCpWrites makes the first N WriteCheckpoint calls return failErr — the
// crash-between-writes window (SYNC-03).
type fakeEventStore struct {
	mu           sync.Mutex
	inserts      [][]storage.EventRow
	cpHeight     uint64
	cpHash       string
	cpFound      bool
	cpWrites     []uint64 // heights of successful WriteCheckpoint calls
	failCpWrites int
	failErr      error
}

func (f *fakeEventStore) InsertEvents(_ context.Context, rows []storage.EventRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inserts = append(f.inserts, append([]storage.EventRow(nil), rows...))
	return nil
}

func (f *fakeEventStore) ReadCheckpoint(_ context.Context, _ string) (uint64, string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cpHeight, f.cpHash, f.cpFound, nil
}

func (f *fakeEventStore) WriteCheckpoint(_ context.Context, _ string, height uint64, blockHash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.cpWrites) < f.failCpWrites {
		return f.failErr
	}
	f.cpHeight, f.cpHash, f.cpFound = height, blockHash, true
	f.cpWrites = append(f.cpWrites, height)
	return nil
}

func (f *fakeEventStore) state() (uint64, string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cpHeight, f.cpHash, f.cpFound
}

func (f *fakeEventStore) insertCalls() [][]storage.EventRow {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]storage.EventRow(nil), f.inserts...)
}

func (f *fakeEventStore) setFailCpWrites(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failCpWrites = n
}

func runSyncAsync(t *testing.T, cr indexer.ChainReader, es indexer.EventStore, cfg *config.Config) (<-chan error, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- indexer.RunSync(ctx, cr, es, cfg) }()
	t.Cleanup(cancel)
	return errCh, cancel
}

func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("%s not reached within %v", what, timeout)
}

func awaitCanceled(t *testing.T, errCh <-chan error) {
	t.Helper()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("indexer.RunSync returned %v, want context.Canceled after cancel", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("indexer.RunSync did not return after cancel")
	}
}

// TestRunSyncFollowsHeadAcrossTwoWindows proves bounded windows advance
// through the eligible head exactly once each (SYNC-01): seeded at
// start_block 100 with head 2100 and window_blocks 1000, the loop ingests
// [101,1100] then [1101,2100], and the checkpoint lands on the second
// window's end with that header's hash.
func TestRunSyncFollowsHeadAcrossTwoWindows(t *testing.T) {
	srv := (&cannonRPC{
		head: 2100,
		logs: []types.Log{
			usdtIssueLog(200, 1, 1_000_000),
			usdtIssueLog(1200, 2, 2_000_000),
		},
	}).start(t)
	client, err := rpc.New(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	fake := &fakeEventStore{}
	cfg := syncConfig()
	cfg.StartBlock = 100

	errCh, cancel := runSyncAsync(t, client, fake, cfg)
	waitFor(t, 5*time.Second, "checkpoint at head 2100", func() bool {
		h, _, _ := fake.state()
		return h == 2100
	})
	cancel()
	awaitCanceled(t, errCh)

	inserts := fake.insertCalls()
	if len(inserts) != 2 {
		t.Fatalf("insert calls = %d, want 2 (one per window)", len(inserts))
	}
	if len(inserts[0]) != 1 || inserts[0][0].BlockNumber != 200 {
		t.Fatalf("window 1 rows = %+v, want one row at block 200", inserts[0])
	}
	if len(inserts[1]) != 1 || inserts[1][0].BlockNumber != 1200 {
		t.Fatalf("window 2 rows = %+v, want one row at block 1200", inserts[1])
	}
	h, hash, _ := fake.state()
	if h != 2100 || hash != cannonHash(2100, nil) {
		t.Fatalf("checkpoint = (%d, %s), want (2100, %s)", h, hash, cannonHash(2100, nil))
	}
}

// TestRunSyncIdleAtHeadSleepsWithoutAdvancing proves the SYNC-01 idle path:
// with the checkpoint already at the eligible head, the loop sleeps
// poll_seconds without erroring, inserting rows, or writing the checkpoint.
func TestRunSyncIdleAtHeadSleepsWithoutAdvancing(t *testing.T) {
	srv := (&cannonRPC{head: 2000}).start(t)
	client, err := rpc.New(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	fake := &fakeEventStore{cpFound: true, cpHeight: 2000, cpHash: cannonHash(2000, nil)}

	errCh, cancel := runSyncAsync(t, client, fake, syncConfig())
	time.Sleep(300 * time.Millisecond)
	select {
	case err := <-errCh:
		t.Fatalf("indexer.RunSync returned %v while idle at head; want it still sleeping", err)
	default:
	}
	if calls := fake.insertCalls(); len(calls) != 0 {
		t.Fatalf("idle loop inserted %d times, want 0", len(calls))
	}
	if h, hash, found := fake.state(); h != 2000 || hash != cannonHash(2000, nil) || !found {
		t.Fatalf("idle loop moved the checkpoint: (%d, %s, %v)", h, hash, found)
	}
	cancel()
	awaitCanceled(t, errCh)
}

// TestRunSyncEmptyWindowStillAdvancesCheckpoint proves an empty getLogs
// result is not an error (SYNC-03): the checkpoint still advances to the
// window-end height with that block's canonical hash.
func TestRunSyncEmptyWindowStillAdvancesCheckpoint(t *testing.T) {
	srv := (&cannonRPC{head: 150}).start(t)
	client, err := rpc.New(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	fake := &fakeEventStore{cpFound: true, cpHeight: 100, cpHash: cannonHash(100, nil)}

	errCh, cancel := runSyncAsync(t, client, fake, syncConfig())
	waitFor(t, 5*time.Second, "checkpoint at window end 150", func() bool {
		h, _, _ := fake.state()
		return h == 150
	})
	cancel()
	awaitCanceled(t, errCh)

	inserts := fake.insertCalls()
	if len(inserts) != 1 || len(inserts[0]) != 0 {
		t.Fatalf("empty-window inserts = %+v, want exactly one call with zero rows", inserts)
	}
	if h, hash, _ := fake.state(); h != 150 || hash != cannonHash(150, nil) {
		t.Fatalf("checkpoint = (%d, %s), want (150, %s)", h, hash, cannonHash(150, nil))
	}
}

// TestRunSyncSeedsAtStartBlockAndAtHeadPerD04 proves both D-04 seeding
// paths: with start_block set the checkpoint seeds at that block's header
// hash and the first fetch starts at start_block+1 (nothing before the seed
// is fetched); with start_block unset it seeds at the current eligible head
// and ingests nothing.
func TestRunSyncSeedsAtStartBlockAndAtHeadPerD04(t *testing.T) {
	t.Run("seeds at start_block and fetches forward only", func(t *testing.T) {
		cannon := &cannonRPC{head: 2000, logs: []types.Log{usdtIssueLog(600, 1, 5_000_000)}}
		srv := cannon.start(t)
		client, err := rpc.New(context.Background(), srv.URL)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		fake := &fakeEventStore{}
		cfg := syncConfig()
		cfg.StartBlock = 500

		errCh, cancel := runSyncAsync(t, client, fake, cfg)
		waitFor(t, 5*time.Second, "checkpoint at head 2000", func() bool {
			h, _, _ := fake.state()
			return h == 2000
		})
		cancel()
		awaitCanceled(t, errCh)

		if h, hash, _ := fake.state(); h != 2000 {
			t.Fatalf("final checkpoint height = %d, want 2000", h)
		} else if hash != cannonHash(2000, nil) {
			t.Fatalf("final checkpoint hash = %s, want %s", hash, cannonHash(2000, nil))
		}
		// Nothing before the seed is fetched: the first window opens at 501.
		froms := cannon.fromBlocks()
		if len(froms) == 0 {
			t.Fatal("no eth_getLogs served; want the first window fetch")
		}
		if froms[0] != 501 {
			t.Fatalf("first eth_getLogs FromBlock = %d, want 501 (start_block+1)", froms[0])
		}
	})

	t.Run("seeds at eligible head and ingests nothing", func(t *testing.T) {
		srv := (&cannonRPC{head: 2000}).start(t)
		client, err := rpc.New(context.Background(), srv.URL)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		fake := &fakeEventStore{}

		errCh, cancel := runSyncAsync(t, client, fake, syncConfig())
		time.Sleep(300 * time.Millisecond)
		if h, hash, found := fake.state(); !found || h != 2000 || hash != cannonHash(2000, nil) {
			t.Fatalf("checkpoint = (%d, %s, %v), want seed (2000, %s, true)", h, hash, found, cannonHash(2000, nil))
		}
		if calls := fake.insertCalls(); len(calls) != 0 {
			t.Fatalf("head-seeded run inserted %d times, want 0 (seed block itself is not ingested)", len(calls))
		}
		cancel()
		awaitCanceled(t, errCh)
	})
}

// TestRunSyncMismatchReturnsTypedError proves SYNC-05 detection: when the
// provider's canonical header at the checkpointed height no longer carries
// the checkpointed hash, indexer.RunSync returns *indexer.CheckpointMismatchError naming the
// height and both hashes before any window is fetched.
func TestRunSyncMismatchReturnsTypedError(t *testing.T) {
	cannon := &cannonRPC{head: 150, timeOverride: map[uint64]uint64{100: 1700009999}}
	srv := cannon.start(t)
	client, err := rpc.New(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	fake := &fakeEventStore{cpFound: true, cpHeight: 100, cpHash: cannonHash(100, nil)}

	err = indexer.RunSync(context.Background(), client, fake, syncConfig())
	if err == nil {
		t.Fatal("indexer.RunSync must fail on checkpoint hash mismatch")
	}
	var mm *indexer.CheckpointMismatchError
	if !errors.As(err, &mm) {
		t.Fatalf("indexer.RunSync returned %T (%v), want *indexer.CheckpointMismatchError", err, err)
	}
	if mm.Height != 100 {
		t.Errorf("mismatch Height = %d, want 100", mm.Height)
	}
	if want := cannonHash(100, nil); mm.ExpectedHash != want {
		t.Errorf("mismatch ExpectedHash = %s, want %s", mm.ExpectedHash, want)
	}
	if want := cannonHash(100, cannon.timeOverride); mm.ObservedHash != want {
		t.Errorf("mismatch ObservedHash = %s, want %s", mm.ObservedHash, want)
	}
	if froms := cannon.fromBlocks(); len(froms) != 0 {
		t.Fatalf("eth_getLogs calls after mismatch = %d, want 0", len(froms))
	}
}

// TestRunSyncCrashBetweenInsertAndCheckpoint proves the SYNC-03/SYNC-04
// crash window: a failure between a successful event insert and the
// checkpoint write leaves the window un-checkpointed; a restart re-processes
// the same window (the insert log shows it twice) while the logical event
// set — keyed on (token, block_number, tx_hash, log_index) — contains
// exactly one entry per event, and the checkpoint then advances.
func TestRunSyncCrashBetweenInsertAndCheckpoint(t *testing.T) {
	srv := (&cannonRPC{head: 150, logs: []types.Log{usdtIssueLog(120, 3, 7_000_000)}}).start(t)
	client, err := rpc.New(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	failErr := errors.New("injected checkpoint write failure")
	fake := &fakeEventStore{
		cpFound:      true,
		cpHeight:     100,
		cpHash:       cannonHash(100, nil),
		failCpWrites: 1,
		failErr:      failErr,
	}

	// First pass: events are stored, the checkpoint write fails, indexer.RunSync
	// returns the injected error with the checkpoint unmoved.
	err = indexer.RunSync(context.Background(), client, fake, syncConfig())
	if !errors.Is(err, failErr) {
		t.Fatalf("first indexer.RunSync error = %v, want the injected checkpoint write failure", err)
	}
	inserts := fake.insertCalls()
	if len(inserts) != 1 || len(inserts[0]) != 1 {
		t.Fatalf("first pass inserts = %+v, want one call with the window's row", inserts)
	}
	if h, hash, _ := fake.state(); h != 100 || hash != cannonHash(100, nil) {
		t.Fatalf("checkpoint moved despite write failure: (%d, %s)", h, hash)
	}

	// Restart (crash recovery): the same window is re-fetched and
	// re-inserted, the checkpoint then advances to the window end.
	fake.setFailCpWrites(0)
	errCh, cancel := runSyncAsync(t, client, fake, syncConfig())
	waitFor(t, 5*time.Second, "checkpoint at window end 150", func() bool {
		h, _, _ := fake.state()
		return h == 150
	})
	cancel()
	awaitCanceled(t, errCh)

	inserts = fake.insertCalls()
	if len(inserts) != 2 {
		t.Fatalf("insert calls across restart = %d, want 2 (window processed twice)", len(inserts))
	}
	dedup := map[string]struct{}{}
	for _, call := range inserts {
		for _, r := range call {
			dedup[r.Token+"|"+strconv.FormatUint(r.BlockNumber, 10)+"|"+r.TxHash+"|"+strconv.FormatUint(uint64(r.LogIndex), 10)] = struct{}{}
		}
	}
	if len(dedup) != 1 {
		t.Fatalf("logical event set has %d entries, want exactly 1 (replay collapses)", len(dedup))
	}
	if h, hash, _ := fake.state(); h != 150 || hash != cannonHash(150, nil) {
		t.Fatalf("checkpoint after recovery = (%d, %s), want (150, %s)", h, hash, cannonHash(150, nil))
	}
}
