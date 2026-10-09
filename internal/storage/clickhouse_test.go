package storage

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"
)

// maxUint256 is 2^256-1, the exact decimal string being:
// 115792089237316195423570985008687907853269984665640564039457584007913129639935
const maxUint256Decimal = "115792089237316195423570985008687907853269984665640564039457584007913129639935"

// openStore returns a schema-ready Store against CLICKHOUSE_URL, skipping
// the whole suite with the documented server command when it is unset.
func openStore(t *testing.T) *Store {
	t.Helper()
	url := strings.TrimSpace(os.Getenv("CLICKHOUSE_URL"))
	if url == "" {
		t.Skipf("CLICKHOUSE_URL not set; start the pinned local server first:\n" +
			"  docker run -d --name tm-clickhouse -p 127.0.0.1:9000:9000 -e CLICKHOUSE_SKIP_USER_SETUP=1 clickhouse/clickhouse-server:26.8\n" +
			"(the 26.8 tag resolved to concrete version 26.8.20.9 at execution time;\n" +
			"  SKIP_USER_SETUP keeps the default user reachable through the loopback port mapping)",
		)
	}
	ctx := context.Background()
	store, err := Open(ctx, url)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = store.conn.Close() })
	if err := store.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	return store
}

// testRow builds a row with a unique identity via tx suffix.
func testRow(block uint64, log uint32, txSuffix string, amount *big.Int) EventRow {
	return EventRow{
		Chain:           "ethereum",
		Token:           "USDT",
		ContractAddress: "0xdac17f958d2ee523a2206206994597c13d831ec7",
		BlockNumber:     block,
		BlockHash:       fmt.Sprintf("0x%064x", block),
		BlockTime:       time.Unix(1700000000, 0).UTC(),
		TxHash:          "0x" + strings.Repeat("a", 62) + txSuffix,
		LogIndex:        log,
		EventType:       "destroyed_black_funds",
		FromAddress:     "0x532961b23a28a9e5a8b1769bdb3e8906fc7c7a01",
		RawAmount:       amount,
	}
}

// TestEnsureSchemaIsIdempotent applies the DDL twice.
func TestEnsureSchemaIsIdempotent(t *testing.T) {
	store := openStore(t)
	if err := store.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("second EnsureSchema must succeed: %v", err)
	}
}

// TestExactUint256RoundTrip stores the real DestroyedBlackFunds fixture
// amount and the maximum uint256, then requires the exact decimal strings
// back via toString(raw_amount) — no floating point anywhere in the path.
func TestExactUint256RoundTrip(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	fixture := testRow(0x15aac45, 0x277, "01", new(big.Int).SetUint64(200000000000))
	maxUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	maxRow := testRow(0x15aac45, 0x278, "02", maxUint256)
	if err := store.InsertEvents(ctx, []EventRow{fixture, maxRow}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	for _, tc := range []struct{ tx, want string }{
		{fixture.TxHash, "200000000000"},
		{maxRow.TxHash, maxUint256Decimal},
	} {
		var got string
		if err := store.conn.QueryRow(ctx,
			"SELECT toString(raw_amount) FROM stablecoin_events FINAL WHERE tx_hash = ?", tc.tx,
		).Scan(&got); err != nil {
			t.Fatalf("read back %s: %v", tc.tx, err)
		}
		if got != tc.want {
			t.Errorf("raw_amount for %s = %s, want exact %s", tc.tx, got, tc.want)
		}
	}
}

// TestReplayDuplicateInsertKeepsFinalCountAtOne proves re-ingesting the same
// range is idempotent under FINAL reads: the identical row inserted twice
// collapses to one logical row.
func TestReplayDuplicateInsertKeepsFinalCountAtOne(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()
	row := testRow(0x167ab1a, 0x249, "03", new(big.Int).SetUint64(1000000000000000))
	for i := 0; i < 2; i++ {
		if err := store.InsertEvents(ctx, []EventRow{row}); err != nil {
			t.Fatalf("insert %d: %v", i+1, err)
		}
	}
	var n uint64
	if err := store.conn.QueryRow(ctx,
		"SELECT count() FROM stablecoin_events FINAL WHERE tx_hash = ?", row.TxHash,
	).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("FINAL count after duplicate insert = %d, want 1", n)
	}
}

