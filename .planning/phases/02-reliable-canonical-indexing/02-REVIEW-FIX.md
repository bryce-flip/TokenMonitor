---
phase: 02-reliable-canonical-indexing
fixed_at: 2026-10-10T10:15:00Z
review_path: .planning/phases/02-reliable-canonical-indexing/02-REVIEW.md
iteration: 1
findings_in_scope: 4
fixed: 4
skipped: 8
status: all_fixed
---

# Phase 2: Code Review Fix Report

**Fixed at:** 2026-10-10T10:15:00Z
**Source review:** .planning/phases/02-reliable-canonical-indexing/02-REVIEW.md
**Iteration:** 1

**Summary:**
- Findings in scope (critical_warning): 4
- Fixed: 4 (CR-01, WR-01, WR-02, WR-03)
- Skipped: 8 (all Info severity — outside the critical_warning fix scope)

## Fixed Issues

### CR-01: Rewind re-anchor is invisible to ReadCheckpoint — the recovery path never completes on the real store

**Files modified:** `internal/storage/clickhouse.go`, `internal/indexer/rewind.go`, `internal/indexer/reorg_test.go`, `internal/indexer/rewind_integration_test.go` (new file)
**Commit:** 1f50cf9
**Applied fix:**
- Added `Store.DeleteCheckpointAbove(ctx, chain, after)` — lightweight `DELETE FROM indexer_checkpoint WHERE chain = ? AND height > ?` (bound params), mirroring the events-delete visibility contract and its strictly-above rule.
- Extended `RewindStore` with `DeleteCheckpointAbove`; `Rewind` now calls it inside the destructive section AFTER `DeleteEventsFrom` and BEFORE the re-anchor `WriteCheckpoint`, so the orphaned chain's higher checkpoint rows cannot keep winning the `ORDER BY height DESC LIMIT 1` read (pre-merge over physical rows, post-merge as the ReplacingMergeTree(height) survivor).
- Rewind safety invariants preserved: still operator-gated (`rewind` subcommand only), deletes strictly above the verified point, checkpoint cleanup uses the same strictly-above bound as the events delete.
- New env-gated integration proof `TestRewindReanchorWinsOnRealStore` (`internal/indexer/rewind_integration_test.go`): fresh per-run ClickHouse database (devnet isolation pattern, `tm_rewind_<pid>`), checkpoints 100/110/120 on the orphaned chain, `indexer.Rewind` to the verified common ancestor 100, then asserts `ReadCheckpoint` returns (100, canonical hash) immediately AND after `OPTIMIZE TABLE indexer_checkpoint FINAL`, plus the strictly-above prohibition (block-100 row survives). Offline: fake gained `DeleteCheckpointAbove` (recorded bounds), asserted in the ancestor-walk test and the rejected-override test.
- **Red-check performed:** with the `DeleteCheckpointAbove` call temporarily disabled, the integration test fails on the real server with exactly the reviewer's empirical result — `post-rewind checkpoint = (120, 0x9266...)` — then passes with the fix restored.

### WR-01: `isRateLimitErr` matches the bare substring "429" — block numbers and hashes in provider error text get misrouted into retry backoff

**Files modified:** `internal/rpc/client.go`, `internal/rpc/client_test.go`
**Commit:** 11dac89
**Applied fix:** Replaced `strings.Contains(s, "429")` with a precompiled token regex `(?:^|[^0-9A-Za-z])429(?:[^0-9A-Za-z]|$)` — the 429 status matches only as a standalone numeric token ("429 Too Many Requests", "HTTP 429", "(429)", "status=429"), never as a digit run inside a block number (4290, 1429001) or an alphanumeric hash fragment (0x429ab...). Kept the phrase disjuncts ("Too Many Requests", "Rate limit exceeded", "temporarily unavailable"); "query timeout exceeded" still routes to the result-cap path (checked first in `FetchLogsResilient`, unchanged). New unit table `TestRateLimitErrMatchesStatusTokenNotDigitRuns` pins both sides; all existing discrimination tests pass unchanged.

### WR-02: A single transient error in the finalized-tag probe downgrades the whole run to confirmed semantics

**Files modified:** `internal/rpc/client.go`, `internal/indexer/sync.go`, `internal/rpc/client_test.go`, `internal/indexer/sync_test.go`
**Commit:** d6a4775
**Applied fix:** `SupportsHeadTag` now returns `(bool, error)`. Only a genuine method-not-found / not-supported rejection (new `isTagUnsupportedErr` classifier: "method not found", "not supported", "unsupported", "does not exist", "not available") reports `(false, nil)` — the legitimate downgrade path, still logged loudly. Transport/5xx/exhausted-throttle failures return the error, and `RunSync` fails the run (`finalized-tag support probe: ...`) instead of downgrading head policy. `ChainReader` seam updated accordingly. New proofs: `TestSupportsHeadTagTrueAndFalse` extended (true / -32601 false / transport error), and `TestRunSyncFailsLoudlyWhenTagProbeFails` (RunSync errors, `head_policy` stays finalized, no downgrade log, nothing ingested); the existing loud-downgrade test still passes.

### WR-03: Post-destructive failure in Rewind discards the report — the operator cannot see what was deleted

