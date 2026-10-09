# Phase 2: Reliable Canonical Indexing - Research

**Researched:** 2026-10-09
**Domain:** Continuous Ethereum head-following sync, durable ClickHouse checkpointing, replay idempotence, reorg rewind; Kurtosis localnet + httptest reorg harness test environments
**Confidence:** HIGH (core storage/RPC claims verified by live probes against the pinned ClickHouse 26.8.20.9 container, a running Kurtosis devnet, three public Mainnet RPCs, and the pinned go-ethereum v1.17.7 module cache)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Default sync head is `finalized`; `confirmed` (latest − confirmation_blocks) is a config option (`head_policy: finalized | confirmed` in tokens.json, default `finalized`; `confirmation_blocks` applies only when `confirmed`).
- **D-02:** Kurtosis local Ethereum testnet as this phase's integration environment for reorg/restart behavior: minimal `ethereum-package` enclave configured to allow competitive-fork reorgs, real TetherToken contract source deployed for identical event semantics, env-gated integration tests (same opt-in pattern as CLICKHOUSE_URL/ETH_RPC_URL). Mainnet pinned fixtures remain the classification anchor. **Scope guard:** if Kurtosis setup exceeds ~1 plan of effort, fall back to a simulated reorg harness (httptest RPC serving two conflicting chains) and record the shortfall.
- **D-03:** Operator-gated rewind, not automatic. On checkpoint hash mismatch the indexer stops ordinary indexing/reporting and emits a clear diagnostic (expected vs actual hash, affected block range); a `rewind` subcommand performs the verified rewind and is run by the operator.
- **D-04:** Continuous sync resumes from the checkpoint; on first run with no checkpoint it starts at the current eligible head and indexes forward only. Historical backfill is an explicit opt-in: `start_block` in config (or a CLI flag) seeds the checkpoint at that block.
- **D-05:** Fixed default window (e.g. 5,000 blocks) per getLogs per token, with deterministic halving-and-retry on a result-cap/rate-limit error down to a floor, then advance.

