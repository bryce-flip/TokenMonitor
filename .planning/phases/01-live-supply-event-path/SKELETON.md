# Walking Skeleton — Ethereum Stablecoin Monitor

**Phase:** 1
**Generated:** 2026-10-08

## Capability Proven End-to-End

An operator can run one bounded command that loads token metadata from a config file, fail-fast probes an external Ethereum Mainnet HTTP RPC provider, fetches a bounded range of filtered USDT/USDC logs, classifies each supply-changing event per token, stores exact raw amounts with full chain provenance in ClickHouse, and inspects the stored rows with SQL.

This product has no web UI; the skeleton's "face" is the data path config -> provider probe -> bounded RPC log fetch -> token-specific decode -> ClickHouse insert -> SQL inspection.

## Architectural Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Language / module | Go 1.27.1, module `TOkenMonitor` (existing go.mod spelling) | Source spec and existing toolchain directive; no rename in Phase 1 |
| RPC client | go-ethereum v1.17.7 `ethclient` over HTTP; URL from `ETH_RPC_URL` env only (D-01) | Canonical typed client for FilterLogs/HeaderByNumber/ChainID; credentials never in source |
| Provider gate | Fail-fast probe before ingest: chain ID 1, eligible head bound, sample historical eth_getLogs (D-02) | Catches wrong-network URLs and history-limited providers before any data is written |
| Storage | ClickHouse 26.8 LTS via clickhouse-go/v2 v2.48.0 native protocol; `stablecoin_events` ReplacingMergeTree(created_at) ORDER BY (chain, token, block_number, tx_hash, log_index); FINAL on all correctness reads | Exact UInt256 amounts, stable log identity for later replay dedup (Phase 2), eventual replacement acknowledged up front |
| Amounts | `*big.Int` in Go, `UInt256` in ClickHouse, no float or Decimal in the Phase 1 path | Raw units are the exact ledger; scaled presentation returns with Phase 4 |
| Classification | Token-specific decoder: USDT Issue/Redeem/DestroyedBlackFunds topics; USDC zero-address Transfer only (paired Mint/Burn are corroboration, never rows) | Deployed-contract semantics verified against live Mainnet receipts in Phase 1 research |
| Config | Stdlib `encoding/json` (`config/tokens.json`) + `os.Getenv`; no YAML dependency | Research "Don't Hand-Roll": stdlib covers Phase 1 needs |
| Logging | Stdlib `log/slog`; RPC URLs redacted to scheme://host | No logging dependency; credential hygiene |
| Directory layout | `cmd/indexer/` + `internal/{config,rpc,indexer,storage}` + `migrations/` | Simplified from the spec's proposed tree (PATTERNS.md discretion); entry point name kept |
| Deployment (Phase 1) | Local `docker run` ClickHouse bound to 127.0.0.1:9000 | Single-instance proof; the three-service Compose stack is Phase 4 (OPER-01) |

## Stack Touched in Phase 1

- [x] Project scaffold — Go module builds, `go test ./...`, `go vet` (plan 01-01)
- [x] One real bounded command path — `cmd/indexer` flags `-config/-from/-to` (no web routing in this product)
- [x] Database — real batch write AND real read (`InsertEvents`, `Inspect` over `stablecoin_events`)
- [x] Operator surface — SQL inspection of stored rows with full provenance (EVENT-03); no UI by design
- [x] Runnable stack — documented local run: docker-run ClickHouse + `ETH_RPC_URL` + `go run ./cmd/indexer` (README, plan 01-02)

## Out of Scope (Deferred to Later Slices)

- Continuous sync, durable checkpoints, retry policy, replay/orphan repair — Phase 2 (SYNC-01..05)
- Supply anchoring, `totalSupply()` calls, reconciliation states — Phase 3 (SUPP-01..04)
- Grafana, Compose three-service stack, health metrics — Phase 4
- WebSocket RPC, YAML config, extra chains/tokens, ordinary-transfer storage

## Subsequent Slice Plan

Each later phase adds one vertical slice on top of this skeleton without altering its architectural decisions:

- Phase 2: Reliable Canonical Indexing — the bounded path becomes continuous, checkpointed, replay-safe, reorg-aware
- Phase 3: Anchored Supply Reconciliation — `eth_call totalSupply()` joins the path at named canonical blocks
- Phase 4: Issuance and Operator Views — Grafana reads the table through a read-only account; Compose runs the stack
