// Deterministic reorg harness and rewind behavior proofs (02-03, SYNC-05 /
// D-03): one httptest server serves two conflicting chains — chain A (the
// originally indexed one) and chain B, which agrees with A below forkAt and
// serves different headers and logs from forkAt up. Flipping the server
// mid-test makes the reorg deterministic, which no localnet can (D-02 scope
// guard: ethereum-package has no reorg knob). Reuses the cannonHeader /
// cannonTime helpers from sync_test.go (same external test package).
package indexer_test

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"TOkenMonitor/internal/indexer"
	"TOkenMonitor/internal/rpc"
	"TOkenMonitor/internal/storage"
)

// reorgTimeOffset shifts chain-B header timestamps from forkAt up; because
// header hashes are RLP-derived from the header fields, the shift yields a
// genuinely different canonical hash series above the fork while the two
// chains stay identical (same hash) below it — the common ancestor.
const reorgTimeOffset = 500_000_000

func reorgHashA(n uint64) string { return cannonHash(n, nil) }

// reorgHashB is chain B's hash at n — equal to chain A's below forkAt.
func reorgHashB(n, forkAt uint64) string {
	t := cannonTime(n, nil)
	if n >= forkAt {
		t += reorgTimeOffset
	}
	return cannonHeader(n, t).Hash().Hex()
}

// twoChainRPC answers eth_chainId, eth_blockNumber, eth_getBlockByNumber
// (including the finalized tag, at head), and eth_getLogs with the active
// chain's logs filtered to the requested range. onB flips the served chain.
type twoChainRPC struct {
	head   uint64
	forkAt uint64
	onB    atomic.Bool
	logsA  []types.Log
	logsB  []types.Log

	getLogsCalls atomic.Int32

	mu        sync.Mutex
	logRanges [][2]uint64
}

func (c *twoChainRPC) headerAt(n uint64) *types.Header {
	t := cannonTime(n, nil)
	if c.onB.Load() && n >= c.forkAt {
		t += reorgTimeOffset
	}
	return cannonHeader(n, t)
}

func (c *twoChainRPC) start(t *testing.T) *httptest.Server {
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
			resp["result"] = "0x1"
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
			b, err := json.Marshal(c.headerAt(n))
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
			c.logRanges = append(c.logRanges, [2]uint64{from, to})
			c.mu.Unlock()
			c.getLogsCalls.Add(1)
			logs := c.logsA
			if c.onB.Load() {
				logs = c.logsB
			}
			out := []types.Log{}
			for _, lg := range logs {
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

// fakeRewindStore is an in-memory indexer.RewindStore: checkpoint state comes
// from the embedded fakeEventStore; the row list is what DeleteEventsFrom /
// StoredBlockHashes / RangeTotals operate on, so before/after reports behave
// like the real store. `deletes` records every DeleteEventsFrom bound;
// `cpDeletes` records every DeleteCheckpointAbove bound.
type fakeRewindStore struct {
	*fakeEventStore
	mu                 sync.Mutex
	rows               []storage.EventRow
	deletes            []uint64
	cpDeletes          []uint64
	failNthRangeTotals int // 0 = off; fail the Nth RangeTotals call (WR-03 proof)
	rangeTotalsCalls   int
}

func (f *fakeRewindStore) InsertEvents(ctx context.Context, rows []storage.EventRow) error {
	if err := f.fakeEventStore.InsertEvents(ctx, rows); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = append(f.rows, rows...)
	return nil
}

func (f *fakeRewindStore) DeleteEventsFrom(_ context.Context, chain string, after uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	kept := f.rows[:0]
	for _, r := range f.rows {
		if r.Chain != chain || r.BlockNumber <= after {
			kept = append(kept, r)
		}
	}
	f.rows = kept
	f.deletes = append(f.deletes, after)
	return nil
}

func (f *fakeRewindStore) DeleteCheckpointAbove(_ context.Context, _ string, after uint64) error {
	f.mu.Lock()
	f.cpDeletes = append(f.cpDeletes, after)
	f.mu.Unlock()
	// The fake holds one checkpoint row; mirroring the strictly-above rule
	// drops it whenever it sits above the bound. The re-anchor
	// WriteCheckpoint immediately follows in Rewind.
	es := f.fakeEventStore
	es.mu.Lock()
	defer es.mu.Unlock()
	if es.cpFound && es.cpHeight > after {
		es.cpFound = false
	}
	return nil
}

func (f *fakeRewindStore) StoredBlockHashes(_ context.Context, chain string, from, to uint64) (map[uint64]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[uint64]string{}
	for _, r := range f.rows {
		if r.Chain == chain && r.BlockNumber >= from && r.BlockNumber <= to {
			if _, ok := out[r.BlockNumber]; !ok {
				out[r.BlockNumber] = r.BlockHash
			}
		}
	}
	return out, nil
}

func (f *fakeRewindStore) RangeTotals(_ context.Context, chain string, from, to uint64) (uint64, *big.Int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rangeTotalsCalls++
	if f.failNthRangeTotals != 0 && f.rangeTotalsCalls == f.failNthRangeTotals {
		return 0, nil, errors.New("injected range-totals failure")
	}
	n := uint64(0)
	sum := new(big.Int)
	for _, r := range f.rows {
		if r.Chain == chain && r.BlockNumber >= from && r.BlockNumber <= to {
			n++
			sum = new(big.Int).Add(sum, r.RawAmount)
		}
	}
	return n, sum, nil
}

func (f *fakeRewindStore) deleteBounds() []uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uint64(nil), f.deletes...)
}

