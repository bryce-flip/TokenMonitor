-- stablecoin_events: one row per classified supply-changing event.
-- Phase 1 decisions (01-01-PLAN.md):
--   * raw_amount UInt256 is the exact on-chain ledger value; the spec's
--     Decimal(38,6) scaled column returns with Phase 4 presentation.
--   * ReplacingMergeTree(created_at) keyed on the full event identity
--     (chain, token, block_number, tx_hash, log_index) makes re-ingesting
--     the same range idempotent under FINAL reads; background replacement
--     is eventual, so all correctness-sensitive reads use FINAL.
CREATE TABLE IF NOT EXISTS stablecoin_events
(
    chain            String,
    token            String,
    contract_address String,
    block_number     UInt64,
    block_hash       String,
    block_time       DateTime,
    tx_hash          String,
    log_index        UInt32,
    event_type       LowCardinality(String),
    from_address     String,
    to_address       String,
    raw_amount       UInt256,
    created_at       DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(created_at)
ORDER BY (chain, token, block_number, tx_hash, log_index);

-- indexer_checkpoint: the durable sync resume point (SYNC-03). The sync loop
-- writes one row per window advance, only AFTER that window's
-- stablecoin_events insert returned nil (advance-after-accept), so a crash
-- between the two writes leaves the window un-checkpointed and a restart
-- re-processes it (FINAL dedup collapses the replay).
--   * ReplacingMergeTree(height): the version column is the block height, so
--     the merged survivor is ALWAYS the greatest height even when writes
--     arrive out of order. ReplacingMergeTree(updated_at) would instead let
--     a stale late write win the merge (02-RESEARCH Pattern 2, probe-verified
--     on the pinned 26.8.20.9 server).
--   * Reads use ORDER BY height DESC LIMIT 1 — never FINAL: background
--     replacement is eventual, and the ordered read returns the max height
--     both pre-merge (over physical rows) and post-merge (over the survivor).
CREATE TABLE IF NOT EXISTS indexer_checkpoint
(
    chain      String,
    height     UInt64,
    block_hash String,
    updated_at DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(height)
ORDER BY (chain);
