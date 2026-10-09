# Ethereum Stablecoin Monitor

Indexes USDT and USDC supply-changing events on Ethereum Mainnet: a Go
indexer continuously follows the configured eligible head (finalized by
default), reads each contract's supply events in bounded windows, and stores
them with exact raw amounts and full provenance in ClickHouse for SQL
inspection. Restarts resume from a durable block-number-and-hash checkpoint;
replays collapse to one row per event. Dashboards come later.

## 1. Prerequisites

- Go 1.27.1 (pinned by `go.mod`)
- Docker, for the ClickHouse server
- A free-tier Ethereum Mainnet RPC provider key — Alchemy, Infura, or
  QuickNode all work. Create a project in the provider dashboard and copy its
  Ethereum Mainnet HTTP endpoint. Keep the key in your environment only;
  it is never committed to this repository (decision D-01).

## 2. Storage (ClickHouse)

Start the pinned ClickHouse LTS container once:

```sh
docker run -d --name tm-clickhouse \
  -p 127.0.0.1:9000:9000 \
  -e CLICKHOUSE_SKIP_USER_SETUP=1 \
  clickhouse/clickhouse-server:26.8.20.9
```

- `-p 127.0.0.1:9000:9000` binds the native protocol to loopback only —
  the database is not reachable from outside the machine.
- `CLICKHOUSE_SKIP_USER_SETUP=1` keeps the stock `default` user usable from
  the host through the loopback mapping (the 26.8 image otherwise restricts
  `default` to connections originating inside the container).

For later runs: `docker start tm-clickhouse`

## 3. Configuration

Required — the RPC endpoint (replace with your own provider endpoint; never
a real key in a file):

```sh
export ETH_RPC_URL=<your-mainnet-http-endpoint>
```

Optional — the ClickHouse DSN (this is also the default when unset):

```sh
export CLICKHOUSE_URL=clickhouse://default@127.0.0.1:9000/default
```

Token metadata, head policy, and window sizing live in `config/tokens.json`:

| Field | Default | Meaning |
|-------|---------|---------|
| `chain_id` | — | Must be 1 (Ethereum Mainnet); the provider probe rejects anything else |
| `head_policy` | `finalized` | Which head the sync follows: `finalized` (consensus finalized tag, default — reorg-safe) or `confirmed` (latest minus `confirmation_blocks`) |
| `confirmation_blocks` | 20 | Only used when `head_policy` is `confirmed`; blocks at or below `head - confirmation_blocks` are eligible |
| `window_blocks` | 5000 | Blocks per `eth_getLogs` window per token |
| `window_floor` | 100 | Lower bound for window halving on provider result caps |
| `poll_seconds` | 60 | Sleep between polls once caught up with the head |
| `start_block` | unset | Backfill opt-in: seeds the checkpoint at this block. Unset = start at the current eligible head and index forward only |
| `tokens[].symbol` | — | `USDT` or `USDC` (MVP scope) |
| `tokens[].issuer` | — | Documentation only (Tether, Circle) |
| `tokens[].contract` | — | The deployed token contract address (must be unique across tokens) |
| `tokens[].decimals` | — | Raw-unit scale for human reading (6 for both tokens) |

## 4. Continuous ingest

```sh
go run ./cmd/indexer run            # or just: go run ./cmd/indexer
go run ./cmd/indexer -from <N>      # seed the checkpoint at block N (backfill opt-in)
```

Behavior:

- The indexer follows the configured `head_policy` head in bounded windows
  of `window_blocks` and keeps polling (`poll_seconds`) as new eligible
  blocks arrive. Ctrl-C stops it cleanly between windows.
- Every window is verified and checkpointed: the durable checkpoint records
  `(block_number, block_hash)` and advances only after the window's events
  are durably stored. A crash or restart resumes exactly at the checkpoint;
  a crash between the event insert and the checkpoint write simply makes the
  restart re-process that window — replays collapse to one row per event
  (ReplacingMergeTree keyed on full event identity, read with FINAL), so
  re-running is always safe.
- On startup without a checkpoint, the run seeds at `start_block` when
  configured (or the `-from` flag), otherwise at the current eligible head,
  and indexes forward only — it never back-fills history unless you ask.
- If the provider's canonical header at the checkpointed height no longer
  matches the checkpointed hash (a displaced finalized block — a
  catastrophic consensus or hostile-provider event), the indexer halts
  ordinary indexing with a loud diagnostic naming the block and both hashes
  and exits non-zero. Recovery is a manual, verified rewind (a `rewind`
  subcommand lands in a later phase); the indexer never auto-heals this.
- The fail-fast provider probe (chain id 1, confirmed-range boundary,
  historical `eth_getLogs` sample) still runs before any ingest.
- Free-tier providers rate-limit bursty requests; the indexer automatically
  retries throttled requests (HTTP 429 / JSON-RPC -32005) with bounded
  backoff.
- The Phase 1 bounded `-from <N> -to <M>` one-shot mode is superseded by the
  continuous loop; `-from` now means "seed the checkpoint at N".
- Mind the provider's per-query result cap. Infura, for example, rejects an
  `eth_getLogs` returning more than 10,000 logs. The default 5000-block
  window is safe for USDT (its supply topics are rare); dense USDC ranges
  may still need a smaller `window_blocks` until automatic window halving
  lands.

## 5. Inspection

```sh
docker exec -it tm-clickhouse clickhouse-client --query "
SELECT token, event_type, raw_amount, contract_address, block_number, block_hash,
       block_time, tx_hash, log_index, from_address, to_address
FROM stablecoin_events FINAL
ORDER BY block_number DESC, log_index DESC
LIMIT 20"
```

- `event_type` vocabulary: `mint`, `redeem`, `destroyed_black_funds`, `burn`.
- `raw_amount` is the exact on-chain uint256 in raw units — never a float.
  With 6 decimals, `1000000` means 1 token.
- `FINAL` is mandatory on reads: background replacement in ReplacingMergeTree
  is eventual, and FINAL collapses re-ingested duplicates deterministically.
- Ordinary transfers never produce rows: USDT supply moves only through
  `Issue`/`Redeem`/`DestroyedBlackFunds`, and USDC rows come only from
  zero-address `Transfer` logs (paired `Mint`/`Burn` logs corroborate but are
  not stored, so nothing is double-counted).

## 6. Tests

Offline (no environment needed):

```sh
go test ./...
```

Environment-gated suites:

```sh
# ClickHouse exactness: UInt256 round trip incl. 2^256-1, replay idempotence
CLICKHOUSE_URL=clickhouse://default@127.0.0.1:9000/default \
  go test ./internal/storage -v -count=1

# Live chain proof: five pinned Mainnet fixtures classified through your
# provider, paired-event no-double-count, USDT-Transfer negative path.
# Requires a provider with historical eth_getLogs; skips when unset.
ETH_RPC_URL=<your-mainnet-http-endpoint> \
  go test ./internal/indexer -run TestLive -v -count=1
```
