---
phase: 01-live-supply-event-path
reviewed: 2026-10-08T11:27:18Z
depth: standard
files_reviewed: 13
files_reviewed_list:
  - cmd/indexer/main.go
  - config/tokens.json
  - go.mod
  - internal/config/config.go
  - internal/indexer/decoder.go
  - internal/indexer/decoder_test.go
  - internal/indexer/live_test.go
  - internal/rpc/client.go
  - internal/rpc/client_test.go
  - internal/storage/clickhouse.go
  - internal/storage/clickhouse_test.go
  - migrations/clickhouse.sql
  - README.md
findings:
  critical: 0
  warning: 3
  info: 8
  total: 11
status: issues_found
---

# Phase 1: Code Review Report

**Reviewed:** 2026-10-08T11:27:18Z
**Depth:** standard
**Files Reviewed:** 13 (go.sum excluded as lockfile; consistency verified transitively — build, `go vet`, and all offline tests pass)
**Status:** issues_found

## Summary

Adversarial review of the bounded Mainnet ingest path: config, RPC client with
rate-limit retry, topic-based decode, ClickHouse storage, migration, and the
two test suites (offline + live-fixture). Verification performed beyond
reading:

- All four production topic0 constants plus both USDC paired-event constants
  were recomputed with keccak256 against the event signatures
  (`Issue(uint256)`, `Redeem(uint256)`, `DestroyedBlackFunds(address,uint256)`,
  `Transfer(address,address,uint256)`, `Mint(address,address,uint256)`,
  `Burn(address,uint256)`) — all six match exactly.
- The four correctness invariants were traced end to end. USDT rows can only
  originate from the three USDT topics (`SupplyTopics` pre-filters the
  `eth_getLogs` query; `decodeTransfer` hard-guards `SymbolUSDC`); USDC rows
  only from zero-address Transfers (paired Mint/Burn fall to the default
  `nil` branch); amounts travel as `*big.Int`/UInt256 with no float anywhere;
  no credentials or provider URLs appear in any tracked file (grep clean;
  error paths render through `config.HostOnly`).
- `go vet ./...` clean; offline `go test ./...` passes.
- go-ethereum v1.17.7 semantics checked in the module cache:
  `HeaderByNumber` never returns `(nil, nil)`, so main.go's header map use is
  dereference-safe.

