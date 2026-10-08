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
ORDER BY (chain, token, block_number, tx_hash, log_index)