**Files modified:** `internal/indexer/rewind.go`, `cmd/indexer/main.go`, `internal/indexer/reorg_test.go`
**Commit:** c96420e
**Applied fix:** `RewindReport` gained `AfterTotalsUnavailable bool`; when the after-totals `RangeTotals` fails after the delete and re-anchor already ran, `Rewind` returns the partial report (rewind point, delete range, before-totals) alongside an error that states exactly what completed ("delete and re-anchor completed at N, but after-totals failed: ..."). `rewindCommand` now prints any non-nil report before exiting on error, and `printRewindReport` renders an explicit "unavailable — the measurement failed; the delete and re-anchor DID run" marker row. New proof `TestRewindReturnsPartialReportWhenAfterTotalsFails` (fake's Nth `RangeTotals` injected to fail): partial report fields, marker, store state shows the delete and re-anchor really ran.

## Skipped Issues

### IN-01: README §4 stale claim and duplicated section number
**Reason:** info severity — outside critical_warning fix scope
### IN-02: EnsureSchema and the default config path are CWD-relative
**Reason:** info severity — outside critical_warning fix scope
### IN-03: Load silently coerces explicit invalid zero values into defaults
**Reason:** info severity — outside critical_warning fix scope
### IN-04: Interrupt during backoff is misreported as a throttle failure
**Reason:** info severity — outside critical_warning fix scope
### IN-05: FetchLogsResilient has no guard against window == 0
**Reason:** info severity — outside critical_warning fix scope
### IN-06: Idle loop skips checkpoint verification whenever cpHeight >= head
**Reason:** info severity — outside critical_warning fix scope
### IN-07: Confirmed rewind plan can differ from the executed plan
**Reason:** info severity — outside critical_warning fix scope
### IN-08: Inspect has no chain predicate
**Reason:** info severity — outside critical_warning fix scope

## Verification

All verification ran in the isolated review-fix worktree
(`rf-02-1126726-1791625849` on branch `gsd-reviewfix/02-1126726`, fast-forwarded
to `main` by the cleanup tail — the numbers are reproducible from the main
checkout after the merge). ClickHouse server: the local pinned `tm-clickhouse`
container (26.8.20.9) on 127.0.0.1:9000. No credentials or proxy addresses
were embedded anywhere (diff scanned).

Per-fix and final gates, all green:

```
go build ./...                                        # OK
go vet ./...                                          # OK
env -u ETH_RPC_URL -u CLICKHOUSE_URL -u DEVNET_RPC_URL \
  go test ./... -count=1                              # ok: config, indexer, rpc, storage

docker start tm-clickhouse; sleep 3
CLICKHOUSE_URL=clickhouse://default@127.0.0.1:9000/default \
  go test ./internal/storage ./internal/indexer -count=1          # full gated suites: ok (2.6s / 7.3s)
CLICKHOUSE_URL=clickhouse://default@127.0.0.1:9000/default \
  go test ./internal/storage ./internal/indexer -count=1 \
  -run 'Rewind|Checkpoint' -v
```

Mandated CR-01 integration suite output:

```
--- PASS: TestCheckpointOrderedReadPreMerge (0.17s)
--- PASS: TestRewindDeleteExcludesOrphansImmediately (0.17s)
--- PASS: TestRewindSameIdentityReinsertNotSwallowed (1.38s)
PASS  ok  TOkenMonitor/internal/storage 1.723s
--- PASS: TestRewindWalksToCommonAncestorAndExcludesOrphans (0.00s)
--- PASS: TestRewindToOverrideRejectedOnDisagreement (0.00s)
--- PASS: TestRewindReturnsPartialReportWhenAfterTotalsFails (0.00s)
--- PASS: TestRewindSameIdentityReinsertPath (0.01s)
--- PASS: TestRewindReanchorWinsOnRealStore (0.74s)      <- the CR-01 proof
--- PASS: TestRunSyncEmptyWindowStillAdvancesCheckpoint (0.01s)
--- PASS: TestRunSyncCrashBetweenInsertAndCheckpoint (0.01s)
--- PASS: TestRunSyncFloorTripStopsRunCheckpointUnmoved (0.00s)
PASS  ok  TOkenMonitor/internal/indexer 0.793s
```

## Per-finding table

| ID | Status | Commit | What changed | Test evidence |
|----|--------|--------|--------------|---------------|
| CR-01 | fixed | 1f50cf9 | `DeleteCheckpointAbove` (storage + RewindStore + Rewind destructive section, before re-anchor); env-gated real-store integration proof | `TestRewindReanchorWinsOnRealStore` PASS on real ClickHouse incl. post-OPTIMIZE; red-check reproduced the bug (120 wins) with the call disabled; offline rewind tests + new cpDeleteBounds assertions green |
| WR-01 | fixed | 11dac89 | 429 matched as standalone status token via regex, phrase disjuncts kept | `TestRateLimitErrMatchesStatusTokenNotDigitRuns` PASS (7 throttle shapes, 5 permanent shapes); all existing throttle/cap discrimination tests unchanged and green |
| WR-02 | fixed | d6a4775 | `SupportsHeadTag` -> (bool, error) with not-supported classifier; RunSync fails loudly on probe error, downgrades only on genuine capability gap | `TestSupportsHeadTagTrueAndFalse` (3 paths) and `TestRunSyncFailsLoudlyWhenTagProbeFails` PASS; existing loud-downgrade test PASS |
| WR-03 | fixed | c96420e | Partial report (`AfterTotalsUnavailable`) + error naming what completed; CLI prints non-nil report before exit; explicit unavailable marker row | `TestRewindReturnsPartialReportWhenAfterTotalsFails` PASS (report fields, marker, delete + re-anchor observable in store state) |

---

_Fixed: 2026-10-10T10:15:00Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
