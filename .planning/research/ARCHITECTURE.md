# Architecture Research

**Domain:** Ethereum stablecoin supply and issuance monitoring
**Researched:** 2026-10-08
**Confidence:** MEDIUM (official specifications and product documentation cross-checked; live provider behavior and token history remain to be tested)

## Standard Architecture

### System Overview

```text
Ethereum JSON-RPC
  | fixed canonical block range: logs, headers, totalSupply()
  v
Single Go indexer
  |-- range scheduler + checkpoint + block-hash verification
  |-- USDT/USDC-specific event decoders -> normalized supply deltas
  `-- exact-integer reconciliation at the same block height
  | durable writes: raw events, then checkpoint; periodic snapshots
  v
ClickHouse
  |-- raw supply-change events (source of derived metrics)
  |-- processed-range checkpoint and historical supply anchor
  `-- block-aligned supply samples
  |
  v
Grafana ClickHouse datasource -> supply, issuance, large events, lag/health
```

One Go process and three Compose services suffice for this two-token MVP. The indexer owns chain interpretation and write order; ClickHouse stores auditable facts and answers aggregate queries; Grafana owns presentation. No queue, API service, or real-time subscription is needed while a bounded HTTP polling loop meets freshness requirements. The repository currently has no application code, so these are proposed boundaries, not existing modules. [Confidence: MEDIUM]

### Component Responsibilities

| Component | Responsibility | Boundary |
|-----------|----------------|----------|
| Configuration | Ethereum chain ID, RPC URL, token addresses/decimals, start or anchor block, finality policy, thresholds | Validate before indexing; keep secrets in environment |
| RPC reader | Chain ID/head, bounded `eth_getLogs`, block headers, historical `eth_call` | Never writes storage; retry rate limits and shrink rejected ranges |
| Token decoder | Turn contract-specific logs into one signed supply change per economic action | USDT: `Issue`, `Redeem`, `DestroyedBlackFunds`; USDC: zero-address `Transfer` (do not also count `Mint`/`Burn`) |
| Range indexer | Choose canonical range, verify headers, decode, persist events, advance checkpoint | One writer for one chain; a failed range is retried in full |
| ClickHouse store | Raw log provenance, logical deduplication, checkpoint, anchor, snapshots | Read paths must account for eventual `ReplacingMergeTree` merges |
| Metrics/query layer | Daily and rolling sums, supply, reconciliation difference | Start as ClickHouse queries/views over deduplicated raw events |
| Grafana | Provisioned ClickHouse datasource and dashboards | Read-only user; no chain interpretation in panels |

The token-specific boundary is required by deployed contract behavior, not a speculative multi-chain abstraction. ERC-20 only *recommends* zero-address `Transfer` for mint. Tether's USDT implementation changes `_totalSupply` in `issue`, `redeem`, and `destroyBlackFunds`, emitting `Issue`, `Redeem`, and `DestroyedBlackFunds` respectively, and seeds supply in its constructor. Circle's USDC emits both `Mint`/`Burn` and matching zero-address `Transfer` for an action; selecting both double-counts. Contract source and Circle design support these rules, but verify them against the deployed addresses and a few known transactions in the first implementation phase. [Confidence: MEDIUM] [Sources: ERC-20, Tether contract, Circle token design]

### Recommended Project Structure

```text
cmd/indexer/main.go             # config, dependencies, process lifecycle
internal/ethereum/              # RPC range, header, and state reads
internal/indexer/               # loop, checkpoint/restart, finality policy
internal/token/                 # USDT and USDC decoding into supply deltas
internal/storage/               # ClickHouse schema access and exact queries
migrations/                     # raw events, checkpoint, anchor, snapshots
deployments/                    # Compose and Grafana provisioning
```

Keep packages coarse until a real second implementation appears. `internal/token` is where token differences belong; neither the Ethereum reader nor SQL queries should infer mint/burn from generic transfer direction alone. [Confidence: MEDIUM]

## Architectural Patterns

### 1. Commit a Range Before Advancing Its Checkpoint

**What:** For each range `[from,to]`, read a fixed eligible head, fetch relevant logs, confirm their block hashes match canonical headers, insert all normalized events, then persist `(chain, indexer, to, to_block_hash)`. A zero-event range also advances after successful RPC/header checks. On restart, verify the stored hash against the canonical block at `to`, then start at `to+1`. If the event write is uncertain, retry the same range; if checkpoint write fails, replay it. [Confidence: MEDIUM]

**Why:** Cross-table writes are separate operations. Write ordering makes a crash cause replay, not an unfillable gap. The checkpoint is a statement that *all* events through its block were durably acknowledged. A logical deduplication read handles replayed rows; ClickHouse insert-token deduplication is an extra retry optimization with a finite window, not the correctness proof. [Sources: ClickHouse insert retry and ReplacingMergeTree docs]

**Trade-off:** A single writer and replay simplify correctness but limit throughput. Two token contracts and bounded log filters do not justify a distributed coordinator yet.

### 2. Prefer Finalized Blocks, Detect Any Canonicality Change

