# Project Research Summary

**Project:** Ethereum Stablecoin Monitor
**Domain:** Ethereum Mainnet USDT/USDC issuance monitoring
**Researched:** 2026-10-08
**Confidence:** MEDIUM

## Executive Summary

This is an operator monitor for Ethereum contract supply and issuance, not a reserve attestation or global circulation estimate. The lean architecture is one Go indexer polling confirmed RPC ranges, ClickHouse holding auditable raw events and block-aligned snapshots, and Grafana querying that data directly. Docker Compose provides the three services; an external RPC endpoint remains configurable.

The source specification's Transfer-only assumption must be corrected before requirements or implementation are finalized. USDC issuance can be classified from zero-address `Transfer` logs without also counting its paired `Mint`/`Burn` events. USDT instead changes supply through `Issue`, `Redeem`, and `DestroyedBlackFunds`; its constructor seeds supply without a mint log, and deprecation can delegate `totalSupply()` to an upgraded contract. These are contract-specific rules within the existing two-token scope, not a reason to expand it.

Correctness depends on a verified historical supply anchor, exact integer amounts, canonical block hashes, and same-block `totalSupply()` comparisons. ClickHouse replacement is eventual, so replay and reorg recovery require logical deduplication and orphan exclusion before any daily sums or dashboard values can be trusted. Live contract fixtures, provider history support, and compatibility smoke tests remain open.

## Key Findings

### Recommended Stack

Use [STACK.md](STACK.md)'s existing Go 1.27.1 module target, go-ethereum v1.17.7 for HTTP JSON-RPC, ClickHouse 26.8 LTS with clickhouse-go/v2 v2.48.0 for exact raw storage, Grafana OSS 13.2.2 with ClickHouse datasource 4.22.1 for operator views, and Docker Compose V2 for local operation. Pin an actual ClickHouse 26.8 patch image only after checking the image tag and smoke-testing driver/plugin connections. Go standard-library configuration, logging, and exact arithmetic avoid extra dependencies. No queue, WebSocket, custom API, or precomputed rollup is justified for the MVP.

### Expected Features

From [FEATURES.md](FEATURES.md), the must-haves are two-token Mainnet supply-event ingestion; restart/replay/reorg-safe raw history; anchored calculated supply with same-block contract comparison; UTC daily mint/burn/net and trailing 24h/7d/30d net; supply and issuance trends; threshold-filtered large events; lag/error/mismatch visibility; and Compose startup. The valuable distinguishing behavior is traceability from a dashboard figure to its transaction and an independently reconciled contract supply. Preserve `DestroyedBlackFunds` as a distinct supply-decrease reason even if its amount contributes to an aggregate burn metric.

Defer extra chains/tokens, ordinary transfers and wallet labels, notifications, reserve or price claims, a custom frontend/API, and query rollups until actual need or a new scope decision. Label amounts as token units rather than asserting exact USD value.

### Architecture Approach

[ARCHITECTURE.md](ARCHITECTURE.md) recommends one writer with bounded RPC ranges, token-specific decoders, and a ClickHouse store for raw provenance, checkpoints, anchors, and snapshots. The indexer validates chain ID and block hashes, inserts all events before advancing a `(height, hash)` checkpoint, and retries a failed range. Correctness-sensitive SQL deduplicates canonical logs; daily aggregates use UTC block timestamps and rolling windows use precise timestamps. At block H, calculate `anchor_supply + signed deltas through H` and compare to `totalSupply(H)`. Grafana uses a provisioned read-only ClickHouse account.

### Critical Pitfalls

1. **Wrong event semantics:** Transfer-only misses USDT changes, while counting USDC `Mint`/`Burn` and Transfer doubles them. Verify historical fixtures for every event path, including a USDT zero-address transfer that does not reduce supply.
2. **False absolute supply:** USDT's constructor amount has no event, and a recent backfill has no opening balance. Record a verified block-specific anchor or reconstruct from deployment with constructor supply; compare contract state at the same block.
3. **Replay duplicates or gaps:** `ReplacingMergeTree` is not immediate uniqueness or a cross-table transaction. Insert events before checkpoint; use canonical deduplicated reads and test a crash between writes.
4. **Orphaned events after reorg:** Confirmation depth alone cannot remove stale rows. Verify checkpoint hashes, stop on mismatch, retire orphaned blocks, and rebuild affected snapshots before resuming.
5. **Silent contract-semantic change:** USDT deprecation delegates supply, while USDC's proxy can upgrade. Detect those transitions and flag the decoder as requiring review rather than reporting a misleading ordinary mismatch.

## Implications for Roadmap

### Phase 1: Contract and RPC Proof
**Rationale:** The Transfer-only premise in `requirements.md` is materially wrong for USDT; historical RPC capabilities also determine the anchor strategy.
**Delivers:** Verified deployed-address fixtures for USDT `Issue`, `Redeem`, `DestroyedBlackFunds`, zero-address Transfer, USDC mint/burn, and available `finalized`/historical `eth_call` behavior; an explicit supply baseline choice.
**Addresses:** Correct two-token event classification and the prerequisite for reconciliation.
**Avoids:** Missed or doubled issuance, a false USDT burn, and an impossible historical anchor.

