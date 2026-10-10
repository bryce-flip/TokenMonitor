---
phase: 02-reliable-canonical-indexing
reviewed: 2026-10-10T09:45:09Z
depth: standard
files_reviewed: 16
files_reviewed_list:
  - cmd/indexer/main.go
  - config/tokens.json
  - go.mod
  - internal/config/config.go
  - internal/config/config_test.go
  - internal/indexer/devnet_test.go
  - internal/indexer/reorg_test.go
  - internal/indexer/rewind.go
  - internal/indexer/sync.go
  - internal/indexer/sync_test.go
  - internal/rpc/client.go
  - internal/rpc/client_test.go
  - internal/storage/clickhouse.go
  - internal/storage/clickhouse_test.go
  - migrations/clickhouse.sql
  - README.md
findings:
  critical: 1
  warning: 3
  info: 8
  total: 12
status: issues_found
---

# Phase 2: Code Review Report

**Reviewed:** 2026-10-10T09:45:09Z
**Depth:** standard
**Files Reviewed:** 16 (go.sum excluded as a lock file per review scope rules)
**Status:** issues_found

## Summary

Phase 2 delivers the continuous checkpointed sync loop, the `-32005`
message-discriminating resilient fetch, the operator-gated verified rewind, and
the devnet suite. The offline suite passes (`go vet ./...` clean, `go test
./...` green), and most domain invariants hold under direct code tracing.
However, the **verified rewind — the SYNC-05 recovery path this phase exists to
deliver — is broken end-to-end on the real ClickHouse store** (CR-01). The
re-anchor writes a checkpoint row at a *lower* height, but
`indexer_checkpoint` is `ReplacingMergeTree(height)` read with
`ORDER BY height DESC LIMIT 1`, so the old, higher, orphaned-chain checkpoint
row wins forever. I reproduced this empirically on the pinned server
(26.8.20.9): after checkpoints at 100/110/120 and a re-anchor insert at 100,
`ReadCheckpoint` returns `120 / 0xhashA120` both pre-merge and post-OPTIMIZE.
A `run` after `rewind` re-reads the stale checkpoint, mismatches again, and
halts — the operator loops forever. Every offline rewind test uses the
in-memory fake (whose `WriteCheckpoint` overwrites state), and neither the
ClickHouse suite nor the devnet suite exercises `Rewind` against
`*storage.Store`, so the defect is invisible to the suite.

### Domain-invariant verification (prompt checklist)

| # | Invariant | Verdict |
|---|-----------|---------|
| 1 | Checkpoint advances only after durable accept | **PASS** — `ingestWindow` writes the checkpoint strictly after `InsertEvents` returns nil (`sync.go:223-228`); all failure paths return before the write; the batch covers all tokens atomically (single `INSERT`). |
| 2 | Retries never skip/advance; halving only on cap messages; throttle backoff; floor fails closed | **PASS with note** — throttle retries the identical range (`FetchLogs` + `withRateLimitRetry`); cap halves the same start block; floor trips with the named fail-closed error (`client.go:261-274`). "query timeout exceeded" routes to halving, not backoff — this *matches* the plan ground truth (02-02-PLAN Task 1, 02-RESEARCH Pitfall 1: Infura documents it as the second `-32005` cap shape), even though the review prompt's parenthetical read it as a throttle message. See WR-01 for a real classification defect adjacent to this. |
| 3 | Rewind operator-gated; run only detects+halts; delete strictly above; nothing auto-rewinds | **PARTIAL FAIL** — gating and the `block_number > point` predicate are correct (`rewind.go:206`, delete reachable only via the `rewind` subcommand; grep-verified no other caller), but the re-anchor is a no-op on the real store (**CR-01**), so recovery never completes. |
| 4 | Replay idempotence pre-merge and post-OPTIMIZE via FINAL | **PASS** — proven in `clickhouse_test.go` (TestReplayTotalsStableBeforeAndAfterOptimize, TestRewindSameIdentityReinsertNotSwallowed) and the devnet replay; identity key `(chain, token, block_number, tx_hash, log_index)` matches the ReplacingMergeTree ORDER BY. |
| 5 | Checkpoint read `ORDER BY height DESC LIMIT 1`, never FINAL; correctness reads FINAL | **PASS** — `ReadCheckpoint` (`clickhouse.go:246-258`); `Inspect`/`StoredBlockHashes`/`RangeTotals` all use FINAL. (The same max-height read is what makes CR-01 fatal — the schema is right for sync, wrong for rewind.) |
| 6 | Exact integers; no credentials in tracked files | **PASS** — `*big.Int` end to end into `UInt256` (2^256-1 round-trip tested); repo-wide grep for Infura keys / proxy addresses / secrets found none; the devnet private key is the documented public ethereum-package prefunded fixture. |
| 7 | head_policy finalized-default/confirmed-enum validated at load; start_block respected | **PASS** — `Load` defaults empty to `finalized`, `Validate` enforces the enum and confirmed-requires-depth; seeding honors `start_block`/`-from` including the above-head rejection (`sync.go:93-111`). See WR-02 for the runtime downgrade edge. |