**What:** Prefer the RPC `finalized` head when the configured provider supports it. If unavailable, process no newer than `latest - confirmation_blocks` (the spec proposes 20), and keep that choice configurable. Persist the checkpoint block hash and compare it at startup and before continuation. Compare returned log block hashes to fetched headers for the range; a mismatch means the range must not commit. [Confidence: MEDIUM]

**Recovery:** On a checkpoint mismatch, stop and expose an unhealthy state. A repair command or documented operator procedure must rewind to a common canonical block, exclude or remove orphaned rows and snapshots, then replay forward. Merely moving the checkpoint backward leaves orphaned events visible. Using the finalized head makes normal short reorgs rare, but it does not eliminate provider error or a consensus incident. [Sources: Ethereum JSON-RPC, Ethereum finality docs]

**Trade-off:** Finalized reads lag the tip more than a 20-block heuristic, but give a clearer chain-consistency contract. Confirm actual RPC-provider support and retention during the first end-to-end proof.

### 3. Separate Log Identity From Economic Event Identity

**What:** Preserve `chain_id`, contract, block number/hash/time, transaction hash, log index, topic/event kind, and raw integer amount. A physical log is identified by `(chain_id, contract, block_hash, tx_hash, log_index)`; its normalized supply delta carries `MINT` or `BURN` plus a source/reason. For USDC, use one chosen log family per action. For USDT, decode all three supply-changing event kinds. Avoid treating the original spec's `chain + block_number + tx_hash + log_index` as a reorg-proof identity because the same transaction can appear in another block after a reorg. [Confidence: MEDIUM]

**ClickHouse consequence:** `ReplacingMergeTree` does not enforce uniqueness on insert. Its `ORDER BY` key defines which rows eventually replace each other; raw `SUM` can double-count duplicate inserts until a merge. Every correctness-sensitive aggregate must read through `FINAL` or another proven logical-deduplication query. Do not build an insert-triggered additive daily materialized view until replay/reorg semantics have been demonstrated; a later merge of source duplicates does not retract previously added aggregates. [Sources: ClickHouse ReplacingMergeTree and materialized-view docs]

**Amounts:** Keep native token integer units for ingestion and reconciliation. Convert to six-decimal human units only for presentation; never use binary floating point for a `totalSupply` equality check. [Confidence: MEDIUM]

### 4. Make Absolute Supply an Anchored Calculation

**What:** For each token, choose a recorded historical block `B` and read `totalSupply()` at exactly `B`; store `(chain_id, contract, B, block_hash, raw_supply)` as an immutable anchor. Index all supply-changing logs from `B+1` onward. At processed block `H`, calculate `anchor_supply + SUM(signed_delta for B < block <= H)` and compare with `totalSupply()` sampled at `H` (same block, ideally same canonical hash). Store snapshot block number/hash, both raw amounts, difference, and sync status. [Confidence: MEDIUM]

**Alternative:** Full deployment-to-tip backfill is valid if the RPC provider serves all logs and the USDT constructor's initial supply is included. Starting from an arbitrary recent block with `SUM(mint-burn)` alone is not an absolute supply. Historical `eth_call` may require archive-capable state access; verify provider limits before selecting `B`. A mismatch is evidence to investigate decoder coverage, missing ranges, historical contract behavior, or provider consistency. It must not silently re-anchor calculated supply. [Sources: Ethereum archive-node and JSON-RPC docs; Tether contract]

## Data Flow and Failure Semantics

```text
startup -> validate chain ID/config -> load checkpoint + anchors
        -> verify checkpoint hash -> determine eligible head
        -> fetch bounded logs for contract + selected topics
        -> decode token-specific supply changes
        -> verify block hashes/ordering -> insert raw events
        -> persist checkpoint (height + hash) -> sample totalSupply at height
        -> calculate from anchor + deduplicated events -> save snapshot
        -> repeat
```

| Failure | Required behavior |
|---------|-------------------|
| RPC timeout/rate limit | Retry with backoff; reduce range on provider result-size/range errors; no checkpoint movement |
| Events inserted, checkpoint absent | Replay same range; exact queries deduplicate logical logs |
| Checkpoint written, snapshot absent | Rebuild/sample snapshot separately; checkpoint means logs complete, not dashboard metrics complete |
| Reorg or inconsistent provider hashes | Stop normal advancement, mark unhealthy, rewind/repair orphaned state before resuming |
| Historical state unavailable | Select a later explicit anchor or obtain archive-capable RPC; do not claim pre-anchor supply history |

### Query and State Boundaries

For MVP, query the deduplicated raw event stream for daily and 24-hour/7-day/30-day sums, and query snapshots for supply trends. Daily bucketing uses UTC block timestamps, not ingestion time. Rolling windows use precise timestamps rather than summing whole calendar days. Checkpoints and snapshots can be `ReplacingMergeTree` rows queried with `FINAL`; all row identities and versions must be explicit. Use a read-only Grafana ClickHouse account. [Confidence: MEDIUM]

## Build Order for Roadmap

