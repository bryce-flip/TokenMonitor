---
gsd_state_version: "1.0"
milestone: v1.0
current_phase: 2
current_phase_name: Reliable Canonical Indexing
status: Phase 1 shipped — pushed to origin/main (56e3961..5bcf4f4)
stopped_at: Phase 1 complete, ready to plan Phase 2
last_updated: "2026-10-08T12:47:50.290Z"
last_activity: 2026-10-08
state_head: 5bcf4f4ad08c2e83fb9bc500adafa28f2fb93ce8
progress:
  total_phases: 4
  completed_phases: 1
  total_plans: 2
  completed_plans: 2
  percent: 25
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-10-08)

**Core value:** Give a trustworthy, reproducible view of USDT and USDC supply and mint/burn activity on Ethereum Mainnet.
**Current focus:** Phase 1 — Live Supply Event Path

## Current Position

Phase: 2 — Reliable Canonical Indexing
Plan: Not started
Status: Phase 1 shipped — pushed to origin/main (56e3961..5bcf4f4)
Last activity: 2026-10-08

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

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md. Phase 1 proves real RPC logs through token-specific classification into ClickHouse; later phases establish replay/reorg safety and same-block supply reconciliation.
- [Phase 01]: 01-01: stdlib JSON config + env lookups (no YAML dependency) per research recommendation
- [Phase 01]: 01-01: stablecoin_events stores raw_amount UInt256 only; Decimal(38,6) scaled column deferred to Phase 4
- [Phase 01]: 01-01: ReplacingMergeTree(created_at) on full event identity + FINAL reads make range re-ingest idempotent; ClickHouse pinned to concrete 26.8.20.9 with loopback-only binding
- [Phase 01]: 01-02: free-tier rate-limit tolerance implemented as bounded retry in internal/rpc (probe, fetch, header) after live 429/-32005 bursts deterministically blocked ingest
- [Phase 01]: 01-02: live USDC row-count assertion is per fixture receipt plus a block-wide 1:1 zero-address-Transfer invariant (block 0x1638e19 carries unrelated extra mints)
- [Phase 01]: 01-02: smoke window sized to provider result cap — Infura rejects eth_getLogs over 10k results, so ~40 blocks for USDC; documented in README, chunked fetching deferred to continuous sync

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

Last session: 2026-10-08T11:49:31.996Z
Stopped at: Phase 1 complete, ready to plan Phase 2
Resume file: .planning/phases/01-live-supply-event-path/01-UAT.md