// TestInspectOrderedByBlockThenLogIndex requires deterministic newest-first
// block_number then log_index ordering within the inspected range, and that
// rows outside the range are excluded.
func TestInspectOrderedByBlockThenLogIndex(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()
	// Inserted deliberately out of order; expected read order below.
	rows := []EventRow{
		testRow(200, 1, "11", big.NewInt(1)),
		testRow(100, 7, "12", big.NewInt(2)),
		testRow(100, 3, "13", big.NewInt(3)),
		testRow(200, 0, "14", big.NewInt(4)),
	}
	outside := testRow(300, 0, "15", big.NewInt(5))
	if err := store.InsertEvents(ctx, append(rows, outside)); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := store.Inspect(ctx, 100, 200, 1000)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	// Expected newest-first (descending) order of this subtest's rows.
	wantOrder := []string{rows[0].TxHash, rows[3].TxHash, rows[1].TxHash, rows[2].TxHash}
	pos := map[string]int{}
	for i, r := range got {
		pos[r.TxHash] = i
	}
	if p, ok := pos[outside.TxHash]; ok {
		t.Errorf("row %s at position %d is outside the inspected range 100-200 but appeared in Inspect output", outside.TxHash, p)
	}
	last := -1
	for _, tx := range wantOrder {
		p, ok := pos[tx]
		if !ok {
			t.Fatalf("row %s missing from Inspect output", tx)
		}
		if p <= last {
			t.Errorf("row %s at position %d breaks descending block_number,log_index order after position %d", tx, p, last)
		}
		last = p
	}
}

// TestInsertEventsEmptySliceIsNoOp covers the zero-log range path.
func TestInsertEventsEmptySliceIsNoOp(t *testing.T) {
	store := openStore(t)
	if err := store.InsertEvents(context.Background(), nil); err != nil {
		t.Fatalf("nil rows: %v", err)
	}
	if err := store.InsertEvents(context.Background(), []EventRow{}); err != nil {
		t.Fatalf("empty rows: %v", err)
	}
}

// totals returns (count, sum(raw_amount)) over the inclusive block range —
// the probe-verified logical-totals oracle (02-RESEARCH Pattern 3): FINAL is
// mandatory, amounts stay exact integers, parameters stay bound.
func totals(t *testing.T, ctx context.Context, store *Store, from, to uint64) (uint64, *big.Int) {
	t.Helper()
	var n uint64
	var sum big.Int
	if err := store.conn.QueryRow(ctx,
		"SELECT count(), sum(raw_amount) FROM stablecoin_events FINAL WHERE block_number BETWEEN ? AND ?",
		from, to,
	).Scan(&n, &sum); err != nil {
		t.Fatalf("totals %d-%d: %v", from, to, err)
	}
	return n, &sum
}

// assertTotals fails the test unless the range totals equal (wantN, wantSum).
func assertTotals(t *testing.T, ctx context.Context, store *Store, from, to, wantN uint64, wantSum *big.Int, stage string) {
	t.Helper()
	n, sum := totals(t, ctx, store, from, to)
	if n != wantN || sum.Cmp(wantSum) != 0 {
		t.Fatalf("%s: totals for %d-%d = (%d, %s), want (%d, %s)", stage, from, to, n, sum.String(), wantN, wantSum.String())
	}
}

// TestCheckpointOrderedReadPreMerge proves the ordered checkpoint read is
// correct both pre-merge and post-merge (SYNC-03): checkpoints written out
// of order (100, 110, 90) read back as height 110 immediately — without
// FINAL — and still 110 after a forced merge, because
// ReplacingMergeTree(height) makes the merged survivor the greatest height
// even under out-of-order writes (02-RESEARCH Pattern 2, probe-verified).
// The test uses its own chain key so it cannot collide with sibling tests.
func TestCheckpointOrderedReadPreMerge(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()
	const chain = "test-checkpoint-ordered"

	for _, h := range []uint64{100, 110, 90} {
		if err := store.WriteCheckpoint(ctx, chain, h, fmt.Sprintf("0x%064x", h)); err != nil {
			t.Fatalf("write checkpoint %d: %v", h, err)
		}
	}
	height, hash, found, err := store.ReadCheckpoint(ctx, chain)
	if err != nil || !found {
		t.Fatalf("read checkpoint pre-merge: found=%v err=%v", found, err)
	}
	if height != 110 || hash != fmt.Sprintf("0x%064x", 110) {
		t.Fatalf("pre-merge checkpoint = (%d, %s), want (110, %s)", height, hash, fmt.Sprintf("0x%064x", 110))
	}

	if err := store.conn.Exec(ctx, "OPTIMIZE TABLE indexer_checkpoint FINAL"); err != nil {
		t.Fatalf("optimize indexer_checkpoint: %v", err)
	}
	height, hash, found, err = store.ReadCheckpoint(ctx, chain)
	if err != nil || !found {
		t.Fatalf("read checkpoint post-merge: found=%v err=%v", found, err)
	}
	if height != 110 || hash != fmt.Sprintf("0x%064x", 110) {
		t.Fatalf("post-merge checkpoint = (%d, %s), want (110, %s): the merged survivor must be the greatest height", height, hash, fmt.Sprintf("0x%064x", 110))
	}
}