The core is solid. The three Warnings are a provenance gap under reorg
(block_hash is refetched by number instead of using the log's own hash), a
misleading post-ingest inspection table (oldest-20 global, not the ingested
range), and a config-validation gap (empty/typo'd symbol silently skips a
token at runtime instead of failing at startup).

## Critical Issues

None found.

## Warnings

### WR-01: Stored block_hash comes from a by-number header fetch, not the log's own BlockHash — silent provenance mismatch under reorg

**File:** `cmd/indexer/main.go:100-108,121-128`
**Issue:** `types.Log.BlockHash` (returned by `eth_getLogs` and already in
hand) is never used. Instead, one `eth_getBlockByNumber` per distinct block is
issued afterwards and the canonical header's hash is stored
(`BlockHash: h.Hash().Hex()`). If the chain reorgs between the `eth_getLogs`
call and the header fetch (or a reorg deeper than `confirmation_blocks` = 20
invalidates the fetched log itself), the stored `block_hash`/`block_time`
belong to whatever block now occupies that height — not the block that emitted
the log — and nothing detects it. requirements.md's data-correctness
constraint ("tolerate ... short chain reorganizations without corrupting
reported metrics") is met for amounts/event types, but the stored provenance
can silently disagree with the log. The extra header round-trips are also
pure redundant RPC cost for the hash value.
**Fix:** Trust the log's own hash and fail loudly on disagreement:

```go
h := headers[tl.log.BlockNumber]
if h.Hash() != tl.log.BlockHash {
    return nil, fmt.Errorf("reorg detected: block %d header %s does not match log's block hash %s",
        tl.log.BlockNumber, h.Hash().Hex(), tl.log.BlockHash.Hex())
}
// ...
BlockHash: tl.log.BlockHash.Hex(), // the hash the provider attests emitted this log
```

(Keep the header fetch for `block_time`; the equality check turns a silent
mismatch into a retryable failure, consistent with the idempotent re-run story.)

### WR-02: Post-ingest inspection prints the 20 oldest rows in the whole table, not the just-ingested range

**File:** `cmd/indexer/main.go:151-166` (with `internal/storage/clickhouse.go:106-112`)
**Issue:** After printing `stored N supply events for range from-to`, the
command dumps `Inspect(ctx, 20)`, which is `ORDER BY block_number, log_index`
ascending with `LIMIT 20` over the entire table. On any database that already
holds earlier history (a prior run over older blocks), the operator sees rows
unrelated to the run just completed, immediately below a line claiming they
were just stored — including after a `stored 0` run. The README's own
inspection guidance uses `ORDER BY ... DESC LIMIT 20` for "recent" rows. Not
a data-corruption bug (INSERT path is unaffected) but incorrect operator-facing
behavior for the command's stated purpose.
**Fix:** Scope the inspection to the ingested range, newest first:

```go
func (s *Store) Inspect(ctx context.Context, from, to uint64, limit uint32) ([]EventRow, error) {
    ... `SELECT ... FROM stablecoin_events FINAL
          WHERE block_number BETWEEN ? AND ?
          ORDER BY block_number DESC, log_index DESC LIMIT ?`, from, to, limit)
}
```

### WR-03: Config validation accepts empty or typo'd symbol — token silently skipped at runtime instead of rejected at startup

**File:** `internal/config/config.go:47-71`
**Issue:** `Validate` checks duplicate symbols, address format, and decimals,
but never that `t.Symbol` is non-empty (or one of USDT/USDC). A token entry
with a typo'd JSON key (`"symboL"`) yields `Symbol == ""`, passes validation,
and only surfaces at runtime as `no supply topics for token; skipping` on
stderr — while the run reports success. For a supply monitor, a silently
missing token is a data gap that should be a fail-fast config error at the
trust boundary.
**Fix:**

```go
if strings.TrimSpace(t.Symbol) == "" {
    return fmt.Errorf("config: token with contract %s has an empty symbol", t.Contract)
}
```

(Optionally `json.Unmarshal` with a `DisallowUnknownFields` decoder so a typo'd
key is reported instead of silently dropped.)

## Info

### IN-01: Rate-limit detection by substring "429" can false-positive on error text

**File:** `internal/rpc/client.go:29-38`
**Issue:** `strings.Contains(s, "429")` matches any error text containing
"429" anywhere — e.g. an error quoting a block number (`429496...`) or a hash
fragment. Consequence is bounded (up to 4 wasted retries, ~30 s of backoff,
then the original error still surfaces), but it also misses common throttle
wordings ("rate limit", HTTP 503 "Service Unavailable"). Consider matching
`http.StatusText(429)`, "rate limit", and the `-32005` code specifically.
**Fix:** Tighten the patterns; keep the bounded-retry behavior unchanged.

### IN-02: Cancellation during backoff reports the stale throttle error, not ctx.Err()

**File:** `internal/rpc/client.go:42-53`
**Issue:** When `ctx.Done()` fires during a backoff sleep, `withRateLimitRetry`
returns the previous rate-limit error. The run then fails with a misleading
"throttled" message when the real cause was cancellation/timeout. Returning
`ctx.Err()` (wrapped) from that branch makes the failure mode diagnosable.
**Fix:** `case <-ctx.Done(): return fmt.Errorf("rpc retry canceled: %w", ctx.Err())`

### IN-03: Probe indexes cfg.Tokens[0] without a guard

**File:** `internal/rpc/client.go:96`
**Issue:** `Probe` panics with an index-out-of-range on an empty `Tokens`
slice. `main` calls `Validate` first (which enforces non-empty), but `Probe`
is an exported method taking the config directly; one length check removes the
hidden ordering dependency.
**Fix:** `if len(cfg.Tokens) == 0 { return fmt.Errorf("probe: no tokens configured") }`

### IN-04: Decoder classifies by topic0 first, token second — negative guarantee is empirical, not structural

**File:** `internal/indexer/decoder.go:77-107`
**Issue:** The topic switch applies the USDT event shapes regardless of token
symbol: a log from the USDC contract whose topic0 happened to equal
`IssueTopic` would decode as a USDC "mint". Today this is unreachable in
production because `FetchLogs` pre-filters topics per token, and no USDC event
hashes to those topic0s — but the invariant then rests on the fetch-side
filter plus the empirical non-collision of topic0 hashes across the two
contracts. Gating the switch on `token.Symbol` (mirroring `SupplyTopics`)
would make the decoder independently safe, as the address-only live tests
already exercise it with unfiltered logs.
**Fix:** Resolve the expected topic set per token (`SupplyTopics(token)`) and
error/nil on topics outside it before shape decoding.

### IN-05: Migration located by CWD-relative candidate paths

**File:** `internal/storage/clickhouse.go:56-69`
**Issue:** `EnsureSchema` reads `migrations/clickhouse.sql` relative to the
process working directory (`.` or `../../`). Works for `go run` from the repo
root and for the storage tests; a compiled binary invoked from anywhere else
fails with "migration not found". Fine for Phase 1's operator flow, but it will
break first in a container. `go:embed` cannot reach a parent directory, so the
file would need to live beside the package (or the path become a flag).
**Fix when containerizing:** move the DDL into `internal/storage/` and
`//go:embed migrations.sql`, deleting the candidate walk.

### IN-06: Failed Append leaves the prepared batch un-aborted

**File:** `internal/storage/clickhouse.go:87-96`
**Issue:** On an append error the function returns without
`batch.Abort()`/`Send()`, leaving the prepared batch dangling on the
connection. Harmless today because every caller `os.Exit(1)`s immediately, but
the library documents Abort for the error path and a future long-lived caller
would leak batch state.
**Fix:** `defer batch.Abort()` before the append loop (Abort after a
successful Send is a no-op in clickhouse-go v2), or call it in the error branch.

### IN-07: Inspect interpolates LIMIT via fmt.Sprintf

**File:** `internal/storage/clickhouse.go:107-112`
**Issue:** `LIMIT %d` on a `uint32` is not injectable in practice, but the
file already uses bound parameters elsewhere (`clickhouse_test.go:86,110`);
using `LIMIT ?` keeps the pattern uniform and removes the exception a future
editor might generalize incorrectly.
**Fix:** Parameterize the limit.

### IN-08: rpc.Client exposes no Close; Validate does not reject duplicate contracts

**File:** `internal/rpc/client.go:56-68`; `internal/config/config.go:57-69`
**Issue:** Two minor hardening gaps. (a) The wrapper hides `ethclient.Close`;
tests and any future daemon leak the underlying transport until process exit.
(b) Two tokens configured with the same `contract` but different symbols pass
validation and would double-count the same contract's events as two tokens
(the ReplacingMergeTree key includes `token`, so rows would not collapse).
Both are one-line guards.
**Fix:** Add `func (c *Client) Close() { c.ec.Close() }`; track seen contract
addresses (lowercased) in `Validate` and reject duplicates.

---

_Reviewed: 2026-10-08T11:27:18Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
