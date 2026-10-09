# Phase 2: Reliable Canonical Indexing - Context

**Gathered:** 2026-10-09
**Status:** Ready for planning
**Decision authority:** User delegated all decisions to Claude on 2026-10-09 ("你直接决策就行 我只看最后的成果") — decisions below are Claude's, recorded with rationale for the audit trail. The user reviews final results only.

<domain>
## Phase Boundary

Turn the Phase 1 bounded proof into a continuously running, trustworthy indexer: bounded-range sync that follows a safe head, retry/resume from a durable `(block_number, block_hash)` checkpoint that advances only after events are durably stored, replay-idempotent logical reads, and a verified rewind that excludes orphaned events after a checkpoint hash mismatch. Requirements: SYNC-01, SYNC-02, SYNC-03, SYNC-04, SYNC-05.

Not in this phase: supply arithmetic and `totalSupply()` anchoring (Phase 3); Grafana/Compose/health metrics (Phase 4).

</domain>

<decisions>
## Implementation Decisions

### Sync head policy
- **D-01:** Default sync head is `finalized`; `confirmed` (latest − confirmation_blocks) is a config option (`head_policy: finalized | confirmed` in tokens.json, default `finalized`; `confirmation_blocks` applies only when `confirmed`). Rationale: Phase 2's entire purpose is trustworthy canonical history; an issuance monitor has no latency requirement that justifies ~3-min-head reorg exposure, and a finalized head makes the SYNC-05 rewind a rare backstop instead of a routine path. — **Reversibility:** reversible — one config field + head resolution branch.

### Reorg / restart test environment
- **D-02:** Adopt a Kurtosis local Ethereum testnet as this phase's integration environment for reorg and restart behavior (user-proposed, backlog 999.2): minimal `ethereum-package` enclave configured to allow competitive-fork reorgs, real TetherToken contract source deployed for identical event semantics, env-gated integration tests (same opt-in pattern as CLICKHOUSE_URL/ETH_RPC_URL — gate on a KURTOSIS/localnet RPC env). Mainnet pinned fixtures remain the classification anchor — the localnet is for behavior (reorg/restart/replay), never a substitute for Mainnet classification proof. Scope guard: if Kurtosis setup exceeds ~1 plan of effort, fall back to a simulated reorg harness (httptest RPC serving two conflicting chains — the Phase 1 probe tests already establish this pattern) and record the shortfall. — **Reversibility:** reversible — test infrastructure only.

### Rewind handling
- **D-03:** Operator-gated rewind, not automatic. On checkpoint hash mismatch the indexer stops ordinary indexing/reporting and emits a clear diagnostic (expected vs actual hash, affected block range); a `rewind` subcommand performs the verified rewind (exclude orphaned events at/after the mismatch, rebuild any affected derived state, re-anchor the checkpoint at the last verified block) and is run by the operator. Rationale: a reorg that displaces a finalized-and-checkpointed block is a catastrophic consensus event or a hostile/buggy provider — auto-healing would silently mask it; SYNC-05's own wording ("stops ordinary reporting until a verified rewind…") implies stop-then-verify, and an unattended auto-rewind that trusts the same provider is not "verified". — **Reversibility:** reversible — procedure can be automated later without schema change.

### Backfill & catch-up
- **D-04:** Continuous sync resumes from the checkpoint; on first run with no checkpoint it starts at the current eligible head and indexes forward only. Historical backfill is an explicit opt-in: `start_block` in config (or a CLI flag) seeds the checkpoint at that block. Rationale: forward-only default keeps the MVP honest (Phase 3 establishes a supply anchor anyway — sum-from-genesis is never claimed); backfill is then just "checkpoint seeded lower", no separate code path. — **Reversibility:** reversible — config addition.
- **D-05:** Chunk sizing under provider result caps: fixed default window (e.g. 5,000 blocks) per getLogs per token, with deterministic halving-and-retry on a result-cap/rate-limit error down to a floor, then advance. Rationale: Phase 1 hit Infura's 10,000-result cap at ~200 blocks in dense USDC ranges; halving is stateless, predictable, and reuses the existing bounded-backoff machinery rather than adding adaptive window state. — **Reversibility:** reversible — fetch-loop policy.