// TestCrashBetweenWritesReplayIsIdempotent proves the SYNC-03 crash recipe:
// a crash between a successful event insert and the checkpoint write leaves
// the window un-checkpointed; the restart re-inserts the identical rows and
// the FINAL totals for the range equal the single-set values (no doubling,
// no gap); the checkpoint then advances past the range.
func TestCrashBetweenWritesReplayIsIdempotent(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()
	const chain = "test-crash-between"
	const from, to = uint64(0x5000000), uint64(0x5000003)

	rows := []EventRow{
		testRow(0x5000000, 0, "c1", big.NewInt(100)),
		testRow(0x5000001, 1, "c2", big.NewInt(200)),
		testRow(0x5000003, 2, "c3", big.NewInt(300)),
	}
	wantN, wantSum := uint64(3), big.NewInt(600)

	// First pass: events stored, checkpoint NOT written (the crash window).
	if err := store.InsertEvents(ctx, rows); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	// Restart re-processes the window: the identical rows are re-inserted.
	if err := store.InsertEvents(ctx, rows); err != nil {
		t.Fatalf("replay insert: %v", err)
	}
	assertTotals(t, ctx, store, from, to, wantN, wantSum, "after crash-replay")

	// The checkpoint then advances past the range and reads back.
	if err := store.WriteCheckpoint(ctx, chain, to, fmt.Sprintf("0x%064x", to)); err != nil {
		t.Fatalf("write checkpoint after recovery: %v", err)
	}
	height, hash, found, err := store.ReadCheckpoint(ctx, chain)
	if err != nil || !found {
		t.Fatalf("read checkpoint after recovery: found=%v err=%v", found, err)
	}
	if height != to || hash != fmt.Sprintf("0x%064x", to) {
		t.Fatalf("recovered checkpoint = (%d, %s), want (%d, %s)", height, hash, to, fmt.Sprintf("0x%064x", to))
	}
}

// TestReplayTotalsStableBeforeAndAfterOptimize proves SYNC-04 byte-identical
// replay totals: re-inserting a processed range's rows leaves FINAL (count,
// sum) unchanged before background merges, after OPTIMIZE ... FINAL, and
// when the replay covers a wider range than the original one.
func TestReplayTotalsStableBeforeAndAfterOptimize(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()
	const subFrom, subTo = uint64(0x5000100), uint64(0x5000110)
	const wideTo = uint64(0x5000120)

	rows := []EventRow{
		testRow(0x5000100, 0, "d1", big.NewInt(1_000_000_000_000)),
		testRow(0x5000105, 1, "d2", big.NewInt(2_000_000_000_000)),
		testRow(0x5000110, 2, "d3", big.NewInt(3_000_000_000_000)),
	}
	wantN, wantSum := uint64(3), big.NewInt(6_000_000_000_000)

	if err := store.InsertEvents(ctx, rows); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	assertTotals(t, ctx, store, subFrom, subTo, wantN, wantSum, "after first ingest")

	// Replay the same identity rows: identical totals pre-merge.
	if err := store.InsertEvents(ctx, rows); err != nil {
		t.Fatalf("replay insert: %v", err)
	}
	assertTotals(t, ctx, store, subFrom, subTo, wantN, wantSum, "after replay, pre-merge")

	// Force every background merge: identical totals post-OPTIMIZE.
	if err := store.conn.Exec(ctx, "OPTIMIZE TABLE stablecoin_events FINAL"); err != nil {
		t.Fatalf("optimize stablecoin_events: %v", err)
	}
	assertTotals(t, ctx, store, subFrom, subTo, wantN, wantSum, "after OPTIMIZE ... FINAL")

	// Replay a WIDER range (superset bounds): the same rows again plus one
	// event outside the original sub-range. The original sub-range's totals
	// must not move; the wider range sees each event exactly once.
	wider := append(append([]EventRow(nil), rows...), testRow(0x5000120, 0, "d4", big.NewInt(4_000_000_000_000)))
	if err := store.InsertEvents(ctx, wider); err != nil {
		t.Fatalf("wider replay insert: %v", err)
	}
	assertTotals(t, ctx, store, subFrom, subTo, wantN, wantSum, "after wider replay, sub-range")
	assertTotals(t, ctx, store, subFrom, wideTo, 4, big.NewInt(10_000_000_000_000), "after wider replay, wider range")
}