## Critical Issues

### CR-01: Rewind re-anchor is invisible to ReadCheckpoint — the recovery path never completes on the real store

**File:** `internal/indexer/rewind.go:209` (with `internal/storage/clickhouse.go:168-180,246-258`, `migrations/clickhouse.sql:41-49`)
**Issue:** `Rewind` re-anchors by inserting a checkpoint row at the rewind point
(a *lower* height than the existing rows). `indexer_checkpoint` is
`ReplacingMergeTree(height)` with `ORDER BY (chain)`, and `ReadCheckpoint`
returns `ORDER BY height DESC LIMIT 1`. Every prior window advance left a
physical row at a height above the rewind point, so:

- pre-merge, the ordered read returns the old (orphaned-chain) checkpoint row;
- post-merge, the ReplacingMergeTree survivor is the *greatest* height — the
  same old row (this max-height-wins behavior is the migration's own documented
  design, and `TestCheckpointOrderedReadPreMerge` pins it).

Empirically reproduced on the pinned server (26.8.20.9) with the production
schema and the production read query:

```text
after-sync-read          120  0xhashA120
pre-merge-after-rewind   120  0xhashA120     # re-anchor insert at 100 exists but loses
post-merge-after-rewind  120  0xhashA120     # survivor is still the max height
```

Consequence: `run` after `rewind` reads the stale checkpoint (old chain's hash
at the old height), fails the SYNC-05 verification again, and halts — README
§5 step 4 ("restart the indexer; it resumes from the re-anchored checkpoint")
is false on the real store. The operator is stuck in a halt/rewind loop; the
only escape is out-of-band surgery on the checkpoint table. All offline rewind
tests use `fakeEventStore`/`fakeRewindStore` (in-memory overwrite), and
neither `clickhouse_test.go` nor `devnet_test.go` ever calls `indexer.Rewind`
against `*storage.Store`, which is why the suite is green.
**Fix:** delete the superseded checkpoint rows above the point before
re-anchoring (lightweight `DELETE` is immediately visible to non-FINAL reads —
the property the events-table rewind already relies on):

```go
// internal/indexer/rewind.go — RewindStore gains:
DeleteCheckpointAbove(ctx context.Context, chain string, after uint64) error

// internal/storage/clickhouse.go:
func (s *Store) DeleteCheckpointAbove(ctx context.Context, chain string, after uint64) error {
	if err := s.conn.Exec(ctx,
		`DELETE FROM indexer_checkpoint WHERE chain = ? AND height > ?`, chain, after,
	); err != nil {
		return fmt.Errorf("clickhouse delete checkpoint above %d: %w", after, err)
	}
	return nil
}

// internal/indexer/rewind.go, destructive section (before the re-anchor write):
if err := rs.DeleteCheckpointAbove(ctx, Chain, point); err != nil {
	return nil, fmt.Errorf("rewind: delete checkpoint above %d: %w", point, err)
}
if err := rs.WriteCheckpoint(ctx, Chain, point, pointHash); err != nil { ... }
```

Also add an env-gated integration proof against the real store: write
checkpoints 100/110/120, run `Rewind` to 100, assert `ReadCheckpoint` returns
(100, canonical hash) both immediately and after
`OPTIMIZE TABLE indexer_checkpoint FINAL`.