### Phase 2: Canonical Raw Ingestion
**Rationale:** Every metric depends on complete, replayable canonical events.
**Delivers:** Bounded Go RPC polling, exact token-unit decoders, ClickHouse raw schema, event-before-checkpoint writes, restart/replay checks, block-hash validation, and a tested rewind/orphan-exclusion procedure.
**Addresses:** Mainnet ingestion, raw-event traceability, restart recovery, duplicate protection, and reorg-safe selection.
**Avoids:** ClickHouse duplicate sums, skipped ranges, orphaned rows, and imprecise amounts.

### Phase 3: Anchored Supply and Reconciliation
**Rationale:** Absolute supply is meaningful only after the chosen start block and complete delta stream exist.
**Delivers:** Durable per-token anchors, block-aligned `totalSupply()` snapshots, exact calculated supply and difference, and clear unavailable/stale/mismatch states. Detect USDT deprecation and USDC implementation changes as contract-semantic warnings.
**Addresses:** Current and combined on-chain supply, supply trend foundation, independent reconciliation.
**Avoids:** Constructor-offset errors, mismatched-head false alerts, and silent upgraded-contract assumptions.

### Phase 4: Issuance Views and Operations
**Rationale:** Once canonical events and snapshots are trustworthy, SQL and Grafana can expose them without duplicating chain logic.
**Delivers:** UTC daily and rolling metrics, large-event table with transaction links, Grafana supply/issuance/health panels, read-only datasource, logs/lag/errors, and Compose deployment smoke test.
**Addresses:** Daily/24h/7d/30d issuance, charts, large events, health, and one-command operation.
**Avoids:** Duplicate rollups, UTC boundary mistakes, stale figures presented as current, and untested plugin provisioning.

### Phase Ordering Rationale

The decoder and anchor decision gate all later correctness claims. Durable ingestion must precede supply calculation and dashboard aggregates. Keep `DestroyedBlackFunds` separately identifiable throughout; decide its presentation under burn versus adjustment before accepting daily metrics. Use raw deduplicated queries first, then optimize only after measured latency warrants it.

### Research Flags

Phases needing `$gsd-plan-phase --research-phase <N>`:
- **Phase 1:** Confirm live deployed contract behavior, constructor argument, event fixtures, finalized-tag support, and provider historical state/log limits.
- **Phase 2:** Validate the precise ClickHouse deduplication and orphan-repair design with replay/reorg fault cases.
- **Phase 3:** Check USDT deprecation and USDC proxy-change detection against deployed contracts and the chosen anchor method.

Standard patterns; skip additional research unless a smoke test fails:
- **Phase 4:** Grafana provisioning, UTC SQL queries, read-only datasource, and Compose are documented patterns.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | MEDIUM | Official release/API sources; exact image, driver, and plugin combinations need local smoke tests. |
| Features | MEDIUM | Product scope is explicit locally, but deployed token behavior and operator presentation choices need validation. |
| Architecture | MEDIUM | Failure ordering and component boundaries follow official APIs/docs; RPC history and repair behavior need proof. |
| Pitfalls | MEDIUM | Primary contract, Ethereum, and ClickHouse sources support them; historical transactions remain untested locally. |

**Overall confidence:** MEDIUM

### Gaps to Address

- Confirm the USDT constructor amount from deployed chain data, known event transactions, and the current USDC proxy implementation; source/explorer findings alone are insufficient acceptance evidence.
- Verify the actual RPC provider serves chosen historical logs/state and `finalized`; select an explicit later anchor if archive state is unavailable.
- Define whether `DestroyedBlackFunds` is displayed as a burn subtype or separate supply adjustment, while always including its signed delta in supply.
- Define mismatch warning persistence and freshness criteria; exact same-block integer equality is the baseline, but one failed RPC sample should not masquerade as persistent drift.
- Amend `requirements.md` during requirements definition to replace the Transfer-only USDT assumption and state the anchor/deprecation behavior. Keep the existing Ethereum USDT/USDC product boundary.

## Sources

### Primary

- [Project scope](../PROJECT.md) and [source specification](../../requirements.md) — intended MVP boundary, not implemented behavior.
- [Tether USDT contract](https://github.com/tethercoin/USDT/blob/main/TetherToken.sol), [Circle FiatToken design](https://github.com/circlefin/stablecoin-evm/blob/master/doc/tokendesign.md), and [ERC-20](https://eips.ethereum.org/EIPS/eip-20) — token supply semantics and limits of Transfer conventions.
- [Ethereum JSON-RPC](https://ethereum.org/developers/docs/apis/json-rpc/), [ClickHouse ReplacingMergeTree](https://clickhouse.com/docs/en/engines/table-engines/mergetree-family/replacingmergetree), and [Grafana ClickHouse datasource](https://grafana.com/docs/plugins/grafana-clickhouse-datasource/latest/configure/) — block-qualified reads, eventual replacement, and presentation integration.
- [Go releases](https://go.dev/doc/devel/release), [go-ethereum API](https://pkg.go.dev/github.com/ethereum/go-ethereum/ethclient), [ClickHouse Go integration](https://clickhouse.com/integrations/go), and [Compose startup ordering](https://docs.docker.com/compose/how-tos/startup-order/) — proposed implementation stack.

### Secondary

- [USDT explorer record](https://etherscan.io/token/0xdac17f958d2ee523a2206206994597c13d831ec7) — reported constructor supply; verify directly against deployed chain data.
- [STACK.md](STACK.md), [FEATURES.md](FEATURES.md), [ARCHITECTURE.md](ARCHITECTURE.md), and [PITFALLS.md](PITFALLS.md) — full evidence, caveats, and source lists.

---
*Research completed: 2026-10-08*
*Ready for roadmap: yes*
