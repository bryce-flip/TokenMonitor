# Phase 1: Live Supply Event Path - Context

**Gathered:** 2026-10-08
**Status:** Ready for planning

<domain>
## Phase Boundary

A Go indexer that pulls one bounded range of real Ethereum Mainnet logs over HTTP RPC, decodes USDT (`Issue` / `Redeem` / `DestroyedBlackFunds`) and USDC (zero-address `Transfer`) supply-changing events with token-specific classification, and stores them with full provenance in ClickHouse for direct SQL inspection.

Not in this phase: continuous sync, durable checkpoints, replay/reorg safety (Phase 2); supply arithmetic and `totalSupply()` anchoring (Phase 3); Grafana, Compose stack, health metrics (Phase 4).

</domain>

<decisions>
## Implementation Decisions

### RPC Provider
- **D-01:** Phase 1 runs against a free-tier provider key (Alchemy, Infura, or QuickNode — user's choice at run time). Free tiers serve full-history `eth_getLogs` on Ethereum Mainnet, which is sufficient for the bounded proof range. The credential is supplied externally via env var; never in source.
- **D-02:** The indexer performs a fail-fast provider probe before ingesting: verify chain ID 1 and that the provider serves the requested block range (`eth_blockNumber` + a sample `eth_getLogs`/`eth_getBlockByNumber`), then abort with a clear error on failure. This addresses the STATE.md Phase 1 blocker (provider historical-log verification) and catches wrong-network URLs and history-limited providers immediately.

### Claude's Discretion
- **Proof range & evidence** — the user deferred this discussion by moving to planning. Researcher/planner should choose a bounded recent Mainnet range that exercises all classification paths (USDT `Issue`, `Redeem`, `DestroyedBlackFunds`, zero-address `Transfer` on both tokens, ordinary transfers to exclude), plus pinned historical fixture transactions for rare paths (notably USDT `DestroyedBlackFunds`). Verification evidence should be automated Go tests against pinned real transactions per research recommendations.
- **Config shape** — source spec proposes `config/tokens.yaml` with contract addresses, decimals, and confirmation policy (CHAIN-01); Go stdlib config per research. Follow the spec's YAML proposal unless research finds a simpler stdlib-only path.
- **ClickHouse runtime in Phase 1** — single local ClickHouse instance (docker run or minimal compose) is enough; the full three-service Compose stack is Phase 4 (OPER-01). Inspection is raw SQL via clickhouse-client.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Product specification
- `requirements.md` — source specification: token contracts, event rules, proposed ClickHouse schemas, config format, acceptance criteria. Note: its Transfer-only premise for USDT is corrected by research below.
- `.planning/REQUIREMENTS.md` — v1 requirement set for this phase: CHAIN-01, EVENT-01, EVENT-02, EVENT-03.

### Research findings
- `.planning/research/PITFALLS.md` — event-semantics pitfalls: USDT `Issue`/`Redeem`/`DestroyedBlackFunds` without zero-address `Transfer`; USDC double-count risk; fixture-verification requirement. Critical for the decoder design.
- `.planning/research/STACK.md` — pinned library choices: go-ethereum HTTP client, clickhouse-go/v2, ClickHouse 26.x LTS, Go stdlib config/logging.
- `.planning/research/ARCHITECTURE.md` — recommended writer/decoder/store shape that Phase 1 starts.
- `.planning/research/SUMMARY.md` — cross-cutting summary of all of the above.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- None — repository contains only `go.mod` (module `TOkenMonitor`, Go 1.27.1) and `requirements.md`. Greenfield implementation.

### Established Patterns
- None established. Follow standard Go layout; `gofmt` formatting. The package layout in `requirements.md` is a proposal, not existing convention.

### Integration Points
- New code connects to: external Ethereum JSON-RPC provider (HTTP), ClickHouse instance, and env/file configuration. No existing code to integrate with.

</code_context>

<specifics>
## Specific Ideas

- Ordinary USDT zero-address `Transfer` events must be recorded as non-supply-changing (or excluded) — never misclassified as issuance (see success criterion 2).
- USDC paired `Mint`/`Burn` events are corroborating evidence only — counting them alongside zero-address `Transfer` doubles issuance.
- Exact raw integer amounts (`uint256` / `Int128`-style) end to end; no floating point.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 1-live-supply-event-path*
*Context gathered: 2026-10-08*
