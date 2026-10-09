---
gsd_state_version: "1.0"
milestone: v1.0
current_phase: 2
current_phase_name: Reliable Canonical Indexing
status: executing
stopped_at: Completed 02-02-PLAN.md
last_updated: "2026-10-09T11:03:00.427Z"
last_activity: 2026-10-09
last_activity_desc: Phase 2 execution started
state_head: 698b332ea64936cf2018ad5e7d1afb3125c8e23a
progress:
  total_phases: 4
  completed_phases: 1
  total_plans: 5
  completed_plans: 4
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-10-08)

**Core value:** Give a trustworthy, reproducible view of USDT and USDC supply and mint/burn activity on Ethereum Mainnet.
**Current focus:** Phase 2 — Reliable Canonical Indexing

## Current Position

Phase: 2 (Reliable Canonical Indexing) — EXECUTING
Plan: 3 of 3
Status: Ready to execute
Last activity: 2026-10-09 — Phase 2 execution started

Progress: [███░░░░░░░] 25%

## Performance Metrics

**Velocity:**
- Total plans completed: 2
- Average duration: -
- Total execution time: 0 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 1 | 2 | - | - |

**Recent Trend:**
- Last 5 plans: -
- Trend: -

**Per-Plan Metrics:**

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| Phase 01 P01 | 31min | 3 tasks | 12 files |
| Phase 01 P02 | 26min | 2 tasks | 4 files |
| Phase 02 P01 | 38min | 2 tasks | 11 files |
| Phase 02 P02 | 33min | 3 tasks | 4 files |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md. Phase 1 proves real RPC logs through token-specific classification into ClickHouse; later phases establish replay/reorg safety and same-block supply reconciliation.
- [Phase 01]: 01-01: stdlib JSON config + env lookups (no YAML dependency) per research recommendation
- [Phase 01]: 01-01: stablecoin_events stores raw_amount UInt256 only; Decimal(38,6) scaled column deferred to Phase 4
- [Phase 01]: 01-01: ReplacingMergeTree(created_at) on full event identity + FINAL reads make range re-ingest idempotent; ClickHouse pinned to concrete 26.8.20.9 with loopback-only binding
- [Phase 01]: 01-02: free-tier rate-limit tolerance implemented as bounded retry in internal/rpc (probe, fetch, header) after live 429/-32005 bursts deterministically blocked ingest
- [Phase 01]: 01-02: live USDC row-count assertion is per fixture receipt plus a block-wide 1:1 zero-address-Transfer invariant (block 0x1638e19 carries unrelated extra mints)
- [Phase 01]: 01-02: smoke window sized to provider result cap — Infura rejects eth_getLogs over 10k results, so ~40 blocks for USDC; documented in README, chunked fetching deferred to continuous sync
- [Phase 02]: 02-01: EligibleHead resolves finalized via ethclient HeaderByNumber tag constant (go-ethereum serializes the tag; no hand-rolled JSON-RPC); confirmed = latest minus confirmation_blocks clamped at 0
- [Phase 02]: 02-01: RunSync depends on ChainReader/EventStore structural seams (rpc imports indexer for SupplyTopics; reverse import would cycle); proof suite pins the seams in external package indexer_test
- [Phase 02]: 02-01: checkpoint reads are ORDER BY height DESC LIMIT 1, never FINAL; ReplacingMergeTree(height) keeps the merged survivor at max height under out-of-order writes
- [Phase 02]: 02-01: EnsureSchema applies migrations statement-by-statement (native protocol rejects multi-statement; comment lines carry semicolons and apostrophes)

### Pending Todos

None yet.

### Blockers/Concerns

- Phase 1: Verify deployed-contract event fixtures and chosen provider's historical logs before accepting decoder behavior.
- Phase 3: Verify historical block-specific `totalSupply()` support and choose a canonical supply anchor before claiming absolute supply.

## Deferred Items

| Category | Item | Status | Deferred At | Milestone |
|----------|------|--------|-------------|-----------|
| *(none)* | | | | |

## Session Continuity

Last session: 2026-10-09T11:03:00.370Z
Stopped at: Completed 02-02-PLAN.md
Resume file: None