## Warnings

### WR-01: `isRateLimitErr` matches the bare substring "429" — block numbers and hashes in provider error text get misrouted into retry backoff

**File:** `internal/rpc/client.go:60`
**Issue:** `strings.Contains(s, "429")` fires on any error text containing
"429" anywhere — including a block number (`headerByNumber` embeds
`block %d`; heights 429, 4290-4299, 14290x… all match), a tx/block hash
fragment quoted by the provider, or unrelated codes. A *permanent* error then
burns the full bounded backoff (2+4+8+16 = 30s) before failing, stalling the
sync loop on a non-transient failure; conversely a genuine non-429 permanent
throttle-shaped message is fine, but the loose disjunct widens the
misclassification surface the careful `-32005` split was built to close
(02-RESEARCH Pitfall 1 / T-02-07).
**Fix:** Match the throttle by code and explicit phrases, not a bare digit
run — e.g. keep `"Too Many Requests"`, `"Rate limit exceeded"`,
`"temporarily unavailable"`, and match the status as `" 429"`/`"429 "` or
parse `err` for `*ethrpc.JsonError`/HTTP status instead of substring:
```go
return strings.Contains(s, "Too Many Requests") ||
	strings.Contains(s, "Rate limit exceeded") ||
	strings.Contains(s, "temporarily unavailable") ||
	strings.Contains(s, "429 Too Many Requests")
```

### WR-02: A single transient error in the finalized-tag probe downgrades the whole run to confirmed semantics

**File:** `internal/rpc/client.go:148-151` (with `internal/indexer/sync.go:75-79`)
**Issue:** `SupportsHeadTag` treats *any* error — including a one-off network
failure or 5xx at startup — as "tag unsupported", and `RunSync` then
downgrades `head_policy` from finalized to confirmed (depth 20) for the entire
run. The downgrade is logged loudly but the process proceeds; a run silently
operating on a much weaker reorg margin is exactly the "silent downgrade"
class the A1 research set out to prevent. (The checkpoint-verification halt
still catches actual displacement, so this is robustness, not corruption —
but the trigger conflates "capability missing" with "request failed".)
**Fix:** Classify the probe error: only a method-not-found/not-supported JSON-RPC
error (`-32601`/`"not supported"` message) should report unsupported;
transport/5xx errors should fail startup (or retry the probe) instead of
changing head policy.

### WR-03: Post-destructive failure in Rewind discards the report — the operator cannot see what was deleted

**File:** `internal/indexer/rewind.go:214-217`
**Issue:** The destructive steps (`DeleteEventsFrom`, checkpoint re-anchor)
run before the after-totals read. If that `RangeTotals` call fails, `Rewind`
returns `(nil, err)`: the CLI prints only "rewind failed" and exits non-zero,
with no report of the rewind point, the delete that *did* execute, or the
before-totals — precisely the observability the report exists to provide on
the one path where data was just destroyed.
**Fix:** Keep the report and surface it even when the final measurement fails:
```go
report.After = RangeTotalsResult{} // or a Partial flag
if afterN, afterSum, err := rs.RangeTotals(ctx, Chain, point+1, cpHeight); err != nil {
	return report, fmt.Errorf("rewind: delete and re-anchor completed at %d, but after-totals failed: %w", point, err)
} else { ... }
```
and have `rewindCommand` print the report for any non-nil `report` before
exiting on error.

## Info

### IN-01: README §4 stale claim and duplicated section number

**File:** `README.md:102-108,156,177`
**Issue:** The last §4 bullet tells operators dense USDC ranges "may still
need a smaller `window_blocks` **until automatic window halving lands**" —
halving landed in this very phase (`FetchLogsResilient`, fail-closed floor);
the guidance is out of date and discourages relying on shipped behavior. Also
two consecutive sections are numbered "## 6." (Inspection and Tests).
**Fix:** Rewrite the bullet to describe automatic halving down to
`window_floor` with the fail-closed error; renumber the Tests section to 7
and Localnet to 8 (the §7/§8 references shift accordingly).