func (f *fakeRewindStore) cpDeleteBounds() []uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uint64(nil), f.cpDeletes...)
}

func (f *fakeRewindStore) storedRows() []storage.EventRow {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]storage.EventRow(nil), f.rows...)
}

// reorgRow is one stored chain-A event row for the harness.
func reorgRow(bn uint64, logIndex uint32, amount uint64, blockHash, txHash string) storage.EventRow {
	return storage.EventRow{
		Chain:       indexer.Chain,
		Token:       "USDT",
		BlockNumber: bn,
		BlockHash:   blockHash,
		TxHash:      txHash,
		LogIndex:    logIndex,
		EventType:   indexer.EventMint,
		RawAmount:   new(big.Int).SetUint64(amount),
	}
}

// reorgIssueLog builds a decodable USDT Issue log whose block hash matches
// the harness header at bn on the given chain.
func reorgIssueLog(bn uint64, logIndex uint32, amount uint64, blockHash string, txHash common.Hash) types.Log {
	return types.Log{
		Address:     common.HexToAddress(syncUSDTContract),
		Topics:      []common.Hash{indexer.IssueTopic},
		Data:        syncWord32(new(big.Int).SetUint64(amount)),
		BlockNumber: bn,
		BlockHash:   common.HexToHash(blockHash),
		TxHash:      txHash,
		Index:       uint(logIndex),
	}
}