### Claude's Discretion
- Checkpoint table shape (`indexer_checkpoint` per research sketch: chain, height, hash, updated_at — advanced only after the range's events are accepted).
- Poll cadence, range-batching details, metrics/logging granularity (OPER-02 full health output is Phase 4).
- Which of IN-01..IN-08 review findings get fixed in passing (fix IN-03 Tokens[0] guard and IN-08 Close/duplicate-contract check opportunistically; others stay recorded).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements
- `.planning/REQUIREMENTS.md` — SYNC-01..SYNC-05 (this phase), plus CHAIN-01 config conventions from Phase 1.

### Phase 1 groundwork
- `.planning/phases/01-live-supply-event-path/01-CONTEXT.md` — locked Phase 1 decisions (D-01 fail-fast probe, D-02 credential handling) this phase builds on.
- `.planning/phases/01-live-supply-event-path/01-01-SUMMARY.md` and `01-02-SUMMARY.md` — what exists: decoder, probe, rate-limit retry, ReplacingMergeTree storage, bounded CLI, README runbook.
- `.planning/phases/01-live-supply-event-path/01-RESEARCH.md` — pitfalls 3-5 (replay duplicates, orphaned events, confirmation-count-is-not-a-reorg-strategy) with the canonical-identity and checkpoint-ordering rules this phase implements.
- `.planning/phases/01-live-supply-event-path/01-VERIFICATION.md` — WR-01 hash-assertion and deferred items feeding this phase (10k-cap chunking, A4 fixture).
- `README.md` — operator runbook to keep accurate as sync behavior lands.

### Project context
- `.planning/ROADMAP.md` — Phase 2 goal and success criteria; Backlog 999.1 (A4 fixture) and 999.2 (Kurtosis localnet, adopted as D-02).

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/rpc/client.go` — HTTP client with probe, bounded getLogs, rate-limit retry with bounded backoff (extend for head policy + chunk halving).
- `internal/indexer/decoder.go` — token-specific classification (unchanged this phase).
- `internal/storage/clickhouse.go` — batch insert, FINAL reads, range-scoped Inspect (add checkpoint table + orphan exclusion).
- `cmd/indexer/main.go` — bounded one-shot loop with WR-01 hash assertion (becomes the continuous loop + `rewind` subcommand).
- `internal/rpc/client_test.go` — httptest fake-RPC pattern (the fallback reorg harness per D-02's scope guard).

### Established Patterns
- Env-gated integration tests (CLICKHOUSE_URL / ETH_RPC_URL opt-in, skip with documented command).
- Exact-integer amounts end to end; full provenance per row; FINAL for all correctness reads.
- Command-prefix env for credentials; never embedded in tracked files.

### Integration Points
- `config/tokens.json` gains head_policy/confirmation semantics; `migrations/clickhouse.sql` gains `indexer_checkpoint`; STATE deferred items 999.1/999.2 connect here.

</code_context>

<specifics>
## Specific Ideas

- Checkpoint advances only after the range's events are durably accepted (SYNC-03) — the crash-between-writes test from research pitfall 3 is a required test.
- Replay of a processed range must leave logical counts/sums unchanged INCLUDING before background merges (SYNC-04) — FINAL reads are the existing mechanism.
- Hash mismatch stop must be loud and specific (expected hash, observed hash, block) — no silent reset (mirrors Phase 3's SUPP-04 philosophy).

</specifics>

<deferred>
## Deferred Ideas

- A4 deployed USDT zero-address Transfer fixture — already ROADMAP Backlog 999.1; fold into this phase's research only if trivially locatable, else stays backlog.
- Prometheus/health metrics for sync lag — Phase 4 (OPER-02).

</deferred>

---

*Phase: 2-reliable-canonical-indexing*
*Context gathered: 2026-10-09 (Claude-decided under user delegation)*
