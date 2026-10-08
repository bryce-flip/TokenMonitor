# Feature Research

**Domain:** Ethereum Mainnet USDT/USDC issuance and supply monitor
**Researched:** 2026-10-08
**Confidence:** MEDIUM (research-plan `websearch` findings cross-checked against primary contract and protocol sources)

## Feature Landscape

The product is an operator dashboard for **on-chain contract supply and issuance**, not a global stablecoin market-cap or reserve-attestation service. The project specification defines the MVP boundary; the external research below corrects one material assumption within that boundary.

### Table Stakes (Users Expect These)

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| USDT and USDC Mainnet ingestion | These are the only two in-scope assets; missing either fails the product goal | HIGH | Filter logs by the configured contract, with bounded block ranges and a configurable confirmed head. For USDC use zero-address `Transfer`; for USDT include `Issue` and `Redeem`. [S1][S2][S3] |
| Correct supply-changing event classification | Daily issuance and reconciliation are meaningless if events are missed or counted twice | HIGH | Circle's mint/burn also emit `Mint`/`Burn` events; count one canonical representation. Tether's issue/redeem do **not** emit zero-address `Transfer`. `DestroyedBlackFunds` reduces USDT supply and needs distinct treatment. [S2][S3] |
| Restartable, duplicate-safe and reorg-aware sync | An operator needs metrics to remain stable through retries, restarts and head changes | HIGH | Persist progress only after accepted events; identify canonical block hashes and remove/replay invalidated ranges. Confirmations and lag must be visible. [S1][P1] |
| Current supply by token and combined total | First question in the specification; also a familiar stablecoin analytics view | MEDIUM | Separate contract supply from calculated supply; display data timestamp and block so staleness is apparent. Combined total is sum of USDT and USDC token units, assuming their common USD peg for display only. [P1][C1] |
| Daily mint, burn, net; rolling 24h, 7d, 30d net | The operator's core issuance questions | MEDIUM | Define UTC calendar day separately from trailing 24h; derive from stored normalized issuance events in 6-decimal token units. [P1] |
| Supply trend, issuance charts and large-event table | Allows scanning for recent changes and investigating a transaction | MEDIUM | Grafana panels for token/total supply, daily issuance and threshold-filtered events with block/transaction links. Thresholds remain configurable; notification delivery is out of scope. [P1] |
| Contract `totalSupply()` sampling and difference | Independent consistency check for event-derived supply | HIGH | Read at the same canonical block as the calculation. A history start or same-block anchor is mandatory; a zero difference cannot be promised from an arbitrary recent log start. [S2][S3][P1] |
| Health and explicit failure signals | A quiet indexer could make stale numbers look current | MEDIUM | Show processed/target block, lag, last successful update, RPC/indexer errors and reconciliation mismatch in logs/metrics. [P1] |
| One-command local operation | The specified users need a reproducible monitor | MEDIUM | Docker Compose runs indexer, ClickHouse and Grafana; external RPC URL is configured, not embedded. [P1] |

### Differentiators (Within This MVP)

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Independently reconciled on-chain supply | Makes a dashboard figure auditable rather than only plausible | HIGH | Show contract value, calculated value, difference and comparison block together. This is a project value claim, not proof of issuer reserves. [P1][C2] |
| Traceable large issuance events | Gives an operator a direct path from a change in aggregate supply to the chain event | LOW | Preserve transaction hash, log index, block, timestamp and raw amount; filter the event table by configurable threshold. [P1] |
| Recoverable raw-event history | Permits metric repair after decoder or query changes without rescanning every block | MEDIUM | Keep canonical raw supply events and rebuild derived views. USDT event kinds must remain distinguishable. [P1][S2] |

These are differentiators for this **focused monitor**, not claims that other analytics services lack them. DefiLlama already offers Ethereum stablecoin totals and 1d/7d/1m changes; issuer transparency pages address a different, reserve-backed question. [C1][C2]