1. **Contract/RPC proof:** Validate deployed chain ID, USDT `Issue`/`Redeem`/`DestroyedBlackFunds` and USDC zero-address `Transfer` against known transactions; test bounded log queries and historical `totalSupply()` support. This is a correctness gate before schema or dashboard work.
2. **Raw ingestion:** Implement token decoders, exact units, canonical header checks, ClickHouse raw schema, checkpoint ordering, restart/replay test, and finality configuration.
3. **Absolute supply:** Fix and record anchors (or complete full backfill), sample `totalSupply()` at processed heights, reconcile, and exercise gap/mismatch cases.
4. **Metrics and dashboard:** Add correct deduplicated SQL for daily/rolling issuance and large events, then snapshots and Grafana panels. Add pre-aggregation only after query performance requires it.
5. **Operational finish:** Compose provisioning, lag/error metrics, repair procedure, and a replay/reorg fault-injection check.

The dependency is strict: panels cannot validate supply until token event semantics, replay behavior, and a historical anchor are established. [Confidence: MEDIUM]

## Scaling Considerations

| Pressure | MVP response | Later trigger and response |
|----------|--------------|----------------------------|
| RPC range/provider limit | Adaptive bounded ranges and backoff | Add provider capacity only when measured throughput cannot catch up |
| ClickHouse small inserts | Batch one processed range or larger chunks | Tune batching/async inserts after part pressure appears |
| `FINAL` query cost | Two-token event volume; filter token/time | Rebuildable deduplicated daily table only after query latency is measured |
| More chains/tokens | Keep decoder per token and chain ID in keys | Add worker partitioning and per-chain checkpoints when needed |

## Anti-Patterns

| Anti-pattern | Consequence | Instead |
|--------------|-------------|---------|
| Zero-address `Transfer` for every token | Misses USDT issuance/redemption and black-fund destruction | Token-specific supply-change decoding |
| Count USDC `Mint`/`Burn` and matching `Transfer` | Doubles issuance | One canonical log representation per action |
| `ReplacingMergeTree` as immediate unique constraint | Inflated sums during replay | `FINAL`/logical deduplication on exact reads; test before pre-aggregation |
| Write checkpoint before events | Permanent silent gaps on crash | Events acknowledged first, checkpoint last |
| Compare current `totalSupply` to an older event height | False reconciliation alerts | `eth_call` at the exact processed block |
| Start near tip with zero initial supply | Persistent absolute-supply error | Historical anchor at block `B` or complete deployment backfill |
| Rewind checkpoint only after reorg | Orphaned rows remain in aggregates | Remove/exclude orphaned block hashes and rebuild affected snapshots |

## Sources

- [Ethereum JSON-RPC API](https://ethereum.org/developers/docs/apis/json-rpc/) - `eth_getLogs`, block headers, `safe`/`finalized` tags, and block-specific `eth_call` (current docs checked 2026-10-08).
- [Ethereum proof-of-stake finality FAQ](https://ethereum.org/developers/docs/consensus-mechanisms/pos/faqs/) - finalized-chain semantics (checked 2026-10-08).
- [Ethereum archive nodes](https://ethereum.org/developers/docs/nodes-and-clients/archive-nodes) - historical state availability (checked 2026-10-08).
- [ERC-20 specification](https://eips.ethereum.org/EIPS/eip-20) - `Transfer` mint convention is SHOULD, `totalSupply` interface (checked 2026-10-08).
- [Tether USDT contract source](https://github.com/tethercoin/USDT/blob/main/TetherToken.sol) - constructor supply, `Issue`, `Redeem`, `DestroyedBlackFunds` (checked 2026-10-08; confirm deployed bytecode/source in implementation phase).
- [Circle FiatToken design](https://github.com/circlefin/stablecoin-evm/blob/master/doc/tokendesign.md) - paired USDC mint/burn and zero-address `Transfer` events, proxy upgrade model (checked 2026-10-08).
- [ClickHouse ReplacingMergeTree reference](https://clickhouse.com/docs/en/engines/table-engines/mergetree-family/replacingmergetree) - asynchronous replacement and `FINAL` (checked 2026-10-08).
- [ClickHouse insert retry deduplication](https://clickhouse.com/docs/en/guides/developer/deduplicating-inserts-on-retries) - insert tokens and finite deduplication windows (checked 2026-10-08).
- [ClickHouse materialized views](https://clickhouse.com/blog/using-materialized-views-in-clickhouse) - insert-triggered view behavior (checked 2026-10-08).
- [Grafana ClickHouse datasource](https://grafana.com/docs/plugins/grafana-clickhouse-datasource/latest/configure/) - direct datasource configuration (checked 2026-10-08).

## Open Questions for Implementation

- Does the selected provider expose `finalized`, old logs, and an archive-state `eth_call` at the chosen anchor block within its rate limits?
- What are the deployed USDT/USDC contract creation blocks and known issue/mint/burn transactions for decoder fixtures? Confirm the source code against those addresses and any historical USDC proxy upgrades.
- What operator procedure should handle the exceptional finalized-hash mismatch? A tested rewind/rebuild is needed before claiming unattended reorg recovery.

---
*Architecture research for: Ethereum Stablecoin Monitor*
*Researched: 2026-10-08*
