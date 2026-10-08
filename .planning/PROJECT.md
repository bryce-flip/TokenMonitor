# Ethereum Stablecoin Monitor

## What This Is

A monitor for USDT and USDC issuance on Ethereum Mainnet. A Go indexer reads contract Transfer logs, records mint and burn events in ClickHouse, derives supply and issuance metrics, compares supply with each contract's `totalSupply()`, and presents the results in Grafana. The initial user is an operator or analyst who needs to inspect current supply, recent issuance, large events, and data health.

## Core Value

Give a trustworthy, reproducible view of USDT and USDC supply and mint/burn activity on Ethereum Mainnet.

## Requirements

### Validated

(None yet; the repository contains a specification and Go module declaration, but no application.)

### Active

- [ ] Index USDT and USDC mint/burn events from Ethereum Mainnet Transfer logs with restart recovery, duplicate protection, and reorg-safe block selection.
- [ ] Store raw events and derive current supply, daily mint/burn/net issuance, and 24-hour, 7-day, and 30-day net issuance.
- [ ] Sample contract `totalSupply()` and report differences from calculated supply.
- [ ] Show supply, trends, issuance, and large events in Grafana.
- [ ] Run the indexer, ClickHouse, and Grafana with Docker Compose; expose clear logs and indexer health metrics.

### Out of Scope

- Other chains and tokens beyond Ethereum Mainnet USDT/USDC: deferred until the first data path is reliable.
- Ethereum node hosting, ordinary ERC-20 transfer indexing, address labels, exchange identification, and DeFi or cross-chain analysis: outside the issuance-monitoring MVP.
- Telegram, Discord, email, and webhook notifications: large events may be recorded and shown without delivery integrations.
- A custom web frontend or API: Grafana reads ClickHouse directly for the MVP.

## Context

- [requirements.md](../requirements.md) is the source specification and includes token contracts, acceptance criteria, proposed schemas, dashboard views, and explicit exclusions.
- The codebase currently has `go.mod` and no Go source, migrations, deployment files, or dashboard. `.planning/codebase/` documents that initial state; the package layout in `requirements.md` is proposed, not established code.
- The first technical proof is Ethereum RPC to USDT/USDC Transfer logs to correct mint/burn classification to ClickHouse. Later capabilities build on stored raw events.
- Derived metrics should be rebuildable from stored events without rescanning Ethereum. Planning must resolve the historical starting point or supply anchor needed for an absolute calculated supply before claiming reconciliation with `totalSupply()`.

## Constraints

- **Chain and tokens:** Ethereum Mainnet USDT and USDC only in the MVP, as specified in `requirements.md`.
- **Technology:** Go indexer, ClickHouse storage, Grafana presentation, and Docker Compose deployment, as specified in `requirements.md`.
- **RPC:** Use an externally configured Ethereum JSON-RPC URL; never embed provider credentials or URLs in source.
- **Data correctness:** Event ingestion must tolerate retries, restarts, duplicate logs, and short chain reorganizations without corrupting reported metrics.
- **Configuration:** Keep token contract metadata, confirmation depth, and large-event thresholds configurable.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Use `requirements.md` as the initial product definition | User confirmed it is the project to implement | Pending |
| Start with Ethereum Mainnet USDT and USDC | Prove the complete issuance data path before expansion | Pending |
| Use raw on-chain events as the source for derived metrics | Supports recalculation after logic changes without another chain scan | Pending |
| Use Grafana directly over ClickHouse for the MVP | The specification does not require a custom API or frontend | Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition:**
1. Move invalidated requirements to Out of Scope with a reason.
2. Move shipped and verified requirements to Validated with a phase reference.
3. Add newly agreed requirements to Active.
4. Record significant decisions and revisit this description if the product changes.

**After each milestone:**
1. Review all sections and the Core Value.
2. Recheck exclusions and update context from actual use and verification.

---
*Last updated: 2026-10-08 after initialization*
