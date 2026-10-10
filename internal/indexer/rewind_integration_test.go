// Env-gated real-store rewind proof (SYNC-05 / D-03): indexer.Rewind driven
// end to end against *storage.Store on the pinned ClickHouse server. The
// offline rewind tests use the in-memory fake, whose WriteCheckpoint simply
// overwrites state — they cannot see that the real indexer_checkpoint
// (ReplacingMergeTree(height), read ORDER BY height DESC LIMIT 1) keeps the
// orphaned chain's higher checkpoint rows winning over a re-anchor written
// at a lower height. This suite skips — never fails — without
// CLICKHOUSE_URL, identical to the devnet suite's gating.
package indexer_test

import (
	"context"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"TOkenMonitor/internal/indexer"
	"TOkenMonitor/internal/rpc"
	"TOkenMonitor/internal/storage"
)

// The real store satisfies the extended RewindStore surface (the
// DeleteCheckpointAbove seam this suite proves).
var _ indexer.RewindStore = (*storage.Store)(nil)

// openRewindDB opens a fresh ClickHouse database per test (the devnet
// isolation pattern: RunSync/Rewind key checkpoints under the "ethereum"
// chain constant, so the database is the namespace) and returns the store
// plus a control connection for out-of-band SQL (OPTIMIZE).
func openRewindDB(t *testing.T) (*storage.Store, driver.Conn) {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("CLICKHOUSE_URL"))
	if base == "" {
		t.Skip("CLICKHOUSE_URL not set; start the pinned local server first:\n" +
			"  docker run -d --name tm-clickhouse -p 127.0.0.1:9000:9000 -e CLICKHOUSE_SKIP_USER_SETUP=1 clickhouse/clickhouse-server:26.8")
	}
	ctx := context.Background()
	opts, err := clickhouse.ParseDSN(base)
	if err != nil {
		t.Fatalf("parse CLICKHOUSE_URL: %v", err)
	}
	ctrl, err := clickhouse.Open(opts)
	if err != nil {
		t.Fatalf("open control connection: %v", err)
	}
	name := fmt.Sprintf("tm_rewind_%d", os.Getpid())
	if err := ctrl.Exec(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", name)); err != nil {
		t.Fatalf("create fresh database: %v", err)
	}
	t.Cleanup(func() {
		_ = ctrl.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", name))
		_ = ctrl.Close()
	})
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse CLICKHOUSE_URL: %v", err)
	}
	u.Path = "/" + name
	store, err := storage.Open(ctx, u.String())
	if err != nil {
		t.Fatalf("open fresh store: %v", err)
	}
	if err := store.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	return store, ctrl
}