### Anti-Features (Deliberate Exclusions)

| Feature | Why Requested | Why Problematic Now | Alternative |
|---------|---------------|---------------------|-------------|
| More chains or tokens | Broader market coverage | Each contract and chain has distinct supply semantics and reconciliation paths | Prove both named Ethereum contracts first; revisit adapters after acceptance. [P1][S2][S3] |
| All ERC-20 transfers, wallet labels, exchange/whale or DeFi flows | Rich behavioral analysis | Adds high-volume unrelated data and changes the product question | Retain only supply-relevant events and transaction links. [P1] |
| Notification delivery to Telegram, Discord, email or webhooks | Immediate awareness | Adds delivery credentials, retries and operational burden before event correctness | Show large events and mismatch visibly in Grafana/logs. [P1] |
| Custom API or web frontend | More tailored UX | Duplicates the specified Grafana presentation path | Query ClickHouse from Grafana. [P1] |
| Global circulating supply, reserve coverage or exact USD valuation | Familiar stablecoin market language | Ethereum contract `totalSupply()` does not by itself establish global circulation, redemption backing, or a live dollar price | Label results as Ethereum on-chain token supply and token-denominated amounts. [S4][C1][C2] |
| Treating all zero-address transfers as universal issuance | Simple generic decoder | ERC-20 recommends, but does not require, mint-time zero-address `Transfer`; USDT is a concrete counterexample | Use the minimum contract-specific event mapping for the two MVP assets. [S4][S2][S3] |

## Feature Dependencies and Roadmap Order

```text
Configured RPC and token contracts
  -> bounded canonical block ingestion
  -> USDC Transfer + USDT Issue/Redeem/DestroyedBlackFunds decoding
  -> idempotent raw storage and checkpoint/reorg recovery
  -> daily/rolling issuance and large-event views

Historical start or same-block supply anchor + complete supply adjustments
  -> calculated supply
  -> same-block totalSupply sampling
  -> meaningful reconciliation and supply trend

Stored metrics + indexer health
  -> Grafana operator dashboard
```

- **USDT source semantics are a gating dependency:** A `Transfer`-only proof can validate RPC access and USDC, but cannot satisfy USDT issuance or reconciliation. Tether's constructor also initializes supply without an event, and its contract can delegate `totalSupply()` after deprecation. [S2]
- **Reconciliation requires a defined baseline:** Either index from deployment with an explicit initial-supply anchor, or sample a contract supply anchor at the first indexed block and calculate changes from there. The latter validates forward movement, not all prior history. Match snapshot block and accepted event cutoff. [S2][P1]
- **Supply destruction needs its own label:** `DestroyedBlackFunds` changes USDT supply but is not the contract's `Redeem` event. Include it in the supply equation and expose it separately or explicitly define whether it contributes to the displayed burn total. This product definition needs resolution before daily metrics are accepted. [S2]
- **Daily boundaries need a single convention:** Use UTC for calendar-day views; trailing 24h remains a separate window. The specification does not set a timezone. [P1]

## MVP Definition

### Launch With (v1)

1. **Reliable ingestion and raw event ledger:** USDC zero-address `Transfer` and USDT `Issue`/`Redeem` plus supply-destruction adjustments, with confirmed-block processing, replay, deduplication and checkpoint recovery. [S1][S2][S3][P1]
2. **Reconciled supply:** establish a documented historical start or same-block anchor; sample both contracts' `totalSupply()` and report the exact-block difference and stale/mismatch state. [S2][P1]
3. **Issuance and investigation views:** UTC daily mint/burn/net; trailing 24h/7d/30d net; supply trend; configurable large-event table; Grafana dashboard. [P1]
4. **Operational visibility and repeatable startup:** logs, lag/error metrics and Docker Compose with external RPC configuration. [P1]

### Add After Validation (v1.x)

