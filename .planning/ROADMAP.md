# Roadmap: Ethereum Stablecoin Monitor

## Overview

The v1.0 MVP first proves a real Ethereum Mainnet USDT/USDC event path into ClickHouse, then makes that history safe to replay and reconcile before exposing issuance and supply in Grafana. Every figure remains traceable to canonical token events and a named block.

## Phases

- [ ] **Phase 1: Live Supply Event Path** - Store correctly classified USDT and USDC supply events from real Ethereum RPC logs in ClickHouse.
- [ ] **Phase 2: Reliable Canonical Indexing** - Continue indexing through eligible blocks with restart, replay, and reorg safety.
- [ ] **Phase 3: Anchored Supply Reconciliation** - Compare event-derived supply with each contract's total supply at the same canonical block.
- [ ] **Phase 4: Issuance and Operator Views** - Present trustworthy supply and issuance in Grafana with Compose startup and clear health signals.

## Phase Details

### Phase 1: Live Supply Event Path

**Goal**: Operators can ingest and inspect correctly classified, real USDT and USDC supply-changing logs in ClickHouse.
**Mode:** mvp
**Depends on**: Nothing (first phase)
**Requirements**: CHAIN-01, EVENT-01, EVENT-02, EVENT-03
**Success Criteria** (what must be TRUE):
  1. Operator can provide an external Ethereum HTTP RPC URL, token metadata, and confirmation policy without changing business logic, then run a bounded Mainnet log range through the Go indexer into ClickHouse.
  2. Stored logs from deployed USDT transactions show `Issue` as mint and `Redeem` and `DestroyedBlackFunds` as distinct supply decreases; zero-address `Transfer` is not misclassified as USDT issuance.
  3. Stored logs from deployed USDC transactions show zero-address `Transfer` mint and burn once each, without double-counting paired `Mint` or `Burn` events.
  4. Operator can inspect each stored event's exact raw amount and chain provenance; ordinary transfers do not appear as supply changes.

**Plans**: 2 plans

Plans:
**Wave 1**
- [ ] 01-01-PLAN.md — Walking skeleton: config, D-02 provider probe, bounded getLogs, token-specific decode, exact ClickHouse insert, inspection; offline fixture and storage exactness tests

**Wave 2** *(blocked on Wave 1 completion)*
- [ ] 01-02-PLAN.md — Live fixture proof through the operator's RPC URL, operator runbook (README), bounded recent-range full-path smoke

### Phase 2: Reliable Canonical Indexing

**Goal**: Operators can trust continuous event history across RPC failures, restarts, duplicate reads, and short chain reorganizations.
**Mode:** mvp
**Depends on**: Phase 1
**Requirements**: SYNC-01, SYNC-02, SYNC-03, SYNC-04, SYNC-05
**Success Criteria** (what must be TRUE):
  1. Indexer advances in bounded ranges only through the configured confirmed or finalized head and continues as new eligible blocks arrive.
  2. A transient RPC failure or rate limit retries the same range; restart resumes from a durable block-number-and-hash checkpoint only after that range's events were accepted.
  3. Replaying a processed range leaves logical event counts and issuance sums unchanged, including before ClickHouse background merges finish.
  4. A checkpoint hash mismatch stops ordinary reporting until a verified rewind excludes orphaned events and rebuilds any affected derived data.

**Plans**: TBD

### Phase 3: Anchored Supply Reconciliation

**Goal**: Operators can distinguish accurate block-aligned supply from unavailable, stale, or mismatched supply.
**Mode:** mvp
**Depends on**: Phase 2
**Requirements**: CHAIN-02, SUPP-01, SUPP-02, SUPP-03, SUPP-04
**Success Criteria** (what must be TRUE):
  1. Indexer verifies Mainnet chain ID and the provider's chosen historical logs and block-specific calls before declaring supply available.
  2. Operator can inspect a verified per-token `totalSupply()` anchor at a named canonical block and calculated supply from complete subsequent supply events.
  3. Operator can compare exact calculated and contract `totalSupply()` values at the same named canonical block, with the difference and unavailable or stale state shown separately from a mismatch.
  4. A real difference or detected USDT/USDC contract behavior change raises a clear warning without silently resetting the anchor.

**Plans**: TBD

### Phase 4: Issuance and Operator Views

**Goal**: Operators can monitor supply, issuance, large events, and indexer health from a runnable three-service stack.
**Mode:** mvp
**Depends on**: Phase 3
**Requirements**: METR-01, METR-02, METR-03, DASH-01, DASH-02, DASH-03, OPER-01, OPER-02
**Success Criteria** (what must be TRUE):
  1. Operator can see per-token UTC daily mint, burn, and net issuance plus distinct trailing 24-hour, 7-day, and 30-day net issuance from logically deduplicated canonical events; USDT `DestroyedBlackFunds` counts toward burn but retains its reason.
  2. Grafana shows threshold-filtered large mint and burn events with token, block, time, transaction link, and distinct `DestroyedBlackFunds` reason alongside daily issuance.
  3. Grafana shows current USDT, USDC, and combined token-unit supply with observed block, time, and freshness state, plus per-token and combined trends over 24-hour, 7-day, 30-day, 90-day, and 1-year ranges where history exists.
  4. Operator can start the Go indexer, ClickHouse, and Grafana with Docker Compose using an external RPC URL; Grafana queries ClickHouse through a read-only account.
  5. Operator can diagnose current and eligible blocks, lag or stalled sync, processed blocks and events, RPC/indexer errors, and supply mismatch from health output and logs.

**Plans**: TBD
**UI hint**: yes

## Progress

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Live Supply Event Path | 0/2 | Planning complete | - |
| 2. Reliable Canonical Indexing | 0/TBD | Not started | - |
| 3. Anchored Supply Reconciliation | 0/TBD | Not started | - |
| 4. Issuance and Operator Views | 0/TBD | Not started | - |
