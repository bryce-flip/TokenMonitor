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