Only optimize query tables or switch to finalized-tag selection if actual dashboard latency, backfill cost or reorg behavior warrants it. These are implementation upgrades, not promised new user features. [P1]

### Future Consideration (v2+)

Other chains/tokens, notifications, labels, custom API/web UI and cross-chain analysis require a separately agreed scope. [P1]

## Feature Prioritization Matrix

| Feature | User Value | Cost | Priority |
|---------|------------|------|----------|
| Correct two-token supply-event ingestion | HIGH | HIGH | P1 |
| Canonical replay, idempotency and recovery | HIGH | HIGH | P1 |
| Supply anchor and same-block reconciliation | HIGH | HIGH | P1 |
| Daily/rolling metrics and large events | HIGH | MEDIUM | P1 |
| Grafana panels and health visibility | HIGH | MEDIUM | P1 |
| Additional chains, alerts or custom frontend | LOW for this MVP | HIGH | P3 |

## Existing Product Reference

| Existing view | Observed feature | Implication for this project |
|---------------|------------------|------------------------------|
| DefiLlama Ethereum stablecoins [C1] | Chain-level total and token rows, with short-period changes | Supply and change charts are expected; the reason to build this monitor is verified event provenance and contract-level reconciliation. |
| Circle transparency [C2] | Issuer-side circulation and reserve assurance | Do not present on-chain `totalSupply()` as a reserve or attestation measure. |
| Etherscan USDT token page [S2] | Contract, supply and transaction inspection | Preserve hashes/blocks so a dashboard user can inspect source transactions. |

## Sources and Confidence

- [P1] Local [requirements.md](../../requirements.md) and [PROJECT.md](../PROJECT.md): **HIGH for intended scope**, not evidence of implemented behavior.
- [S1] [Ethereum Execution API `eth_getLogs`](https://ethereum.github.io/execution-apis/next/api/methods/eth_getLogs/): **MEDIUM**, current protocol reference located via `websearch` and cross-checked with project RPC requirements.
- [S2] [Tether USDT contract source](https://github.com/tethercoin/USDT/blob/main/TetherToken.sol) and [Etherscan contract ABI](https://etherscan.io/token/0xdac17f958d2ee523a2206206994597c13d831ec7): **MEDIUM**, primary source and deployed-contract listing agree; verify live deployment and event samples during implementation.
- [S3] [Circle FiatToken design](https://github.com/circlefin/stablecoin-evm/blob/master/doc/tokendesign.md) and [FiatTokenV1 source](https://github.com/circlefin/stablecoin-evm/blob/master/contracts/v1/FiatTokenV1.sol): **MEDIUM**, issuer design and code agree; verify current Ethereum proxy implementation during implementation.
- [S4] [ERC-20 EIP-20](https://eips.ethereum.org/EIPS/eip-20): **MEDIUM**, current standard text.
- [C1] [DefiLlama Ethereum stablecoin view](https://defillama.com/stablecoins/ethereum): **MEDIUM**, directly observed on 2026-10-08; product presentation changes over time.
- [C2] [Circle transparency](https://www.circle.com/transparency): **MEDIUM**, issuer page observed on 2026-10-08; used only to distinguish reserve assurance from on-chain supply.

## Open Questions for Planning

1. Should USDT `DestroyedBlackFunds` appear under a separate "supply adjustment" metric or inside burn, with an explicit subtype? It must affect calculated supply either way.
2. What initial block or anchor will make calculated supply meaningful, and does the chosen RPC provider retain the required history and same-block `eth_call` access?
3. What mismatch tolerance and persistence rule should trigger an operator warning? Exact equality is the target for correctly aligned integer-unit snapshots, but a one-off bad RPC result should be distinguishable from persistent divergence.
4. Is the spec's dollar sign a display shorthand for six-decimal token units, or is a live FX rate actually required? The MVP has no price source, so label amounts in USDT/USDC unless explicitly changed.

---
*Feature research for: Ethereum stablecoin issuance monitor*