// TestRewindReanchorWinsOnRealStore proves the CR-01 failure mode and its
// fix on the production schema: after a normal sync left checkpoints at
// 100/110/120 on the orphaned chain and Rewind re-anchors at the verified
// point 100, ReadCheckpoint must return 100 — immediately (lightweight
// DELETE visible to the ordered read) AND after OPTIMIZE TABLE
// indexer_checkpoint FINAL (the max-height ReplacingMergeTree survivor would
// otherwise resurrect 120). Without the checkpoint cleanup above the point,
// both reads return 120 and a restarted run mismatches forever.
func TestRewindReanchorWinsOnRealStore(t *testing.T) {
	store, ctrl := openRewindDB(t)
	ctx := context.Background()
	const forkAt = uint64(101) // chain B diverges at 101; the common ancestor is 100

	reorg := &twoChainRPC{head: 150, forkAt: forkAt}
	srv := reorg.start(t)
	client, err := rpc.New(ctx, srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// Chain-A history: event rows at 100..103 and window checkpoints at
	// 100/110/120 (the advance records a normal sync would leave behind).
	for _, row := range []storage.EventRow{
		reorgRow(100, 0, 1000, reorgHashA(100), "0x"+strings.Repeat("a", 63)+"0"),
		reorgRow(101, 1, 10, reorgHashA(101), "0x"+strings.Repeat("a", 63)+"1"),
		reorgRow(102, 2, 20, reorgHashA(102), "0x"+strings.Repeat("a", 63)+"2"),
		reorgRow(103, 3, 30, reorgHashA(103), "0x"+strings.Repeat("a", 63)+"3"),
	} {
		row.BlockTime = time.Unix(1700000000, 0).UTC() // reorgRow leaves DateTime zero; the real insert needs a valid one
		if err := store.InsertEvents(ctx, []storage.EventRow{row}); err != nil {
			t.Fatalf("insert chain-A row at %d: %v", row.BlockNumber, err)
		}
	}
	for _, h := range []uint64{100, 110, 120} {
		if err := store.WriteCheckpoint(ctx, indexer.Chain, h, reorgHashA(h)); err != nil {
			t.Fatalf("write checkpoint %d: %v", h, err)
		}
	}
	h, hash, found, err := store.ReadCheckpoint(ctx, indexer.Chain)
	if err != nil || !found || h != 120 {
		t.Fatalf("pre-rewind checkpoint = (%d, %s, found=%v, err=%v), want the orphaned chain's (120, %s)", h, hash, found, err, reorgHashA(120))
	}

	// The reorg: the provider now serves chain B (fork at 100). Rewind must
	// walk to the common ancestor 100, delete strictly above it, drop the
	// superseded checkpoints strictly above it, and re-anchor at 100.
	reorg.onB.Store(true)
	report, err := indexer.Rewind(ctx, client, store, syncConfig(), indexer.RewindOpts{Yes: true})
	if err != nil {
		t.Fatalf("Rewind against the real store: %v", err)
	}
	if report.RewindPoint != 100 || report.RewindHash != reorgHashA(100) {
		t.Fatalf("rewind point = (%d, %s), want the common ancestor (100, %s)", report.RewindPoint, report.RewindHash, reorgHashA(100))
	}
	if report.Before.Count != 3 || report.Before.Sum.Int64() != 60 || report.After.Count != 0 || report.After.Sum.Int64() != 0 {
		t.Fatalf("before/after totals = (%d, %s) / (%d, %s), want (3, 60) / (0, 0)",
			report.Before.Count, report.Before.Sum.String(), report.After.Count, report.After.Sum.String())
	}

	// CR-01 proper: the re-anchor must win the production read immediately —
	// no FINAL, no OPTIMIZE. Before the checkpoint cleanup this returned 120.
	h, hash, found, err = store.ReadCheckpoint(ctx, indexer.Chain)
	if err != nil || !found {
		t.Fatalf("post-rewind ReadCheckpoint: found=%v err=%v", found, err)
	}
	if h != 100 || hash != reorgHashA(100) {
		t.Fatalf("post-rewind checkpoint = (%d, %s), want the re-anchor (100, %s) — the stale higher row must not win the ordered read", h, hash, reorgHashA(100))
	}

	// ...and survive a forced merge: the ReplacingMergeTree(height) survivor
	// over the cleaned table is the re-anchor, not the deleted 110/120 rows.
	if err := ctrl.Exec(ctx, "OPTIMIZE TABLE indexer_checkpoint FINAL"); err != nil {
		t.Fatalf("optimize indexer_checkpoint: %v", err)
	}
	h, hash, found, err = store.ReadCheckpoint(ctx, indexer.Chain)
	if err != nil || !found {
		t.Fatalf("post-OPTIMIZE ReadCheckpoint: found=%v err=%v", found, err)
	}
	if h != 100 || hash != reorgHashA(100) {
		t.Fatalf("post-OPTIMIZE checkpoint = (%d, %s), want the re-anchor to survive the forced merge as (100, %s)", h, hash, reorgHashA(100))
	}

	// The strictly-above prohibition: the block-100 row survives the rewind.
	n, sum, err := store.RangeTotals(ctx, indexer.Chain, 100, 100)
	if err != nil {
		t.Fatalf("range totals at the point: %v", err)
	}
	if n != 1 || sum.Cmp(big.NewInt(1000)) != 0 {
		t.Fatalf("row at the rewind point = (%d, %s), want (1, 1000) untouched", n, sum.String())
	}
}
