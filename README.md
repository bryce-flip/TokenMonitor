# Ethereum Stablecoin Monitor

Indexes USDT and USDC supply-changing events on Ethereum Mainnet: a Go
indexer reads each contract's supply events over a bounded block range and
stores them with exact raw amounts and full provenance in ClickHouse for SQL
inspection. Phase 1 scope: bounded, operator-invoked runs; continuous sync
and dashboards come later.

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

Token metadata and confirmation policy live in `config/tokens.json`:

| Field | Meaning |
|-------|---------|
| `chain_id` | Must be 1 (Ethereum Mainnet); the provider probe rejects anything else |
| `confirmation_blocks` | Only blocks at or below `head - confirmation_blocks` may be ingested |
| `tokens[].symbol` | `USDT` or `USDC` (MVP scope) |
| `tokens[].issuer` | Documentation only (Tether, Circle) |
| `tokens[].contract` | The deployed token contract address |
| `tokens[].decimals` | Raw-unit scale for human reading (6 for both tokens) |

## 4. Bounded ingest

```sh
go run ./cmd/indexer -config config/tokens.json -from <N> -to <M>
```

Rules and tips:

- `-from` and `-to` are inclusive and must be given together; `-from <= -to`.
- `-to` must not exceed `head - confirmation_blocks` (20 with the shipped
  config). The fail-fast provider probe enforces this and exits non-zero
  otherwise, before any data is fetched (decision D-02). The probe also
  verifies chain id 1 and that the provider serves historical `eth_getLogs`.
- To pick a recent confirmed window: get the current head (any block
  explorer, or an `eth_blockNumber` call against your endpoint), then ingest
  `head - confirmation_blocks - N` through `head - confirmation_blocks`.
- Mind the provider's per-query result cap. Infura, for example, rejects an
  `eth_getLogs` returning more than 10,000 logs and names a suggested
  sub-range in the error. USDC emits roughly 70-140 Transfer logs per block,
  so keep windows under ~40 blocks there (USDT-only ranges can be much
  larger — its supply topics are rare).
- A run is idempotent: re-ingesting the same range collapses to one row per
  event (ReplacingMergeTree keyed on full event identity, read with FINAL),
  so a failed run can simply be retried with the same command.
- Free-tier providers rate-limit bursty requests; the indexer automatically
  retries throttled requests (HTTP 429 / JSON-RPC -32005) with bounded
  backoff. Sustained throttling can still fail a run — re-running the same
  command is safe and cheap (idempotent).

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
