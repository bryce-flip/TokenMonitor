# Pitfalls Research

**Domain:** Ethereum Mainnet USDT/USDC issuance and supply monitoring
**Researched:** 2026-10-08
**Confidence:** MEDIUM (official contract and product documentation, retrieved through web search; no live-chain fixture was tested)

## Critical Pitfalls

### 1. Treating ERC-20 zero-address Transfers as the universal supply ledger

**What goes wrong:** USDT mints and burns disappear from the index. Its `issue` and `redeem` functions alter `_totalSupply` and emit `Issue`/`Redeem`, without emitting `Transfer`. `destroyBlackFunds` also reduces supply and emits `DestroyedBlackFunds`, without emitting `Transfer`. By contrast, Circle documents both dedicated events and zero-address `Transfer` events for USDC mint/burn. Combining USDC's dedicated events with its Transfer events would double count. [S1][S2][S3]

**Why it happens:** The specification applies a generic ERC-20 heuristic to two contracts with different event semantics. ERC-20 says a mint *should* emit a zero-address Transfer, not that every deployed token does. [S1][S3]

**How to avoid:** Define a contract-specific supply-event map before implementing the decoder: USDT `Issue` = mint; `Redeem` and `DestroyedBlackFunds` = distinct burn reasons. USDC zero-address Transfer = mint/burn; its `Mint`/`Burn` events are corroborating evidence, not additional issuance. Preserve source signature, transaction hash, log index, raw units, and burn reason so classifications can be rebuilt. Test one real historical transaction for each path. [S1][S2]

**Warning signs:** Large USDT `totalSupply()` movement with zero indexed issuance; USDC totals exactly twice contract changes; no USDT `Issue` rows despite on-chain events.

**Phase to address:** First RPC-to-log-to-ClickHouse proof, before dashboards or supply arithmetic.

### 2. Calling a USDT transfer to zero address a burn

**What goes wrong:** A `Transfer(from, 0x0, amount)` can be counted as USDT destruction even though the legacy `transfer`/`transferFrom` implementation only moves balances and leaves `_totalSupply` untouched. [S1]

**Why it happens:** Zero-address transfers usually mean burns in newer ERC-20 implementations, but that meaning is not enforced by the event alone. [S1][S3]

**How to avoid:** Do not derive USDT burns from zero-address Transfer. Record them as transfers if needed for diagnostics; count only the USDT supply-changing contract events. A regression fixture should show unchanged supply after a zero-address transfer, versus decreased supply after `redeem` or `destroyBlackFunds`.

**Warning signs:** Calculated USDT supply falls on a transaction with no `Redeem` or `DestroyedBlackFunds`; contract supply remains unchanged.

**Phase to address:** Token-specific decoder and fixture validation.

### 3. Calculating absolute supply without an opening balance or a common block

**What goes wrong:** `sum(mint)-sum(burn)` from a recent starting block is called current supply and always disagrees with `totalSupply()`. The deployed USDT constructor had a nonzero `_initialSupply` of 100,000,000,000 raw units (100,000 USDT at six decimals), assigned without a mint log. Comparing events through block H with a `latest` `eth_call` also creates a false mismatch when supply changes after H. [S1][S4][S8]

**Why it happens:** The proposed event table and snapshot omit an explicit start block, opening supply, and snapshot block hash. Ethereum state calls use the block parameter supplied to `eth_call`; `latest` is a moving target. [S4]

**How to avoid:** Choose either a full historical reconstruction with a verified USDT constructor amount, or a named supply anchor: contract `totalSupply()` at block A, then apply supply deltas from A+1 through H. Store anchor and every snapshot with chain ID, token address, block number/hash, raw integer units, and calculation version. Read `totalSupply()` at the same H used for event aggregation; verify historical-state RPC availability before promising an old anchor. [S1][S4]

**Warning signs:** Constant large offset from day one; discrepancy changes with sync lag; a historical `eth_call` fails even though old logs are available.

**Phase to address:** Supply model and reconciliation, before declaring the dashboard's current supply valid.

### 4. Assuming ReplacingMergeTree provides a unique constraint or atomic checkpoint

**What goes wrong:** Replayed inserts create temporarily visible duplicate rows, doubling `sum(amount)` in Grafana. An advanced checkpoint can permanently skip an event batch after a failed insert; writing events first can replay them after a failed checkpoint. A checkpoint table using ReplacingMergeTree may likewise contain multiple physical versions. [S5]

**Why it happens:** ClickHouse replacement occurs during asynchronous merges, keyed by `ORDER BY`; it is not an immediate uniqueness guarantee. Event and checkpoint writes are separate operations. [S5]

**How to avoid:** Give each canonical log a stable identity (`chain_id`, token contract, block hash, transaction hash, log index), use a deterministic replay policy, and make all correctness-sensitive queries deduplicate explicitly (`FINAL` or equivalent grouped latest-version logic). Commit a checkpoint only after the event insert succeeds; read the greatest completed checkpoint version, not an arbitrary row. Exercise an injected crash between the two writes and assert one logical event plus an unskipped range on restart. For a reorg, distinguish orphaned rows from canonical replacements rather than letting both count. [S5]

