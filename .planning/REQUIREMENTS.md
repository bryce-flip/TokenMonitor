# Requirements: Ethereum Stablecoin Monitor

**Defined:** 2026-10-08
**Core Value:** Give a trustworthy, reproducible view of USDT and USDC supply and mint/burn activity on Ethereum Mainnet.

The user-provided [source specification](../requirements.md) defines the intended MVP. Project research corrects one implementation premise: Ethereum USDT supply changes are not fully represented by zero-address `Transfer` events. The requirements below preserve the intended product behavior and require contract-specific evidence.

## v1 Requirements

### Chain and Tokens

- [x] **CHAIN-01**: Operator can configure an external Ethereum Mainnet HTTP RPC endpoint and the USDT/USDC contract addresses, decimals, and confirmation policy without changing business logic.
- [ ] **CHAIN-02**: Indexer verifies chain ID 1 and confirms the configured RPC supports the selected historical block range and block-specific contract calls before reporting supply as available.

### Supply Events

- [x] **EVENT-01**: Indexer recognizes USDT `Issue` as mint and `Redeem` and `DestroyedBlackFunds` as distinct supply-decrease reasons, verified against deployed-contract transactions.
- [x] **EVENT-02**: Indexer recognizes USDC mint and burn from zero-address `Transfer` logs, verified against deployed-contract transactions, without also counting paired `Mint`/`Burn` logs.
- [x] **EVENT-03**: Operator can inspect each stored supply event's token, source event, direction/reason, exact raw amount, block number/hash/time, transaction hash, and log index; ordinary transfers do not affect issuance.

### Reliable Indexing

- [x] **SYNC-01**: Indexer fetches bounded block ranges through HTTP RPC and continuously advances only through the configured confirmed or finalized head.
- [x] **SYNC-02**: Indexer retries transient RPC failures and rate limits without skipping a range or advancing its checkpoint.
- [x] **SYNC-03**: Indexer resumes after restart from a durable block-number-and-hash checkpoint that advances only after that range's events are durably accepted.
- [x] **SYNC-04**: Replaying a processed range does not change logical event counts or supply/issuance sums, even before ClickHouse background merges finish.
- [x] **SYNC-05**: Indexer detects a checkpoint block-hash mismatch and supports a verified rewind that excludes orphaned events and rebuilds affected derived data before normal reporting resumes.

### Supply and Reconciliation

- [ ] **SUPP-01**: Operator can establish and inspect a verified per-token `totalSupply()` anchor at a named canonical block, then calculate supply from complete subsequent supply-changing events.
- [ ] **SUPP-02**: Indexer reads USDT and USDC `totalSupply()` at the same canonical block used for each calculated supply snapshot.
- [ ] **SUPP-03**: Operator can see calculated supply, contract supply, and exact difference for each token at a named block, with unavailable or stale data clearly distinguished from an actual mismatch.
- [ ] **SUPP-04**: Operator receives a clear warning when supply differs or known contract behavior changes invalidate the active decoder; the system does not silently re-anchor a mismatch.

### Issuance and Presentation

- [ ] **METR-01**: Operator can see UTC daily mint, burn, and net issuance per token from logically deduplicated canonical events; USDT `DestroyedBlackFunds` contributes to burn while retaining its distinct reason.
- [ ] **METR-02**: Operator can see trailing 24-hour, 7-day, and 30-day net issuance per token, distinct from UTC calendar-day totals.
- [ ] **METR-03**: Operator can inspect mint and burn events above a configurable nominal token-unit threshold, including block, time, and transaction link; `DestroyedBlackFunds` remains identifiable.
- [ ] **DASH-01**: Grafana shows current USDT, USDC, and combined token-unit supply with the observed block/time and freshness state.
- [ ] **DASH-02**: Grafana shows per-token and combined supply trends over 24-hour, 7-day, 30-day, 90-day, and 1-year ranges where indexed history is available.
- [ ] **DASH-03**: Grafana shows daily mint, burn, and net issuance plus recent large events with transaction links.

### Operations

- [ ] **OPER-01**: Operator can start the Go indexer, ClickHouse, and Grafana with Docker Compose using an externally supplied RPC URL and a read-only Grafana data source.
- [ ] **OPER-02**: Operator can diagnose current/eligible block, sync lag or stalled sync, processed blocks/events, RPC/indexer errors, and supply mismatch through clear health output and logs.

## v2 Requirements

### Expansion

- **EXP-01**: Operator can monitor additional chains and stablecoins after the Ethereum USDT/USDC path is verified.
- **EXP-02**: Operator can receive large-event and mismatch notifications through Telegram, Discord, email, or webhook.
- **EXP-03**: A custom API or web dashboard can serve the stored metrics when Grafana no longer meets product needs.

## Out of Scope

| Feature | Reason |
|---------|--------|
| Ethereum node hosting | An external RPC provider is sufficient for this MVP. |
| Full ordinary ERC-20 transfer indexing, address labels, exchange/whale tracking, DeFi and cross-chain analysis | They do not answer the supply and issuance questions. |
| USD market valuation or reserve backing claims | Token units and contract supply alone cannot prove a market price or issuer reserves. |
| Out-of-band alerts in v1 | Clear logs and dashboard health states cover the initial operating need; delivery integrations are deferred. |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| CHAIN-01 | Phase 1 | Complete |
| CHAIN-02 | Phase 3 | Pending |
| EVENT-01 | Phase 1 | Complete |
| EVENT-02 | Phase 1 | Complete |
| EVENT-03 | Phase 1 | Complete |
| SYNC-01 | Phase 2 | Complete |
| SYNC-02 | Phase 2 | Complete |
| SYNC-03 | Phase 2 | Complete |
| SYNC-04 | Phase 2 | Complete |
| SYNC-05 | Phase 2 | Complete |
| SUPP-01 | Phase 3 | Pending |
| SUPP-02 | Phase 3 | Pending |
| SUPP-03 | Phase 3 | Pending |
| SUPP-04 | Phase 3 | Pending |
| METR-01 | Phase 4 | Pending |
| METR-02 | Phase 4 | Pending |
| METR-03 | Phase 4 | Pending |
| DASH-01 | Phase 4 | Pending |
| DASH-02 | Phase 4 | Pending |
| DASH-03 | Phase 4 | Pending |
| OPER-01 | Phase 4 | Pending |
| OPER-02 | Phase 4 | Pending |

---
*Requirements defined: 2026-10-08*
*Last updated: 2026-10-08 after initial definition*
