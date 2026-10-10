---
last_mapped_commit: 1ab6dc53859f4533eb1563dc0b95eafe9e4e172e
last_mapped_at: 2026-10-10
---
# Codebase Concerns

**Analysis Date:** 2026-10-08

The repository contains `go.mod` and `requirements.md`; no application, migration, deployment, or test files exist. The schema and workflow risks below concern proposals in `requirements.md`, not defects in running code.

## Tech Debt

**Implementation baseline:**
- Issue: The MVP described in `requirements.md` has no Go source, ClickHouse migrations, configuration, or Grafana assets yet.
- Files: `go.mod`, `requirements.md`
- Impact: None of the acceptance criteria in `requirements.md` can currently run or be verified.
- Fix approach: Implement the first vertical path from RPC logs through mint/burn decoding to persisted events, then add checkpointing, reconciliation, and dashboards.

## Known Bugs

**No executable behavior to assess:**
- Symptoms: Not detected; `go.mod` declares a module but the repository has no Go packages.
- Files: `go.mod`, `requirements.md`
- Trigger: Not applicable.
- Workaround: Not applicable.

## Security Considerations

**External RPC and database credentials:**
- Risk: RPC URLs can contain provider credentials; deployment and logs could expose them if printed or committed.
- Files: `requirements.md`
- Current mitigation: `requirements.md` calls for environment-based RPC configuration and no hard-coded URL; there is no implementation.
- Recommendations: Keep credentials out of committed configuration and redact connection strings in errors and logs when the client is added.

## Performance Bottlenecks

**Historical log backfill:**
- Problem: A large fixed `eth_getLogs` range may exceed provider limits or time out during backfill.
- Files: `requirements.md`
- Cause: The proposed indexer uses block ranges and an external RPC provider, but does not specify adaptive range sizing.
- Improvement path: Bound range sizes, retry transient failures with backoff, and split ranges when the provider rejects a query.

**Repeated metric aggregation:**
- Problem: Dashboard queries over all raw events can become expensive as history grows.
- Files: `requirements.md`
- Cause: `stablecoin_daily_metrics` is suggested but has no defined refresh or reconciliation process.
- Improvement path: Start with queries over raw events; materialize daily rows when query cost warrants it, with deterministic recomputation for affected days.

## Fragile Areas

**Event uniqueness and checkpointing:**
- Files: `requirements.md`
- Why fragile: The proposed `stablecoin_events` table uses `ReplacingMergeTree(created_at)`, whose replacement is eventual; ordinary reads can see duplicates before merges. The proposed checkpoint also uses `ReplacingMergeTree`, so multiple versions may be visible. Advancing a checkpoint separately from event writes can skip events after a crash.
- Safe modification: Define deterministic event identity and deduplicated read or write behavior, then advance progress only after event persistence succeeds; replay an overlap on restart.
- Test coverage: No tests exist.

**Chain reorganizations:**
- Files: `requirements.md`
- Why fragile: A configurable 20-block delay reduces exposure but does not itself detect or replace events from a changed canonical block. The event sort key omits `block_hash`, so a replay of a changed block needs explicit invalidation rules.
- Safe modification: Store and compare canonical block hashes near the checkpoint; on mismatch, rewind and replace events and derived metrics for affected blocks.
- Test coverage: No tests exist.

**Supply reconciliation:**
- Files: `requirements.md`
- Why fragile: The formula requires an initial supply or full mint/burn history. The example backfill starts at block 20,000,000, while the snapshot text says calculated supply comes from mint minus burn; that cannot equal current `totalSupply()` without a baseline. Calls at different block heights also produce false mismatches.
- Safe modification: Establish a supply baseline at the exact backfill boundary and compare computed and contract supply at the same block height; record the block number with each snapshot.
- Test coverage: No tests exist.

**Daily metric rows:**
- Files: `requirements.md`
- Why fragile: The proposed `stablecoin_daily_metrics` `MergeTree` allows multiple rows for one chain/token/date, so reruns or late events can double-count in dashboards.
- Safe modification: Recompute each affected day from raw events and use a query or storage strategy that selects one authoritative daily value.
- Test coverage: No tests exist.

## Scaling Limits

**External RPC quotas:**
- Current capacity: Not measured; only a provider requirement appears in `requirements.md`.
- Limit: Provider rate limits and `eth_getLogs` range caps will constrain catch-up speed.
- Scaling path: Measure request throughput and lag first, then tune bounded concurrency and range sizes within provider quotas.

## Dependencies at Risk

**Not detected:**
- Risk: `go.mod` has no third-party dependencies; there is no package-specific risk to assess.
- Impact: Not applicable.
- Migration plan: Reassess after dependencies are selected.

## Missing Critical Features

**MVP runtime and persistence:**
- Problem: There is no executable indexer, RPC client, event decoder, ClickHouse schema, checkpoint, supply reconciliation, metrics, or Grafana deployment.
- Blocks: Every functional acceptance criterion in `requirements.md`.

## Test Coverage Gaps

**All acceptance paths:**
- What's not tested: Mint/burn decoding, duplicate replay, restart recovery, reorg handling, supply reconciliation, and daily/rolling issuance.
- Files: `requirements.md`, `go.mod`
- Risk: Monetary totals and progress could be wrong without a detectable failure.
- Priority: High; add focused tests alongside each implementation step.

---

*Concerns audit: 2026-10-08*