**Warning signs:** Totals change after background merges; duplicate checkpoint rows; event count doubles immediately after restart; a gap appears after a storage timeout.

**Phase to address:** ClickHouse schema and restart recovery, with a crash/replay test before metrics.

### 5. Treating a fixed confirmation count as a complete reorg strategy

**What goes wrong:** Orphaned logs remain in the database after the chain replaces a processed block; duplicate or missing issuance persists even though the indexer resumes from its numeric checkpoint. [S4][S6]

**Why it happens:** `latest - 20` lowers risk but does not prove that the previously stored block hash is still canonical. Ethereum exposes safe/finalized tags and block hashes precisely because canonical state matters. [S4][S6]

**How to avoid:** Prefer the `finalized` head when the RPC supports it; otherwise make confirmation depth configurable and retain a bounded overlap. Store block hash with checkpoint/range metadata and verify the last committed hash against the provider on restart and at the moving boundary. On mismatch, rewind to a common ancestor, retire orphaned rows, and rebuild affected snapshots/aggregates before publishing them. Test a simulated same-height block-hash replacement. [S4][S6]

**Warning signs:** Stored block hash differs from RPC at the checkpoint height; supply mismatch begins at one height and survives retries; more than one canonical-looking row for a log position.

**Phase to address:** Block synchronization/reorg handling, before durable checkpoint claims.

### 6. Ignoring token upgrade/deprecation paths

**What goes wrong:** Reconciliation silently switches meaning or log indexing goes incomplete after a contract change. USDT `totalSupply()` delegates to `upgradedAddress` when `deprecated` is true. USDC is proxy-upgradeable; the same proxy address can gain new implementation behavior. [S1][S2]

**Why it happens:** A static address and ABI are mistaken for an immutable supply model.

**How to avoid:** Sample USDT `deprecated` and `upgradedAddress` alongside supply; monitor `Deprecate` and stop treating the old event map as complete after a transition until the new implementation is examined. For USDC, monitor proxy implementation changes and revalidate mint/burn fixtures against the new code. Flag this as a contract-semantics change, not a routine supply mismatch. [S1][S2]

**Warning signs:** `totalSupply()` jumps while legacy event count remains flat; `Deprecate` appears; USDC implementation changes while decoder version stays fixed.

**Phase to address:** Token adapter and reconciliation design; live upgrade monitoring can follow the first correct backfill.

## Moderate Pitfalls

| Pitfall | Consequence | Prevention | Phase |
|---------|-------------|------------|-------|
| Treating USDT `DestroyedBlackFunds` as ordinary redemption | Burn totals hide administrative confiscation | Preserve `burn_reason` while including it in supply decrease; expose a separate category if displayed | Decoder/metrics |
| Losing precision through floating point or early decimal conversion | Nonzero drift and incorrect large-event threshold | Keep raw `uint256` amounts as exact integers; scale by verified token decimals only for presentation | Schema/metrics |
| Backfilling with a provider that limits old log ranges or lacks old state | Silent gaps or an unworkable historical anchor | Probe historical `eth_getLogs` and `eth_call`, split ranges, retry rate limits, and fail closed on incomplete ranges | RPC ingestion |
| Materializing daily totals from non-deduplicated inserts | Duplicate or orphaned records permanently poison rollups | Build rollups from canonical, deduplicated raw events and regenerate affected days after replay/reorg | Metrics |
| Reading `latest` supply while graphs use finalized events | Repeated transient false discrepancy alerts | Put event head, contract-call block, and finality status in each snapshot and alert only on aligned blocks | Reconciliation |

## Integration And Dashboard Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| Ethereum RPC | Querying only the Transfer topic for both tokens | Query USDT's supply-event topics as well; token-specific topic sets need explicit fixtures [S1][S2] |
| ClickHouse | Assuming `ORDER BY` enforces immediate uniqueness | Use a canonical/deduplicated read contract; test a replay before and after background merges [S5] |
| Grafana ClickHouse | Returning text timestamps or omitting `time` alias/time filter | Return `DateTime`/`DateTime64` as `time`, use the panel time-filter macro, and test UTC day boundaries [S7] |
| Grafana metrics | Showing stale calculated supply as a current value | Display indexed block/time, RPC head and sync lag beside supply; mark incomplete history explicitly |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Tiny ClickHouse inserts per log | Too many parts; merges lag; `FINAL` slows | Batch by bounded block range while respecting crash/replay boundaries [S5] | Sustained high log rate or long backfill |
| Full-history `FINAL` sums for every dashboard refresh | Slow panels as raw events grow | Derive bounded daily data from deduplicated canonical events after correctness is proven [S5] | Months of history with frequent refresh |
| Oversized `eth_getLogs` ranges | RPC timeout or provider result limits | Adaptive range splitting with explicit completeness checks [S4] | Backfills and high-traffic intervals |

