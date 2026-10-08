---
phase: 01-live-supply-event-path
fixed_at: 2026-10-08T12:07:41Z
review_path: .planning/phases/01-live-supply-event-path/01-REVIEW.md
iteration: 1
findings_in_scope: 3
fixed: 3
skipped: 8
status: all_fixed
---

# Phase 1: Code Review Fix Report

**Fixed at:** 2026-10-08T12:07:41Z
**Source review:** .planning/phases/01-live-supply-event-path/01-REVIEW.md
**Iteration:** 1

**Summary:**
- Findings in scope (critical_warning): 3 (WR-01, WR-02, WR-03)
- Fixed: 3
- Skipped: 8 (all Info-tier, outside the critical_warning fix scope)
- Critical findings: 0 (none reported)

**Verification:** after each fix, `go build ./... && go vet ./... && go test ./...`
ran green in the isolated review-fix worktree
(`.claude/worktrees/rf-01-816369-1791460951`, branch `gsd-reviewfix/01-816369`,
fast-forwarded to `main` on cleanup). Storage/live suites skip env-gated as
documented (CLICKHOUSE_URL / ETH_RPC_URL unset) — that is expected offline
behavior, not a pass being claimed for those paths. All three commits and the
fast-forward are on `main`.

## Fixed Issues

### WR-01: Stored block_hash comes from a by-number header fetch, not the log's own BlockHash — silent provenance mismatch under reorg

**Files modified:** `cmd/indexer/main.go`
**Commit:** c9204df
**Applied fix:** Rows now store `types.Log.BlockHash.Hex()` (the hash the
provider attests emitted the log) instead of the refetched header's hash. The
`eth_getBlockByNumber` fetch is kept for `block_time` only, and an equality
assertion `h.Hash() == tl.log.BlockHash` fails the run loudly on mismatch
(adapted to main's `slog.Error` + `os.Exit(1)` style, since main returns no
error), consistent with the idempotent re-run story. No other behavior change.
**Note:** the reorg-mismatch branch is only observable against a live
provider; recommend a human-verified live run (or a follow-up live test)
before relying on it in anger.

### WR-02: Post-ingest inspection prints the 20 oldest rows in the whole table, not the just-ingested range

**Files modified:** `internal/storage/clickhouse.go`, `cmd/indexer/main.go`, `internal/storage/clickhouse_test.go`
**Commit:** 8198091
**Applied fix:** `Store.Inspect` now takes `(ctx, from, to, limit)` and queries
`WHERE block_number BETWEEN ? AND ? ORDER BY block_number DESC, log_index DESC
LIMIT ?` with all-bound parameters (this also removes the old `LIMIT
%d` interpolation noted in IN-07, as a side effect of the reviewed fix shape).
`main` calls `store.Inspect(ctx, from, to, 20)` so the post-ingest table shows
the just-ingested range, newest first (matching README §5 guidance). The
storage test was updated to the new signature and now asserts newest-first
order and that a row outside the inspected range is excluded.
**Note:** the storage test is env-gated (skips without CLICKHOUSE_URL); its
green path should be confirmed once against the local server.

### WR-03: Config validation accepts empty or typo'd symbol — token silently skipped at runtime instead of rejected at startup

**Files modified:** `internal/config/config.go`, `internal/config/config_test.go` (new file)
**Commit:** 9d50429
**Applied fix:** `Validate` now rejects, at the config trust boundary, tokens
whose symbol is empty/whitespace (typo'd JSON key case) and symbols outside
the supported set `{USDT, USDC}`. The set mirrors `indexer.SupplyTopics` and
lives in `internal/config` because importing `internal/indexer` would create a
cycle (indexer imports config); the comment on `supportedSymbols` records that
tie. A new file `internal/config/config_test.go` was created (reviewer's fix
only covered the code change; the test is the one runnable check for the new
validation branch) — table-driven, offline, passing: empty/whitespace/padded/
unknown symbols each fail with the expected message and the valid config still
passes.

## Skipped Issues

### IN-01: Rate-limit detection by substring "429" can false-positive on error text

**File:** `internal/rpc/client.go:29-38`
**Reason:** info severity — outside critical_warning fix scope
**Original issue:** `strings.Contains(s, "429")` can false-positive on error
text containing "429" anywhere; also misses common throttle wordings.

### IN-02: Cancellation during backoff reports the stale throttle error, not ctx.Err()

**File:** `internal/rpc/client.go:42-53`
**Reason:** info severity — outside critical_warning fix scope
**Original issue:** ctx cancellation during backoff surfaces as a misleading
throttle error instead of the cancellation cause.

### IN-03: Probe indexes cfg.Tokens[0] without a guard

**File:** `internal/rpc/client.go:96`
**Reason:** info severity — outside critical_warning fix scope
**Original issue:** `Probe` panics on an empty `Tokens` slice; ordering
dependency on `Validate` running first.

### IN-04: Decoder classifies by topic0 first, token second — negative guarantee is empirical, not structural

**File:** `internal/indexer/decoder.go:77-107`
**Reason:** info severity — outside critical_warning fix scope
**Original issue:** the topic switch is not gated on token symbol; the
no-cross-classification invariant rests on fetch-side filtering plus topic0
non-collision.

### IN-05: Migration located by CWD-relative candidate paths

**File:** `internal/storage/clickhouse.go:56-69`
**Reason:** info severity — outside critical_warning fix scope
**Original issue:** `EnsureSchema` resolves `migrations/clickhouse.sql`
relative to the process working directory; breaks for a binary invoked
elsewhere / in a container.

### IN-06: Failed Append leaves the prepared batch un-aborted

**File:** `internal/storage/clickhouse.go:87-96`
**Reason:** info severity — outside critical_warning fix scope
**Original issue:** append-error path returns without `batch.Abort()`, leaving
the prepared batch dangling.

### IN-07: Inspect interpolates LIMIT via fmt.Sprintf

**File:** `internal/storage/clickhouse.go:107-112`
**Reason:** info severity — outside critical_warning fix scope (incidentally
resolved by the WR-02 fix: `LIMIT ?` is now a bound parameter)
**Original issue:** `LIMIT %d` interpolation was the one non-parameterized
value in the file.

### IN-08: rpc.Client exposes no Close; Validate does not reject duplicate contracts

**File:** `internal/rpc/client.go:56-68`; `internal/config/config.go:57-69`
**Reason:** info severity — outside critical_warning fix scope
**Original issue:** no `Close` on the wrapper; two tokens sharing a contract
address pass validation and would double-count events.

---

_Fixed: 2026-10-08T12:07:41Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