### IN-02: EnsureSchema and the default config path are CWD-relative

**File:** `internal/storage/clickhouse.go:59,64-74`; `cmd/indexer/main.go:30`
**Issue:** `migrationCandidates` ("migrations/clickhouse.sql" /
"../../migrations/clickhouse.sql") and the default `config/tokens.json` only
resolve when the binary runs from the repo root (or `internal/storage` in
tests). Any deployment that runs the built binary from another directory fails
at startup with "clickhouse migration not found".
**Fix:** Accept overrides (`INDEXER_MIGRATIONS`, `INDEXER_CONFIG`) or embed
the DDL via `go:embed` so the binary is location-independent.

### IN-03: Load silently coerces explicit invalid zero values into defaults

**File:** `internal/config/config.go:48-60`
**Issue:** `Load` replaces JSON zero values with defaults before `Validate`
can see them, so a config that explicitly sets `"window_floor": 0` or
`"window_blocks": 0` (a mistake an operator would want rejected) silently
becomes 100/5000 instead of failing loudly — undercutting Validate's
fail-fast contract for everything except the enum fields.
**Fix:** Default only when the key is absent (pointers, or a raw map
presence check), and let explicit zeros reach `Validate`.

### IN-04: Interrupt during backoff is misreported as a throttle failure

**File:** `internal/rpc/client.go:70-78`
**Issue:** When `ctx` is cancelled mid-backoff, `withRateLimitRetry` returns
the original throttle error, not `ctx.Err()`; `scrubURL` therefore also drops
the cancellation bit, and `main`'s `errors.Is(err, context.Canceled)` path
logs "sync stopped" with a confusing throttle message instead of
"sync stopped (interrupt)".
**Fix:** `case <-ctx.Done(): return ctx.Err()` (or wrap the throttle error
with `ctx.Err()` so `errors.Is` still matches).

### IN-05: FetchLogsResilient has no guard against window == 0

**File:** `internal/rpc/client.go:241-255`
**Issue:** With `window == 0`, the first sub-range computes `end = start - 1`
(an inverted range sent to the provider). Unreachable today because
`Validate` requires `window_blocks > 0`, but the function also self-defends
`floor < 1` one line above — the same defense is missing for the other bound.
**Fix:** `if window < 1 { window = 1 }` next to the floor clamp.

### IN-06: Idle loop skips checkpoint verification whenever cpHeight >= head — including cpHeight > head

**File:** `internal/indexer/sync.go:113-123`
**Issue:** Skipping verification while parked at the head is the documented
02-RESEARCH OQ-1 resolution for `cpHeight == head`. But the `>=` also covers
`cpHeight > head` (provider finalized-head regression, or a downgrade to
confirmed after the checkpoint was written higher): the loop then sleeps
indefinitely without ever running the mismatch check, deferring detection of
exactly the catastrophic event SYNC-05 exists to detect — possibly forever if
the provider never catches back up.
**Fix:** Verify the checkpoint on the idle path too (one header read per
poll interval), or at least when `cpHeight > head` (the abnormal direction),
where the cost is one RPC call per minute.

### IN-07: Confirmed rewind plan can differ from the executed plan

**File:** `cmd/indexer/main.go:193-201`
**Issue:** After the operator confirms, `Rewind` is re-invoked and recomputes
the entire walk; if the provider's chain moved during the interactive prompt,
the executed rewind point/delete range can differ from the numbers the
operator just confirmed (the final printed report shows the executed plan,
but nothing flags the divergence).
**Fix:** Pass the confirmed point back as a verified `To` on the second call
(it is already verification-checked), or diff confirmed vs executed
`RewindPoint` and abort/warn when they differ.

### IN-08: Inspect has no chain predicate

**File:** `internal/storage/clickhouse.go:135-142`
**Issue:** `Inspect` filters only on `block_number BETWEEN ? AND ?`, while
every other correctness read (`StoredBlockHashes`, `RangeTotals`) filters
`chain = ?`. Harmless while the MVP is single-chain, but a second chain key
would silently mix rows into the debug view.
**Fix:** Add `AND chain = ?` for symmetry.

---

_Reviewed: 2026-10-10T09:45:09Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