## Security And Data Integrity

| Mistake | Risk | Prevention |
|---------|------|------------|
| Trusting an arbitrary RPC response as canonical | Incorrect supply and events from a lagged/misconfigured provider | Verify chain ID, checkpoint block hash, head status, and contract address; expose provider lag [S4] |
| Publishing an unqualified `difference != 0` alert | False financial signal from incomplete history, reorg, or mismatched block | Require a verified anchor and same-block contract call; report mismatch reason and indexed height [S1][S4] |

## "Looks Done But Isn't" Checklist

- [ ] USDT fixtures cover `Issue`, `Redeem`, `DestroyedBlackFunds`, and zero-address Transfer; supply deltas match contract behavior.
- [ ] USDC mint/burn fixtures produce one counted issuance despite two emitted event types.
- [ ] A documented opening balance and same-block `totalSupply()` call make absolute supply reproducible.
- [ ] Restart after an event insert but before checkpoint update leaves one logical event and no gap.
- [ ] A changed block hash retires orphaned events and recomputes affected metrics.
- [ ] Grafana shows as-of block/time and sync lag, and its 24h/7d/30d windows use block timestamps and explicit UTC semantics.

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Wrong token event model | HIGH if raw source logs were discarded | Add contract-specific topics, rescan affected blocks, replace raw rows, rebuild all supply and issuance metrics |
| Duplicated/orphaned rows | MEDIUM | Identify affected block interval, mark canonical block hashes, deduplicate on read, rebuild derived rows and checkpoint |
| Bad or absent supply anchor | MEDIUM | Establish a verified block-specific anchor, recalculate snapshots from that point, annotate earlier charts as unavailable |
| Wrong Grafana time query | LOW | Correct SQL time alias/filter/UTC bucket and revalidate panels against raw counts [S7] |

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| USDT/USDC event semantics | First vertical data path / decoder | Historical transaction fixtures and same-block `totalSupply()` deltas |
| Zero-address USDT false burn | Decoder | Transfer-to-zero fixture leaves supply unchanged |
| Reorg and checkpoint gaps | Sync/storage reliability | Simulated reorg and crash-between-writes checks |
| Unanchored or misaligned supply | Supply reconciliation | Rebuild from stored anchor; compare at identical block hash |
| Duplicated aggregates | Derived metrics | Replay a range and compare daily/rolling totals before and after merges |
| Misleading panels | Grafana/observability | UTC boundary checks; stale-head and mismatch states visible |

## Sources

- [S1] [Tether's USDT contract source](https://github.com/tethercoin/USDT/blob/main/TetherToken.sol): constructor, `issue`, `redeem`, `destroyBlackFunds`, `transfer`, `deprecate`, `totalSupply`. Official project source; retrieved 2026-10-08. Confidence: MEDIUM per research source classifier.
- [S2] [Circle FiatToken design](https://github.com/circlefin/stablecoin-evm/blob/master/doc/tokendesign.md): USDC mint/burn event behavior and proxy design. Official issuer documentation; retrieved 2026-10-08. Confidence: MEDIUM.
- [S3] [ERC-20 EIP-20](https://eips.ethereum.org/EIPS/eip-20): zero-address mint Transfer is a SHOULD, not a MUST. Standard; retrieved 2026-10-08. Confidence: MEDIUM.
- [S4] [Ethereum JSON-RPC API](https://ethereum.org/developers/docs/apis/json-rpc/): `eth_getLogs` filters, safe/finalized tags, and block-qualified `eth_call`. Official documentation; retrieved 2026-10-08. Confidence: MEDIUM.
- [S5] [ClickHouse ReplacingMergeTree and `FINAL` guidance](https://clickhouse.com/resources/engineering/clickhouse-optimize-table-final): asynchronous replacement and query-time correctness. Vendor documentation; published 2026, retrieved 2026-10-08. Confidence: MEDIUM.
- [S6] [Ethereum Gasper finality](https://ethereum.org/developers/docs/consensus-mechanisms/pos/gasper/): finalized canonical blocks. Official documentation; retrieved 2026-10-08. Confidence: MEDIUM.
- [S7] [Grafana ClickHouse query editor](https://grafana.com/docs/plugins/grafana-clickhouse-datasource/latest/query-editor/): time-series shape and time filters. Official plugin documentation; reviewed 2026-08-12, retrieved 2026-10-08. Confidence: MEDIUM.
- [S8] [USDT contract explorer record](https://etherscan.io/token/0xdac17f958d2ee523a2206206994597c13d831ec7): decoded constructor argument `_initialSupply = 100000000000` raw units. Explorer record; search-index excerpt retrieved 2026-10-08 because the page blocks automated opening. Confidence: MEDIUM.

**Open verification:** Historical mainnet fixtures, independent on-chain verification of USDT deployment arguments, provider support for historical state and finalized tags, and the deployed USDC implementation history require phase-specific live-chain checks.