### Claude's Discretion
- Checkpoint table shape (`indexer_checkpoint` per research sketch: chain, height, hash, updated_at — advanced only after the range's events are accepted).
- Poll cadence, range-batching details, metrics/logging granularity (OPER-02 full health output is Phase 4).
- Which of IN-01..IN-08 review findings get fixed in passing (fix IN-03 Tokens[0] guard and IN-08 Close/duplicate-contract check opportunistically; others stay recorded).

### Deferred Ideas (OUT OF SCOPE)
- A4 deployed USDT zero-address Transfer fixture — already ROADMAP Backlog 999.1; fold into this phase's research only if trivially locatable, else stays backlog. (Not trivially locatable this session — stays backlog.)
- Prometheus/health metrics for sync lag — Phase 4 (OPER-02).
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SYNC-01 | Bounded ranges advance only through the configured confirmed-or-finalized head; continue as new eligible blocks arrive | go-ethereum v1.17.7 serves `finalized`/`safe` tags via `HeaderByNumber` [VERIFIED: module cache]; live behavior confirmed on the Kurtosis devnet and on three public Mainnet RPCs; head resolution + window loop pattern below |
| SYNC-02 | Retry transient RPC failures/rate limits without skipping a range or advancing the checkpoint | Existing bounded backoff in `internal/rpc` [VERIFIED: internal/rpc/client.go:26-53]; `-32005` message discrimination (result-cap vs throttle) documented from Infura docs + live observations |
| SYNC-03 | Resume after restart from a durable block-number-and-hash checkpoint that advances only after events are durably accepted | Checkpoint table design verified by live probes on 26.8.20.9 (ordered read pre-merge, version-column survivor, crash-between-writes recipe) |
| SYNC-04 | Replaying a processed range leaves logical counts/sums unchanged, even before background merges | FINAL-read idempotence re-probed live this session on scratch tables, pre- and post-`OPTIMIZE ... FINAL`, including the same-identity re-insert edge |
| SYNC-05 | Detect checkpoint block-hash mismatch; verified rewind excludes orphaned events and rebuilds affected derived data | Lightweight-DELETE rewind recipe probe-verified (immediate visibility, FINAL-safe, re-insert-safe, merge-safe); rewind subcommand UX per D-03; deterministic two-chain httptest harness specified |
</phase_requirements>

## Summary

Phase 2 turns the Phase 1 bounded proof into a continuous loop, and every load-bearing mechanism it needs was verified live in this session. Storage: on the pinned ClickHouse 26.8.20.9 container, a checkpoint table read with `ORDER BY height DESC LIMIT 1` is correct before background merges, `ReplacingMergeTree(height)` makes the merged survivor the greatest height even under out-of-order writes, and lightweight `DELETE FROM ... WHERE block_number > N` excludes orphans immediately, stays correct under FINAL reads, and does not swallow or resurrect a re-inserted row with the same event identity — before and after forced merges. RPC: go-ethereum v1.17.7 (already in go.mod) sends the `finalized`/`safe` tags, and the tags were exercised live against both the Kurtosis devnet and three public Mainnet providers with consistent behavior (finalized lagging latest by 1–2 epochs). Infura documents the exact `-32005` error shapes: the 10k result cap (`"query returned more than 10000 results"`) and the 10-second timeout (`"query timeout exceeded"`) share a code with rate limiting, so D-05's halving must discriminate on message text, not code — the current `isRateLimitErr` [VERIFIED: internal/rpc/client.go:29-38] matches `-32005` generically and would misroute the result cap into backoff.

Test infrastructure is largely already provisioned. A Kurtosis `eth-devnet` enclave (single geth + lighthouse, chain 3151908) is RUNNING on this machine with working `finalized`/`safe` tags and the standard ethereum-package 21 prefunded keys (account 0 `0x8943545177806ED17B9F23F0a21ee5948eCaa776` verified funded at nonce 0, public test key). Deterministic reorg forcing is NOT a built-in ethereum-package capability [VERIFIED by search, no knob exists], so D-02's scope guard applies: the reorg detection/rewind path is tested on the httptest fake-RPC harness (two conflicting chains — an extension of the existing `cannedRPC` pattern), while the localnet covers restart/replay/finalized-head-following with real consensus-driven finality. The real TetherToken creation bytecode (11,900 bytes, deployment tx `0x2f1c5c2b...` at block 4,634,748) was recovered this session via archive RPC (eth.drpc.org) and can be committed as a test fixture and deployed verbatim on the devnet with `bind/v2.DeployContract` — no solc needed, satisfying D-02's "identical event semantics" without a compiler dependency.

**Primary recommendation:** extend the existing packages (`internal/rpc` head resolution + halving, `internal/storage` checkpoint + rewind deletes, `cmd/indexer` continuous loop + `rewind` subcommand), keep zero new dependencies, gate devnet tests on a new env var (e.g. `DEVNET_RPC_URL`), and commit the USDT creation-code fixture for the localnet end-to-end test.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Head policy resolution (finalized/safe tag vs latest−conf) | Go indexer (internal/rpc) | RPC provider | Tag semantics are consensus-level facts the provider serves; the indexer owns translating config into the right tag call |
| Window batching + result-cap halving | Go indexer (internal/rpc + sync loop) | — | Stateless fetch-loop policy (D-05); provider caps are per-request |
| Event decode/classification | Go indexer (internal/indexer) | — | Unchanged from Phase 1; decoder is token-specific by design |
| Durable checkpoint state | ClickHouse (storage tier) | Go indexer (write ordering) | Checkpoint must share the failure domain with events to make "advance after accept" one system property |
| Replay idempotence | ClickHouse (ReplacingMergeTree + FINAL) | Go indexer (deterministic identity) | Replacement engine owns dedup; indexer owns stable identity keys |
| Reorg detection | Go indexer (on resume + before window ingest) | RPC provider | Hash comparison is an indexer decision; provider supplies canonical header |
| Orphan exclusion + rewind | ClickHouse (lightweight DELETE) | Go indexer (`rewind` subcommand) | Verified delete semantics; operator gate is CLI UX |
| Reorg test environment | Test tier (httptest harness) | Kurtosis localnet | Deterministic two-chain serving is only achievable offline; localnet proves real finality/restart behavior |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| github.com/ethereum/go-ethereum | v1.17.7 (pinned in go.mod) | ethclient head tags, header fetch, contract deployment | Already the Phase 1 RPC layer; `HeaderByNumber` supports `finalized`/`safe` [VERIFIED: module cache ethclient.go:239-261] |
| github.com/ClickHouse/clickhouse-go/v2 | v2.48.0 (pinned in go.mod) | batch insert, checkpoint insert, FINAL reads, lightweight DELETE exec | Phase 1 storage layer; native protocol driver supports all needed statements |
| Go stdlib (log/slog, flag, encoding/json, time, context) | go 1.27.1 | CLI, config, logging | Project convention: stdlib JSON config, no YAML dependency |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| accounts/abi/bind/v2 (part of go-ethereum) | v1.17.7 | `DeployContract(opts, bytecode, backend, constructorInput)` for the localnet USDT deployment | Only in the devnet integration test / fixture tool |

**Installation:**
```bash
# No new dependencies. Existing go.mod already pins everything.
go mod tidy
```

**Version verification:** `go build ./...` green this session against the pinned go.mod (module `TOkenMonitor`, `go 1.27.1`, `clickhouse-go/v2 v2.48.0`, `go-ethereum v1.17.7`) [VERIFIED: go.mod:1-8]. Offline `go build ./... && go vet ./... && go test ./... -count=1` all pass as baseline [VERIFIED: run this session].

## Package Legitimacy Audit

No new external packages are installed or recommended this phase. All work extends existing pinned dependencies (`go-ethereum v1.17.7`, `clickhouse-go/v2 v2.48.0`) and stdlib.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| (none new) | — | — | — | — | — | N/A |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```
                      config/tokens.json (head_policy, window_blocks, start_block, poll_seconds)
                                     |
                                     v
   +--------------------------- cmd/indexer (continuous loop) ---------------------------+
   |                                                                                      |
   |  1. resolve eligible head:                                                           |
   |     finalized -> HeaderByNumber(finalized tag)          [internal/rpc]               |
   |     confirmed -> BlockNumber() - confirmation_blocks                                  |
   |        |                                                                              |
   |        v                                                                              |
   |  2. read checkpoint: SELECT height, block_hash FROM indexer_checkpoint                |
   |     WHERE chain=? ORDER BY height DESC LIMIT 1          [internal/storage]            |
   |        |                                                                              |
   |        v                                                                              |
   |  3. verify: HeaderByNumber(checkpoint.height).hash == checkpoint.block_hash ?         |
   |        |                        |                                                     |
   |        ok                     MISMATCH --> halt ordinary indexing, loud diagnostic    |
   |        |                                (expected vs observed hash, block, range)     |
   |        v                                       |                                      |
   |  4. window = [checkpoint+1 .. min(checkpoint+window, head)]   (seed: start_block      |
   |        |                                                        or current head, D-04) |
   |        v                                                                              |
   |  5. per token: FetchLogs(window) --on -32005 "more than 10000 results"/              |
   |        |                        "query timeout exceeded"--> halve sub-range, retry    |
   |        |                --on 429/"Too Many Requests"--> bounded backoff, same range   |
   |        v                                                                              |
   |  6. decode (Phase 1 decoder, unchanged) + per-event-block hash assertion (WR-01)     |
   |        |                                                                              |
   |        v                                                                              |
   |  7. INSERT events batch -> only on success: INSERT checkpoint row (height=window.to, |
   |        |                hash=window-end header hash)    [advance-after-accept]        |
   |        v                                                                              |
   |  8. caught up? sleep poll_seconds : loop immediately                                  |
   +--------------------------------------------------------------------------------------+

   Operator-gated recovery (D-03):
   cmd/indexer rewind [--to N]  -> verify canonical chain -> walk to safe ancestor ->
   DELETE FROM stablecoin_events WHERE block_number > rewind_point (lightweight) ->
   re-anchor checkpoint at rewind_point with canonical hash -> print before/after counts
```

### Recommended Project Structure
```
internal/
├── rpc/client.go        # + FinalizedHead/ConfirmedHead resolution; error-classification split
├── storage/clickhouse.go# + Checkpoint read/write, DeleteEventsFrom (rewind), range count/sum helpers
├── indexer/sync.go      # NEW: the continuous window loop (head -> windows -> advance), halving policy
└── indexer/rewind.go    # NEW: ancestor walk + verified rewind procedure (called by CLI)
cmd/indexer/main.go      # subcommands: run (default continuous), rewind
migrations/clickhouse.sql# + indexer_checkpoint table
config/tokens.json       # + head_policy, window_blocks, window_floor, poll_seconds, start_block
internal/rpc/client_test.go      # + two-chain reorg harness, -32005 message discrimination tests
internal/storage/clickhouse_test.go # + checkpoint/crash-between-writes/replay/rewind integration
internal/indexer/sync_test.go    # NEW: loop behavior against canned RPC
internal/indexer/devnet_test.go  # NEW: env-gated localnet suite (DEVNET_RPC_URL)
testdata/usdt_creation.hex       # NEW: mainnet creation bytecode fixture (11,900 bytes)
```

### Pattern 1: Head resolution by policy (D-01)
**What:** One method maps config to the correct RPC call.
**When to use:** every poll iteration and every checkpoint verification.
```go
// Source: go-ethereum v1.17.7 module cache (verified this session)
// rpc/types.go:67-71:
//   EarliestBlockNumber  = BlockNumber(-5)
//   SafeBlockNumber      = BlockNumber(-4)
//   FinalizedBlockNumber = BlockNumber(-3)
//   LatestBlockNumber    = BlockNumber(-2)
//   PendingBlockNumber   = BlockNumber(-1)
//
// ethclient.go:239-261 — HeaderByNumber accepts these as big.Int and
// toBlockNumArg (ethclient.go:781-793) serializes negative small ints to the
// tag string via rpc.BlockNumber(...).String() — i.e. "finalized"/"safe".
func (c *Client) EligibleHead(ctx context.Context, cfg *config.Config) (uint64, common.Hash, error) {
    switch cfg.HeadPolicy {
    case "finalized":
        h, err := c.HeaderByTag(ctx, rpc.FinalizedBlockNumber) // wraps ec.HeaderByNumber(ctx, big.NewInt(int64(tag)))
        ...
        return h.Number.Uint64(), h.Hash(), nil
    default: // "confirmed"
        head, err := c.ec.BlockNumber(ctx)
        if head > cfg.ConfirmationBlocks { head -= cfg.ConfirmationBlocks }
        ...
    }
}
```

### Pattern 2: Checkpoint advance-after-accept (SYNC-03)
**What:** Two writes in strict order per window; the checkpoint row is only inserted after `InsertEvents` returns nil. A crash between them leaves the window un-checkpointed, so restart re-processes it, and FINAL dedup collapses the replay.
**When to use:** every window.
```sql
-- Recommended DDL (Claude's discretion per CONTEXT; probe-verified semantics):
CREATE TABLE IF NOT EXISTS indexer_checkpoint
(
    chain      String,
    height     UInt64,
    block_hash String,
    updated_at DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(height)   -- version = height: merged survivor is ALWAYS max height
ORDER BY (chain);
```
```go
// Read — correct BOTH pre-merge and post-merge (probe-verified):
//   raw scan showed 3 physical rows pre-merge; ORDER BY height DESC LIMIT 1
//   returned 110 (max) without FINAL; after OPTIMIZE the single survivor was 110.
//   ReplacingMergeTree(updated_at) would NOT guarantee this under out-of-order
//   writes (a late height-90 row has the newer timestamp and would win the merge);
//   ReplacingMergeTree(height) does.
row := SELECT height, block_hash FROM indexer_checkpoint WHERE chain = ? ORDER BY height DESC LIMIT 1
```
**Crash-between-writes test design (required by CONTEXT specifics):** inject a failure of the checkpoint write after a successful event insert (storage wrapper or killed process); restart; assert: FINAL count of the range equals the logical set (no doubling), checkpoint eventually advances past the range, no block gap.

### Pattern 3: Logical replay-idempotence verification (SYNC-04)
**What:** The proof query for "replay changes nothing, including before merges".
```sql
-- Logical totals over a range — run after first ingest, after replay insert,
-- and after OPTIMIZE TABLE stablecoin_events FINAL; assert byte-identical results.
SELECT count() AS events, sum(raw_amount) AS total
FROM stablecoin_events FINAL
WHERE block_number BETWEEN ? AND ?;
-- Probe-verified this session (26.8.20.9): after duplicate insert of the same
-- identity rows, FINAL count/sum unchanged; non-FINAL count showed the physical
-- duplicates (2x) — FINAL is mandatory for every correctness read (Phase 1 rule).
```

### Pattern 4: Rewind via lightweight DELETE (SYNC-05, D-03)
**What:** Orphan exclusion = `DELETE FROM stablecoin_events WHERE block_number > ?` (lightweight delete), then re-anchor the checkpoint with the provider's canonical hash at the rewind point, then resume normal indexing.
**Probe-verified chain of evidence (26.8.20.9, this session):** after deleting `block_number >= 101`, a re-inserted event with the SAME identity key (same tx_hash + log_index — the real reorg case where a transaction is re-included in the replacement block) but a NEW block_hash and amount: FINAL read returned exactly the correct logical set; `OPTIMIZE TABLE ... FINAL` changed nothing (no resurrection, no swallow). Lightweight deletes appear in `system.mutations` as `UPDATE _row_exists = 0 WHERE ...` with `is_done` observable. [VERIFIED: live probe]
**Why not `ALTER TABLE ... DELETE` mutations or version supersession:** lightweight DELETE is immediately visible to reads (mutations are async and read-invisible until done), needs no version-column semantics interplay, and the re-insert probe covers the only subtle edge. Phase 2 has no derived aggregates yet — "rebuild affected derived data" is events-only; leave a documented hook for Phase 4 rollups.
**Rewind UX (D-03):** `go run ./cmd/indexer rewind [--to N] [--yes]`: prints expected/observed hash, affected range, row counts before/after; `--to` overrides the auto-detected ancestor; exits non-zero on verification failure. Ancestor walk: from the mismatched checkpoint height, step down while the stored per-row block hashes disagree with canonical headers (bounded, e.g. 1,000 blocks — a displaced finalized checkpoint is a catastrophic/hostile event per D-03 rationale); the first height where stored hashes agree (or no rows exist) is the rewind point.

### Pattern 5: Window halving on provider caps (D-05)
**What:** Message-level error discrimination, then deterministic halving.
```go
// Infura documents BOTH of these as code -32005 (docs.infura.io, retrieved
// 2026-10-09) — the code alone cannot distinguish them from rate limiting:
//   {"code":-32005,"message":"query returned more than 10000 results"}
//   {"code":-32005,"message":"query timeout exceeded"}
// Live-observed rate-limit shapes: 429 / -32005 "Too Many Requests" (Phase 1),
// -32005 "Rate limit exceeded" (eth.merkle.io, this session).
func isResultCapErr(err error) bool {
    s := err.Error()
    return strings.Contains(s, "more than 10000 results") ||
        strings.Contains(s, "query timeout exceeded")
}
// Fetch loop, per token, per window: start at window_blocks (default 5000);
// on isResultCapErr: halve the sub-range and retry the SAME sub-range
// immediately (no backoff — the query was rejected, not throttled); on
// isRateLimitErr: existing bounded backoff {2s,4s,8s,16s}
// [VERIFIED: internal/rpc/client.go:26] with the SAME range; floor at
// window_floor (default 100): below the floor the run fails loudly
// (a provider that caps 100 blocks of one token's supply topics is broken).
// Reset window size to the configured default at each new outer window —
// stateless and predictable (D-05 rationale).
```

### Pattern 6: Localnet USDT deployment (D-02) — no solc required
**What:** Commit the real Mainnet creation bytecode; deploy verbatim on the devnet with the deployed go-ethereum API.
```go
// Source: go-ethereum v1.17.7 accounts/abi/bind/v2/lib.go:225 (verified in module cache):
//   func DeployContract(opts *TransactOpts, bytecode []byte, backend ContractBackend,
//                       constructorInput []byte) (common.Address, *types.Transaction, error)
// It sends append(bytecode, constructorInput...) — feeding the verbatim Mainnet
// deployment-tx input as bytecode (constructorInput empty) re-creates the same
// contract with the original constructor args embedded. ethclient satisfies
// ContractBackend (HeaderByNumber/PendingCodeAt/CallContract/SuggestGasPrice/
// SendTransaction method set verified in module cache).
input := hexFromFile("testdata/usdt_creation.hex") // 11,900 bytes, tx 0x2f1c5c2b...
auth := &bind.TransactOpts{From: devnetFunded, Signer: signer, GasLimit: 3_000_000}
addr, tx, err := bindv2.DeployContract(auth, input, ethCl, nil)
// then call issue()/redeem()/destroyBlackFunds() via ABI-encoded calls or a
// tiny bound contract to mint/burn real events for the sync tests.
```
**Fixture provenance [VERIFIED: recovered live this session via eth.drpc.org]:** deployment tx `0x2f1c5c2b44f771e942a8506148e256f94f1a464babc938ae0690c6e34cd79190` at block 4,634,748, deployer `0x36928500bc1dcd7af6a2b4008875cc336b927d57` (located by binary-searching `eth_getCode` non-emptiness; creation tx confirmed by receipt `contractAddress == 0xdAC17F...ec7`). Topic0s are keccak of the canonical signatures — chain-independent — so the pinned decoder constants [VERIFIED: internal/indexer/decoder.go:18-23] classify localnet events unchanged.

### Anti-Patterns to Avoid
- **Matching `-32005` as "rate limit":** the existing `isRateLimitErr` [VERIFIED: internal/rpc/client.go:29-38] does `strings.Contains(s, "-32005")` — under D-05 the result-cap error must be split out FIRST or halving never triggers and bounded backoff ends the run on a dense range.
- **Reading the checkpoint with FINAL or an un-ordered scan:** FINAL survivor is the max-`version` row; with `updated_at` as version a stale late write wins the merge; an un-ordered scan returns arbitrary physical rows pre-merge. Use `ORDER BY height DESC LIMIT 1`.
- **Advancing the checkpoint before the event insert is acked:** the one ordering rule SYNC-03 exists for; crash-between-writes then skips a batch permanently (pitfall 4).
- **Auto-healing a finalized-checkpoint mismatch:** D-03 — auto-rewind masks a catastrophic consensus or hostile-provider event; stop and require the operator.
- **Per-log inserts in the sync loop:** keep range-batched inserts (Phase 1 pattern) — tiny inserts bloat parts and slow FINAL (pitfalls/performance trap: "tiny ClickHouse inserts per log").
- **Treating `confirmed` (latest − N) as reorg-proof:** confirmation depth lowers risk but proves nothing about canonical finality (pitfall 5) — D-01 defaults to `finalized` precisely for this.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Block-tag head resolution | Custom JSON-RPC calls with tag strings | `ethclient.HeaderByNumber(ctx, big.NewInt(int64(rpc.FinalizedBlockNumber)))` | go-ethereum already serializes the tags; verified in v1.17.7 |
| Retry/backoff | New retry framework | Existing `withRateLimitRetry` + `rateLimitBackoffs` | Already bounded, ctx-aware, proven live in Phase 1 |
| Event dedup | Application-level upsert tables keyed queries | ReplacingMergeTree identity + FINAL reads | Probe-verified; replacement is eventual but FINAL reads are exact pre-merge |
| Orphan exclusion | Tombstone columns / version supersession logic | Lightweight `DELETE FROM ... WHERE` | Probe-verified end-to-end incl. re-insert-after-delete |
| Contract deployment ABI plumbing | Hand-encoded deploy tx | `bind/v2.DeployContract` with verbatim creation code | Exists in the pinned dependency; appends constructor input correctly |
| Reorg simulation in CI | Real competitive-fork localnet orchestration | httptest two-chain canned RPC | ethereum-package has no reorg knob; deterministic offline control is the standard approach for this path |

**Key insight:** every mechanism Phase 2 needs is already owned by a pinned dependency or the existing codebase; the phase is orchestration and ordering, not new plumbing.

## Common Pitfalls

### Pitfall 1: `-32005` is three different errors
**What goes wrong:** halving never fires (cap misrouted to backoff) or backoff never fires (throttle misrouted to halving — retrying a throttled query immediately makes throttling worse).
**Why it happens:** Infura uses `-32005` for result cap, query timeout, and (historically) rate limit; third parties reuse it too (live-observed on eth.merkle.io).
**How to avoid:** discriminate on message: `"more than 10000 results"` / `"query timeout exceeded"` → halve; `"Too Many Requests"` / `"Rate limit exceeded"` / `429` → backoff.
**Warning signs:** a dense-range catch-up run dies after exactly `len(rateLimitBackoffs)` attempts with an unchanged range.

### Pitfall 2: Checkpoint read races merges
**What goes wrong:** restart reads a stale or arbitrary checkpoint row pre-merge.
**Why it happens:** ReplacingMergeTree replacement is asynchronous; multiple physical rows are normal.
**How to avoid:** `ORDER BY height DESC LIMIT 1` (probe-verified correct pre-merge) + `ReplacingMergeTree(height)` so the merged survivor is max-height even under out-of-order writes (probe-verified: late height-90 row does NOT survive the merge).
**Warning signs:** duplicate checkpoint rows in raw scans (normal) but a resumed indexer starting BELOW a previously completed height persistently.

### Pitfall 3: Crash between event insert and checkpoint write
**What goes wrong:** naive "skip what's done" logic gaps the range; naive "redo everything" without dedup doubles counts.
**Why it happens:** two separate storage writes cannot be atomic in ClickHouse.
**How to avoid:** advance-after-accept ordering + replay idempotence (FINAL) + the required crash-injection test (CONTEXT specifics).
**Warning signs:** event count doubles immediately after restart (missing FINAL) or a block gap appears after a storage timeout.

### Pitfall 4: Rewind orphans left in pre-merge parts
**What goes wrong:** after `DELETE FROM`, non-FINAL reads still see orphans; or a re-included transaction's events (same tx_hash/log_index, new block_hash) get swallowed by the deleted row's mask.
**Why it happens:** lightweight deletes are row-mask mutations; interplay with ReplacingMergeTree versions is subtle.
**How to avoid:** probe-verified recipe: lightweight DELETE + FINAL reads + normal re-insert; keep all correctness reads FINAL (existing Phase 1 rule covers this).
**Warning signs:** totals change after `OPTIMIZE TABLE ... FINAL`; same-tx re-included events missing after a rewind.

### Pitfall 5: Chain-ID coupling in tests
**What goes wrong:** devnet integration tests fail the startup probe — `Validate` enforces `chain_id == 1` [VERIFIED: internal/config/config.go:56-58] but the enclave chain is 3151908.
**How to avoid:** tests construct `config.Config` structs directly (the existing `testConfig()` pattern in client_test.go) with the devnet chain id; production config stays Mainnet-only (CHAIN-01 untouched).
**Warning signs:** "chain id 3151908, want 1" in test output.

### Pitfall 6: Header calls per event block during catch-up
**What goes wrong:** backfilling thousands of blocks re-fetches one header per event-bearing block (Phase 1 shape) — slow and credit-hungry.
**How to avoid:** one header per window minimum (the window-end block, whose hash anchors the checkpoint); per-event-block headers only where events exist AND the assertion needs them (WR-01 pattern stays for event blocks, but block_time can come from the window-end header plus per-event fetches as today — keep Phase 1 behavior, add the guaranteed window-end fetch).
**Warning signs:** catch-up slower than ~1 getLogs per token per window plus O(event blocks) headers.

## Code Examples

### Existing machine to reuse (verbatim in-repo values)
```go
// [VERIFIED: internal/rpc/client.go:26]
var rateLimitBackoffs = []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}

// [VERIFIED: internal/rpc/client.go:29-38] — must be SPLIT this phase (Pitfall 1)
func isRateLimitErr(err error) bool {
    if err == nil { return false }
    s := err.Error()
    return strings.Contains(s, "429") ||
        strings.Contains(s, "Too Many Requests") ||
        strings.Contains(s, "-32005") ||
        strings.Contains(s, "temporarily unavailable")
}

// [VERIFIED: migrations/clickhouse.sql:25-26] — event identity Phase 2 replays on:
//   ENGINE = ReplacingMergeTree(created_at)
//   ORDER BY (chain, token, block_number, tx_hash, log_index)

// [VERIFIED: internal/indexer/decoder.go:18-23] — chain-independent topic0s:
//   IssueTopic               0xcb8241adb0c3fdb35b70c24ce35c5eb0c17af7431c99f827d44a445ca624176a
//   RedeemTopic              0x702d5967f45f6513a38ffc42d6ba9bf230bd40e8f53b16363c7eb4fd2deb9a44
//   DestroyedBlackFundsTopic 0x61e6e66b0d6339b2980aecc6ccc0039736791f0ccde9ed512e789a7fbdd698c6
//   TransferTopic            0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef

// [VERIFIED: internal/config/config.go:50]
var supportedSymbols = map[string]bool{"USDT": true, "USDC": true}
```

### Devnet integration-test skeleton (env-gated)
```go
// Gate: DEVNET_RPC_URL (opt-in pattern identical to CLICKHOUSE_URL / ETH_RPC_URL).
// A Kurtosis eth-devnet enclave is RUNNING on this machine: EL RPC http://127.0.0.1:32769,
// chain 3151908, finalized/safe tags live [VERIFIED this session].
func TestDevnetFollowsFinalizedHead(t *testing.T) {
    url := os.Getenv("DEVNET_RPC_URL")
    if url == "" {
        t.Skip("DEVNET_RPC_URL not set; see README (kurtosis enclave inspect <name> for the rpc port)")
    }
    cfg := &config.Config{ChainID: 3151908, HeadPolicy: "finalized", /* ... */}
    // drive two+ finalized-head advances through the sync loop; kill/restart
    // mid-catch-up; assert checkpoint continuity and FINAL idempotence.
}
// Prefunded signer (public ethereum-package test key, verified funded nonce 0):
//   address 0x8943545177806ED17B9F23F0a21ee5948eCaa776
//   key     0xbcdf20249abf0ed6d944c0288fad489e33f66b3960d9e6229c1cd214ed3bbe31
```

### Two-chain httptest reorg harness (the D-02 scope-guard fallback, specified)
**What:** a `cannedRPC` extension that serves two conflicting chains and flips between them: chain A (blocks N..N+k, hash series HA) then chain B (same heights N..N+j, hash series HB, j < k, different logs at overlapping heights). Tests: (1) resume-time mismatch → run halts with diagnostic naming expected/observed hash and block; (2) `rewind` to the common ancestor → orphaned B-excluded events removed, re-indexed canonical events stored once; (3) WR-01 per-event assertion fires inside a window. The Phase 1 `cannedRPC` mux pattern [VERIFIED: internal/rpc/client_test.go:32-78] extends directly (it already injects `-32005` bodies and arbitrary results per method).

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `latest`-minus-N confirmations | `finalized`/`safe` RPC tags | Post-Merge (2022), supported across major providers | D-01 defaults to finalized; confirmation depth becomes an opt-in policy |
| Mutation-based deletes (`ALTER TABLE ... DELETE`) | Lightweight `DELETE FROM` | ClickHouse 22.8+ | Immediate read visibility; probe-verified FINAL-safe rewind recipe |
| go-ethereum `bind.DeployContract` (abi + bytecode + params) | `bind/v2.DeployContract(opts, bytecode, backend, constructorInput []byte)` | v2 package in 1.16/1.17 era | Verbatim creation-code deployment without ABI plumbing |
| kurtosis-tech/ethereum-package | ethpandaops/ethereum-package (repo moved) | 2024-2025 | Use the ethpandaops import path when provisioning |

**Deprecated/outdated:**
- Nothing in the current pinned stack is deprecated for this phase's scope.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The operator's Infura endpoint serves `finalized`/`safe` tags on Mainnet (docs list the tags; live probe from this shell was impossible — the LAN proxy was unreachable this session) | Pattern 1 | Head resolution errors at startup; fallback: probe the tag during the fail-fast probe and fall back to `confirmed` with a loud warning |
| A2 | The running `eth-devnet` enclave is reproducible via default-ish `kurtosis run github.com/ethpandaops/ethereum-package` args (exact user args not recorded; package defaults produce 1×geth+lighthouse + the standard 21 prefunded keys, which the enclave exhibits) | Environment Availability | Re-provision with a minimal params JSON; one command, verified services list recorded in this research |
| A3 | USDT deployment block 4,634,748 / tx `0x2f1c5c2b...` — recovered live via archive RPC bisection this session; treat the saved 11,900-byte input in `/tmp/usdt_deploy.json` as needing re-extraction if lost (one-command recipe recorded) | Pattern 6 | Re-run the bisection (documented) against eth.drpc.org or rpc.flashbots.net |
| A4 | Mainnet finalized-head lag stays ~1–2 epochs (observed 68 blocks) — poll cadence default of 60s assumes this | Pattern 1 | Cosmetic only: slower head pickup; config-tunable |
| A5 | `eth_getBlockByNumber` costs 80 credits per call on Infura's metering (docs page states it) — poll-cadence credit math | Pattern 1 / cadence | Free-tier exhaustion if polled aggressively; default 60s keeps it ~115k credits/day worst case; operator can lengthen |
| A6 | TetherToken creation gas fits the devnet block gas limit (0x3938700 = 60M; legacy creation tx gas unknown-but-far-below) | Pattern 6 | Set explicit GasLimit on TransactOpts (3M) and assert mined receipt status |

## Open Questions

1. **Should mid-run checkpoint verification run every poll or only before window ingest?**
   - What we know: SYNC-05 wording covers detection generally; restart-time verification is mandatory; per-window verification costs one `eth_getBlockByNumber`.
   - What's unclear: poll-time credit budget vs detection latency tradeoff for the operator's tier.
   - Recommendation: verify at resume AND before each window ingest (skip when idle at head); cheap during catch-up (1/window), ~0 when idle.
2. **Does the operator want the devnet suite wired into the default `go test ./...` experience?**
   - What we know: existing gates skip cleanly without env.
   - Recommendation: keep opt-in (`DEVNET_RPC_URL`), document the one-liner in README §6, consistent with CLICKHOUSE_URL/ETH_RPC_URL.
3. **Window-floor failure behavior** — hard stop vs skip-and-flag when even `window_floor` blocks trip the cap.
   - Recommendation: hard stop (fail closed); a range that cannot be fetched must never be silently skipped (SYNC-02's "without skipping a range").

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Docker | ClickHouse + Kurtosis | ✓ | 29.1.3 | — |
| ClickHouse container `tm-clickhouse` | storage integration tests | ✓ (restarted this session) | 26.8.20.9 | — |
| Kurtosis CLI + engine | devnet provisioning/inspection | ✓ | 1.20.0 | — |
| Kurtosis enclave `eth-devnet` | localnet integration tests | ✓ RUNNING (EL RPC 127.0.0.1:32769, chain 3151908) | geth+lighthouse, up 6h | re-provision: `kurtosis run github.com/ethpandaops/ethereum-package --enclave <name>` |
| Go toolchain | everything | ✓ | go1.27.1 linux/amd64 | — |
| solc | TetherToken compile | ✗ not needed | — | verbatim creation-bytecode fixture (Pattern 6) |
| Public archive RPC (eth.drpc.org, rpc.flashbots.net) | one-time fixture extraction | ✓ (verified live) | — | operator's Infura via proxy at execution time |
| Public full RPC (ethereum-rpc.publicnode.com) | head-tag probes | ✓ (pruned <15.5M) | — | drpc/flashbots |
| mainnet.infura.io from this shell | live provider probes | ✗ (direct and proxy both failed this session) | — | executor historically reaches it via proxy env prefix; docs-based claims tagged [CITED] |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** Infura live probes (fallback: docs + public-RPC probes already done).

Note: `stablecoin_events` is currently EMPTY (0 rows) in the dev container [VERIFIED this session] — Phase 1 smoke data was cleared; `EnsureSchema` is idempotent and Phase 2 tests re-seed.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib), no framework dependency |
| Config file | none needed (per-package _test.go files; env-gated integration suites) |
| Quick run command | `env -u ETH_RPC_URL -u CLICKHOUSE_URL go test ./... -count=1` |
| Full suite command | `CLICKHOUSE_URL=clickhouse://default@127.0.0.1:9000/default go test ./... -count=1` (plus opt-in `ETH_RPC_URL` for TestLive, `DEVNET_RPC_URL` for devnet suite) |

Baseline verified green this session (offline build/vet/test all `ok`).

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SYNC-01 | Head resolution per policy (finalized tag / latest−conf); loop only advances through it | unit (cannedRPC tag responses) + integration (devnet two finalized advances) | `go test ./internal/rpc ./internal/indexer -run 'Head\|Follows' -count=1` | ❌ Wave 0 |
| SYNC-01 | Halving on 10k-cap message; backoff on throttle message; neither skips nor advances on failure | unit (cannedRPC injects both -32005 messages) | `go test ./internal/rpc -run Cap -count=1` | ❌ Wave 0 |
| SYNC-02 | Range retried (not skipped, checkpoint unmoved) under injected throttle | unit | `go test ./internal/indexer -run Retry -count=1` | ❌ Wave 0 |
| SYNC-03 | Checkpoint roundtrip; advance only after accepted insert | integration (real ClickHouse) | `go test ./internal/storage -run Checkpoint -count=1` | ❌ Wave 0 |
| SYNC-03 | Crash between event insert and checkpoint write → restart leaves one logical event set, no gap, checkpoint then advances | integration (injected checkpoint-write failure) | `go test ./internal/storage -run CrashBetween -count=1` | ❌ Wave 0 |
| SYNC-04 | Replay of processed range: FINAL count/sum identical pre-merge and post-OPTIMIZE | integration | `go test ./internal/storage -run Replay -count=1` (extends existing duplicate-insert test) | ✅ partial (TestReplayDuplicateInsertKeepsFinalCountAtOne) — extend with sum + OPTIMIZE + crash variant |
| SYNC-05 | Checkpoint hash mismatch on resume halts with specific diagnostic | unit (two-chain harness) | `go test ./internal/indexer -run Mismatch -count=1` | ❌ Wave 0 |
| SYNC-05 | Rewind excludes orphans (incl. same-identity re-insert), re-anchors checkpoint, counts reported | integration + unit | `go test ./internal/storage ./internal/indexer -run Rewind -count=1` | ❌ Wave 0 |
| D-02 | Devnet: restart/resume continuity + replay against real finalized head | integration (env-gated) | `DEVNET_RPC_URL=... go test ./internal/indexer -run TestDevnet -count=1` | ❌ Wave 0 |
| D-02 | Localnet end-to-end with real TetherToken bytecode (deploy, issue/redeem, sync, replay) | integration (env-gated, fixture) | `DEVNET_RPC_URL=... go test ./internal/indexer -run TestDevnetUSDT -count=1` | ❌ Wave 0 (needs testdata fixture) |
| CHAIN-01 regression | Config extensions (head_policy/window/poll/start_block) validate; Mainnet-only rule intact | unit | `go test ./internal/config -count=1` | ✅ exists (extend) |
| IN-03/IN-08 | Probe Tokens[0] guard; rpc.Client Close + duplicate-contract validation | unit | `go test ./internal/rpc ./internal/config -count=1` | ✅ files exist (extend) |

### Sampling Rate
- **Per task commit:** `env -u ETH_RPC_URL -u CLICKHOUSE_URL go test ./... -count=1` (offline suite, <10s)
- **Per wave merge:** `CLICKHOUSE_URL=... go test ./... -count=1` (full ClickHouse integration)
- **Phase gate:** full suite + devnet suite + one manual operator runbook walk (continuous run + induced mismatch via test harness) before `/gsd-verify-work`

### Wave 0 Gaps
- [ ] `internal/indexer/sync_test.go` — SYNC-01/02 loop behavior against cannedRPC
- [ ] `internal/storage/clickhouse_test.go` extensions — Checkpoint / CrashBetween / Replay-extend / Rewind (probe recipes in Patterns 2-4 are the test oracles)
- [ ] `internal/rpc/client_test.go` extensions — two-chain harness + -32005 message discrimination
- [ ] `internal/indexer/devnet_test.go` — env-gated DEVNET_RPC_URL suite
- [ ] `testdata/usdt_creation.hex` — bytecode fixture (extraction recipe: tx `0x2f1c5c2b...` input via eth.drpc.org)
- [ ] No framework install needed — existing infrastructure covers all requirements

## Security Domain

`security_enforcement` is enabled (absent = enabled; ASVS level 1, block on high).

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | operator-run CLI; no user auth surface this phase |
| V3 Session Management | no | no sessions |
| V4 Access Control | no (unchanged) | ClickHouse container stays loopback-only bound (Phase 1 T-01-03); no new network surface — the devnet RPC is localhost Kurtosis-mapped |
| V5 Input Validation | yes | `config.Validate` extended for new fields (head_policy enum, window/floor/poll positive bounds, start_block sanity); rewind `--to` bounded and integer-validated; RPC error strings never trusted as data |
| V6 Cryptography | no | no new crypto; devnet test key is a public fixture key, never a secret |
| V12 File/Resource | partially | fixture file path fixed; no user-supplied paths beyond existing config path |

### Known Threat Patterns for Go + ClickHouse + external RPC

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Hostile provider serves fabricated canonical chain (reorg forgery) | Tampering / Spoofing | D-03 stop-then-operator-verify; hash mismatch halts reporting; rewind re-anchors only against provider-canonical headers after the walk; Mainnet pinned fixtures remain the classification anchor (D-02) |
| Credential leak via wrapped RPC errors (AR-01 residual from Phase 1) | Information disclosure | `config.HostOnly` on constructed errors; Phase 2 should scrub `%w`-wrapped `*url.Error` chains in internal/rpc (the deferred redaction Phase 1's 01-SECURITY.md schedules for Phase 2 RPC error handling) — planner should include this |
| SQL injection via rewind bounds | Tampering | bound parameters everywhere (existing storage rule; `DELETE FROM ... WHERE block_number > ?` bound) |
| Untrusted env var (DEVNET_RPC_URL) used as dial target | Spoofing | test-only; no credentials in URL expected; HostOnly hygiene applies |
| Test private key treated as secret | Misuse | public package-default key, documented as fixture material only |

## Sources

### Primary (HIGH confidence — live probes / module cache / in-repo reads this session)
- ClickHouse 26.8.20.9 `tm-clickhouse` container — checkpoint read semantics, ReplacingMergeTree(height) survivor, lightweight DELETE + FINAL + re-insert + OPTIMIZE probes (scratch tables `tm_probe_*`)
- Kurtosis CLI 1.20.0 / engine / enclave `eth-devnet` (UUID e8d1001c4e48) — services, ports, chainId 3151908, finalized/safe/latest live values, genesis alloc (21×2^108-wei accounts, 256 dust precompiles), funded+nonce-0 verification of prefunded account 0
- go-ethereum v1.17.7 module cache — `rpc/types.go:67-71` (block tag constants), `ethclient.go:239-261` + `781-793` (HeaderByNumber tag serialization), `accounts/abi/bind/v2/lib.go:225` (DeployContract v2), ethclient method set for ContractBackend, `accounts/hd.go` (paths, no BIP-39 — established not needed)
- go-ethereum genesis_constants.star (fetched from repo) — 21 prefunded test keys
- Public Mainnet RPCs ethereum-rpc.publicnode.com / eth.drpc.org / rpc.flashbots.net — safe/finalized/latest consistency (26153632 / −36 / −68); archive availability; USDT deployment bisection (block 4,634,748, tx `0x2f1c5c2b...`, 11,900-byte input)
- In-repo: migrations/clickhouse.sql, internal/rpc/client.go, internal/storage/clickhouse.go, internal/config/config.go, internal/indexer/decoder.go, cmd/indexer/main.go, internal/rpc/client_test.go, go.mod (all read this session; quoted values verified)

### Secondary (MEDIUM confidence)
- [Infura eth_getLogs docs](https://docs.infura.io/reference/ethereum/json-rpc-methods/eth_getlogs) — 10,000-result cap, 10-second duration limit, both `-32005` messages, recommended narrowing
- [Infura eth_getBlockByNumber docs](https://docs.infura.io/reference/ethereum/json-rpc-methods/eth_getblockbynumber) — tag list incl. safe/finalized, 80-credit metering
- [ethpandaops/ethereum-package](https://github.com/ethpandaops/ethereum-package) — package defaults, 21 prefunded keys table
- [Ethereum StackExchange: -32005 result cap](https://ethereum.stackexchange.com/questions/86509/ethers-js-why-arent-logs-filtering-query-returned-more-than-10000-results) — exact error JSON shape
- Phase 1 artifacts (01-VERIFICATION.md, 01-RESEARCH.md pitfalls 3-5, 01-REVIEW-DISPOSITION.md IN-03/IN-08) — deferred items this phase closes

### Tertiary (LOW confidence)
- Web search summaries for ethereum-package reorg tooling (no knob found — treated as established absence, not proof of absence; mitigated by the httptest harness being the plan-of-record for reorg determinism regardless)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — zero new deps; everything pinned and live-verified
- Architecture (checkpoint/rewind/idempotence): HIGH — probe-verified on the exact server version
- RPC/head/halving: HIGH for mechanism, MEDIUM for the operator's specific Infura endpoint (docs + third-party probes only; A1)
- Test environment: HIGH for the running enclave's observed behavior; MEDIUM for its exact reprovisioning args (A2)
- Pitfalls: HIGH — each backed by a probe, a doc, or a Phase 1 live observation

**Research date:** 2026-10-09
**Valid until:** 2026-11-08 (stable domain; revisit if ClickHouse major bumps or the enclave is destroyed)

## RESEARCH COMPLETE