// TestResumeMismatchHaltsWithDiagnostic proves the SYNC-05 halt against a
// genuinely conflicting chain: with the checkpoint anchored on chain A at
// 103 and the provider now serving chain B (fork at 101), RunSync returns
// *CheckpointMismatchError whose message names the height, the expected hash,
// and the observed hash — and zero eth_getLogs are ever issued (nothing is
// ingested from the conflicting chain).
func TestResumeMismatchHaltsWithDiagnostic(t *testing.T) {
	const forkAt = 101
	reorg := &twoChainRPC{head: 150, forkAt: forkAt}
	reorg.onB.Store(true)
	srv := reorg.start(t)
	client, err := rpc.New(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	fake := &fakeEventStore{cpFound: true, cpHeight: 103, cpHash: reorgHashA(103)}

	err = indexer.RunSync(context.Background(), client, fake, syncConfig())
	if err == nil {
		t.Fatal("RunSync must halt on a checkpoint hash mismatch")
	}
	var mm *indexer.CheckpointMismatchError
	if !errors.As(err, &mm) {
		t.Fatalf("RunSync returned %T (%v), want *CheckpointMismatchError", err, err)
	}
	wantExpected, wantObserved := reorgHashA(103), reorgHashB(103, forkAt)
	if mm.Height != 103 || mm.ExpectedHash != wantExpected || mm.ObservedHash != wantObserved {
		t.Fatalf("mismatch = (%d, %s, %s), want (103, %s, %s)",
			mm.Height, mm.ExpectedHash, mm.ObservedHash, wantExpected, wantObserved)
	}
	// The diagnostic must NAME all three facts (D-03: loud and specific).
	msg := mm.Error()
	for _, want := range []string{"103", wantExpected, wantObserved} {
		if !strings.Contains(msg, want) {
			t.Fatalf("mismatch diagnostic %q does not name %q", msg, want)
		}
	}
	if calls := reorg.getLogsCalls.Load(); calls != 0 {
		t.Fatalf("eth_getLogs calls after mismatch = %d, want 0 (nothing ingested)", calls)
	}
	if h, hash, _ := fake.state(); h != 103 || hash != wantExpected {
		t.Fatalf("checkpoint moved during mismatch halt: (%d, %s)", h, hash)
	}
}

// TestRewindWalksToCommonAncestorAndExcludesOrphans proves the D-03 verified
// rewind: stored chain-A rows sit at 100..103 with the checkpoint at 103;
// chain B forked at 101. Rewind records expected (stored) vs observed
// (canonical) hashes, walks down to the first agreeing height (100 — the
// common ancestor), deletes strictly above it, re-anchors the checkpoint at
// 100 with the canonical hash, and reports the before/after FINAL totals.
// Rows at or below the point are untouched (the prohibition).
func TestRewindWalksToCommonAncestorAndExcludesOrphans(t *testing.T) {
	const forkAt = 101
	reorg := &twoChainRPC{head: 150, forkAt: forkAt}
	srv := reorg.start(t)
	client, err := rpc.New(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	fake := &fakeRewindStore{fakeEventStore: &fakeEventStore{cpFound: true, cpHeight: 103, cpHash: reorgHashA(103)}}
	fake.rows = []storage.EventRow{
		reorgRow(100, 0, 1000, reorgHashA(100), "0x"+strings.Repeat("a", 63)+"0"),
		reorgRow(101, 1, 10, reorgHashA(101), "0x"+strings.Repeat("a", 63)+"1"),
		reorgRow(102, 2, 20, reorgHashA(102), "0x"+strings.Repeat("a", 63)+"2"),
		reorgRow(103, 3, 30, reorgHashA(103), "0x"+strings.Repeat("a", 63)+"3"),
	}
	reorg.onB.Store(true)

	report, err := indexer.Rewind(context.Background(), client, fake, syncConfig(), indexer.RewindOpts{Yes: true})
	if err != nil {
		t.Fatalf("Rewind: %v", err)
	}
	if report.CheckpointHeight != 103 ||
		report.ExpectedHash != reorgHashA(103) ||
		report.ObservedHash != reorgHashB(103, forkAt) {
		t.Fatalf("report header = (%d, %s, %s), want (103, %s, %s)",
			report.CheckpointHeight, report.ExpectedHash, report.ObservedHash, reorgHashA(103), reorgHashB(103, forkAt))
	}
	if report.RewindPoint != 100 || report.RewindHash != reorgHashA(100) {
		t.Fatalf("rewind point = (%d, %s), want the common ancestor (100, %s)",
			report.RewindPoint, report.RewindHash, reorgHashA(100))
	}
	if got := fake.deleteBounds(); len(got) != 1 || got[0] != 100 {
		t.Fatalf("DeleteEventsFrom bounds = %v, want exactly [100] (strictly above the verified point)", got)
	}
	if got := fake.cpDeleteBounds(); len(got) != 1 || got[0] != 100 {
		t.Fatalf("DeleteCheckpointAbove bounds = %v, want exactly [100] (checkpoint rows strictly above the re-anchor point are dropped so the re-anchor wins the read)", got)
	}
	if h, hash, _ := fake.state(); h != 100 || hash != reorgHashA(100) {
		t.Fatalf("checkpoint after rewind = (%d, %s), want re-anchored (100, %s)", h, hash, reorgHashA(100))
	}
	rows := fake.storedRows()
	if len(rows) != 1 || rows[0].BlockNumber != 100 {
		t.Fatalf("stored rows after rewind = %+v, want only the block-100 row (rows at or below the point are never deleted)", rows)
	}
	if report.Before.Count != 3 || report.Before.Sum.Int64() != 60 {
		t.Fatalf("before totals = (%d, %s), want (3, 60)", report.Before.Count, report.Before.Sum.String())
	}
	if report.After.Count != 0 || report.After.Sum.Int64() != 0 {
		t.Fatalf("after totals = (%d, %s), want (0, 0)", report.After.Count, report.After.Sum.String())
	}
	if report.DeletedRange != "block_number > 100" {
		t.Fatalf("deleted range = %q, want the strictly-above form", report.DeletedRange)
	}
}

// TestRewindToOverrideRejectedOnDisagreement proves the verified-override
// rule: --to 102 points at a height whose stored (chain-A) hash disagrees
// with the provider's canonical (chain-B) header there, so Rewind returns a
// named error and rewinds NOTHING — no delete, no re-anchor.
func TestRewindToOverrideRejectedOnDisagreement(t *testing.T) {
	const forkAt = 101
	reorg := &twoChainRPC{head: 150, forkAt: forkAt}
	srv := reorg.start(t)
	client, err := rpc.New(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	fake := &fakeRewindStore{fakeEventStore: &fakeEventStore{cpFound: true, cpHeight: 103, cpHash: reorgHashA(103)}}
	fake.rows = []storage.EventRow{
		reorgRow(100, 0, 1000, reorgHashA(100), "0x"+strings.Repeat("a", 63)+"0"),
		reorgRow(101, 1, 10, reorgHashA(101), "0x"+strings.Repeat("a", 63)+"1"),
		reorgRow(102, 2, 20, reorgHashA(102), "0x"+strings.Repeat("a", 63)+"2"),
		reorgRow(103, 3, 30, reorgHashA(103), "0x"+strings.Repeat("a", 63)+"3"),
	}
	reorg.onB.Store(true)

	_, err = indexer.Rewind(context.Background(), client, fake, syncConfig(), indexer.RewindOpts{To: 102, Yes: true})
	if !errors.Is(err, indexer.ErrRewindToMismatch) {
		t.Fatalf("Rewind error = %v, want ErrRewindToMismatch", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, reorgHashA(102)) || !strings.Contains(msg, reorgHashB(102, forkAt)) {
		t.Fatalf("override rejection %q does not name the stored and canonical hashes at 102", msg)
	}
	if got := fake.deleteBounds(); len(got) != 0 {
		t.Fatalf("DeleteEventsFrom bounds after rejected override = %v, want none", got)
	}
	if got := fake.cpDeleteBounds(); len(got) != 0 {
		t.Fatalf("DeleteCheckpointAbove bounds after rejected override = %v, want none", got)
	}
	if h, hash, _ := fake.state(); h != 103 || hash != reorgHashA(103) {
		t.Fatalf("checkpoint moved after rejected override: (%d, %s), want (103, %s)", h, hash, reorgHashA(103))
	}
	if rows := fake.storedRows(); len(rows) != 4 {
		t.Fatalf("stored rows after rejected override = %d, want all 4 intact", len(rows))
	}
}

// TestRewindReturnsPartialReportWhenAfterTotalsFails proves WR-03: when the
// after-totals measurement fails AFTER the destructive steps already ran,
// Rewind still returns the report — rewind point, delete range, before
// totals — with AfterTotalsUnavailable set, alongside the error naming what
// completed; the delete and the re-anchor are visible in the store state,
// not rolled back and not hidden.
func TestRewindReturnsPartialReportWhenAfterTotalsFails(t *testing.T) {
	const forkAt = 101
	reorg := &twoChainRPC{head: 150, forkAt: forkAt}
	srv := reorg.start(t)
	client, err := rpc.New(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	fake := &fakeRewindStore{fakeEventStore: &fakeEventStore{cpFound: true, cpHeight: 103, cpHash: reorgHashA(103)}}
	fake.rows = []storage.EventRow{
		reorgRow(100, 0, 1000, reorgHashA(100), "0x"+strings.Repeat("a", 63)+"0"),
		reorgRow(101, 1, 10, reorgHashA(101), "0x"+strings.Repeat("a", 63)+"1"),
		reorgRow(102, 2, 20, reorgHashA(102), "0x"+strings.Repeat("a", 63)+"2"),
		reorgRow(103, 3, 30, reorgHashA(103), "0x"+strings.Repeat("a", 63)+"3"),
	}
	fake.failNthRangeTotals = 2 // 1st call = before-totals (succeeds); 2nd = after-totals (fails)
	reorg.onB.Store(true)

	report, err := indexer.Rewind(context.Background(), client, fake, syncConfig(), indexer.RewindOpts{Yes: true})
	if err == nil {
		t.Fatal("the after-totals failure must still surface as the returned error")
	}
	if msg := err.Error(); !strings.Contains(msg, "after-totals failed") || !strings.Contains(msg, "completed at 100") {
		t.Fatalf("error %q does not state that the destructive steps completed at 100 and only the measurement failed", msg)
	}
	if report == nil {
		t.Fatal("the partial report must survive the after-totals failure")
	}
	if report.RewindPoint != 100 || report.DeletedRange != "block_number > 100" {
		t.Fatalf("partial report = (point %d, range %q), want (100, block_number > 100)", report.RewindPoint, report.DeletedRange)
	}
	if report.Before.Count != 3 || report.Before.Sum.Int64() != 60 {
		t.Fatalf("partial report before totals = (%d, %s), want (3, 60)", report.Before.Count, report.Before.Sum.String())
	}
	if !report.AfterTotalsUnavailable || report.After.Sum != nil {
		t.Fatalf("partial report after side = (unavailable=%v, sum=%v), want (true, nil)", report.AfterTotalsUnavailable, report.After.Sum)
	}
	// The destructive steps really ran and are observable: delete strictly
	// above 100, checkpoint re-anchored at the common ancestor.
	if got := fake.deleteBounds(); len(got) != 1 || got[0] != 100 {
		t.Fatalf("DeleteEventsFrom bounds = %v, want [100]", got)
	}
	if h, hash, _ := fake.state(); h != 100 || hash != reorgHashA(100) {
		t.Fatalf("checkpoint after partial rewind = (%d, %s), want re-anchored (100, %s)", h, hash, reorgHashA(100))
	}
	if rows := fake.storedRows(); len(rows) != 1 || rows[0].BlockNumber != 100 {
		t.Fatalf("stored rows after partial rewind = %+v, want only the block-100 row", rows)
	}
}

// TestRewindSameIdentityReinsertPath proves the reorg re-inclusion shape end
// to end through the sync loop: after the rewind to the common ancestor, a
// transaction re-included on chain B with the SAME tx_hash and log_index but
// a NEW block hash and amount is ingested with exactly the new identity
// values, exactly once. (The storage-level authority for the same-identity
// replacement under FINAL reads is Task 2's ClickHouse proof.)
func TestRewindSameIdentityReinsertPath(t *testing.T) {
	const forkAt = 101
	reorgTx := common.HexToHash("0x" + strings.Repeat("b", 63) + "7")
	reorg := &twoChainRPC{
		head:   103,
		forkAt: forkAt,
		logsA: []types.Log{
			// The displaced chain-A event at 102.
			reorgIssueLog(102, 1, 5_000_000, reorgHashA(102), reorgTx),
		},
	}
	srv := reorg.start(t)
	client, err := rpc.New(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	fake := &fakeRewindStore{fakeEventStore: &fakeEventStore{cpFound: true, cpHeight: 103, cpHash: reorgHashA(103)}}
	fake.rows = []storage.EventRow{
		reorgRow(100, 0, 1000, reorgHashA(100), "0x"+strings.Repeat("a", 63)+"0"),
		reorgRow(102, 1, 5_000_000, reorgHashA(102), reorgTx.Hex()),
	}

	// The reorg: chain B takes over; the same transaction is re-included at
	// block 104 with a new block hash and a new amount.
	reorg.onB.Store(true)
	reorg.logsB = []types.Log{
		reorgIssueLog(104, 1, 9_000_000, reorgHashB(104, forkAt), reorgTx),
	}

	if _, err := indexer.Rewind(context.Background(), client, fake, syncConfig(), indexer.RewindOpts{Yes: true}); err != nil {
		t.Fatalf("rewind: %v", err)
	}
	if got := fake.deleteBounds(); len(got) != 1 || got[0] != 100 {
		t.Fatalf("rewind delete bounds = %v, want [100]", got)
	}

	// Resume ordinary sync on chain B: window [101, head] now ingests the
	// re-included transaction's event with its NEW values.
	reorg.head = 105
	cfg := syncConfig()
	errCh, cancel := runSyncAsync(t, client, fake, cfg)
	waitFor(t, 5*time.Second, "checkpoint at head 105", func() bool {
		h, _, _ := fake.state()
		return h == 105
	})
	cancel()
	awaitCanceled(t, errCh)

	var matches []storage.EventRow
	for _, rows := range fake.insertCalls() {
		for _, r := range rows {
			if r.TxHash == reorgTx.Hex() {
				matches = append(matches, r)
			}
		}
	}
	if len(matches) != 1 {
		t.Fatalf("inserted rows carrying the re-included tx = %d, want exactly 1", len(matches))
	}
	m := matches[0]
	if m.BlockNumber != 104 || m.BlockHash != reorgHashB(104, forkAt) || m.RawAmount.Int64() != 9_000_000 || m.LogIndex != 1 {
		t.Fatalf("re-included row = (block %d, %s, amount %s, log %d), want the NEW identity values (104, %s, 9000000, 1)",
			m.BlockNumber, m.BlockHash, m.RawAmount.String(), m.LogIndex, reorgHashB(104, forkAt))
	}
	// The stored row set holds exactly one row with that identity — the new one.
	stored := fake.storedRows()
	seen := 0
	for _, r := range stored {
		if r.TxHash == reorgTx.Hex() {
			seen++
			if r.BlockNumber != 104 || r.RawAmount.Int64() != 9_000_000 {
				t.Fatalf("stored re-included row = (block %d, amount %s), want (104, 9000000)", r.BlockNumber, r.RawAmount.String())
			}
		}
	}
	if seen != 1 {
		t.Fatalf("stored rows carrying the re-included tx = %d, want exactly 1 (the rewind removed the displaced one)", seen)
	}
}
