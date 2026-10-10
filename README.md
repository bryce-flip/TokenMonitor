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
  and exits non-zero. Recovery is the manual, verified rewind procedure in
  the next section; the indexer never auto-heals this.
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

## 5. Reorg safety — mismatch halt and the rewind procedure

The checkpoint stores `(block_number, block_hash)`. On every resume and
before every window ingest, the indexer re-fetches the provider's canonical
header at the checkpointed height and compares hashes. On a mismatch the
indexer stops ordinary indexing and reporting immediately — before any new
ingest — and exits non-zero with a diagnostic naming the block height, the
expected (checkpointed) hash, and the observed (canonical) hash. Nothing is
ever deleted or re-anchored automatically: a reorg deep enough to displace a
checkpointed finalized block is a catastrophic consensus or hostile-provider
event, and auto-healing would silently mask it (decision D-03).

Recovery is the operator-gated, verified rewind:

1. **Stop and verify the chain.** Confirm with an independent provider or
   explorer which chain is canonical at the mismatched height. The rewind
   below trusts the configured provider's headers; that trust is exactly
   what the operator must confirm first.
2. **Compute the plan** (this is what the subcommand does before touching
   anything): walk down from the checkpointed height while the stored
   per-block hashes disagree with the provider's canonical headers. The
   rewind point is the first height where they agree — or the first height
   with no stored rows. The walk is bounded at 1,000 blocks; deeper
   displacement aborts with a named error for operator investigation.
3. **Run the rewind and confirm:**

   ```sh
   go run ./cmd/indexer rewind          # prints the plan, then asks to confirm
   go run ./cmd/indexer rewind --yes    # scripted: skip the prompt
   go run ./cmd/indexer rewind --to N   # override the walked point (verified
                                       # against stored rows first; rejected
                                       # on any disagreement)
   ```

   The subcommand prints the expected/observed hashes, the verified rewind
   point, the delete range (`block_number > point` — rows at or below the
   point are never touched), and the FINAL event count and sum over the
   affected range **before and after** the delete. Anything but an explicit
   `yes` at the prompt aborts with nothing deleted. Any verification failure
   exits non-zero with nothing deleted.
4. **Restart the indexer** (`run`). It resumes from the re-anchored
   checkpoint and re-ingests the rewound range from the canonical chain;
   replayed events collapse to one row per event (ReplacingMergeTree + FINAL
   reads), including the reorg case where a re-included transaction lands
   with the same identity but a new block hash and amount.

## 6. Inspection

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
# ClickHouse exactness: UInt256 round trip incl. 2^256-1, replay idempotence,
# checkpoint ordering, rewind delete/re-insert/OPTIMIZE proofs
CLICKHOUSE_URL=clickhouse://default@127.0.0.1:9000/default \
  go test ./internal/storage -v -count=1

# Live chain proof: five pinned Mainnet fixtures classified through your
# provider, paired-event no-double-count, USDT-Transfer negative path.
# Requires a provider with historical eth_getLogs; skips when unset.
ETH_RPC_URL=<your-mainnet-http-endpoint> \
  go test ./internal/indexer -run TestLive -v -count=1
```

## 7. Localnet (Kurtosis devnet)

Restart/resume/replay behavior and a real deployed TetherToken are proven
against a Kurtosis local Ethereum testnet (decision D-02). The deterministic
reorg path (mismatch halt, ancestor walk, orphan exclusion) is covered
offline by the two-chain httptest harness in `internal/indexer/reorg_test.go`
— ethereum-package has no reorg knob, so the localnet covers real
consensus-driven finality, restart, and replay behavior only. **The localnet
is never a substitute for Mainnet classification proof: Mainnet pinned
fixtures remain the classification anchor.**

Inspect or (re)provision the enclave:

```sh
kurtosis enclave inspect eth-devnet    # note the el-1-geth-lighthouse rpc port
# if the enclave is gone:
kurtosis run github.com/ethpandaops/ethereum-package --enclave eth-devnet
```

Run the env-gated devnet suite (opt-in, exactly like the other gates; skips
with a pointer to the commands above when unset). Budget ~10 minutes: the
restart test catches up to the real finalized head twice, and the lifecycle
test deploys the token and syncs it once the chain has 64 blocks (2 epochs)
of confirmation depth on top — the enclave's beacon-finality signal itself
takes ~20 minutes to cover a tip block (batched epoch jumps), which does not
fit a single bounded test run; the finalized-head behavior is what the
restart test proves:

```sh
DEVNET_RPC_URL=http://127.0.0.1:<rpc-port> \
CLICKHOUSE_URL=clickhouse://default@127.0.0.1:9000/default \
  go test ./internal/indexer -run TestDevnet -v -count=1 -timeout 20m
```

The suite deploys the token from `testdata/usdt_creation.hex` using the
public ethereum-package prefunded test key (published fixture material, not a
secret) and drives `issue`/`redeem` through the real sync loop.

## 8. Test fixtures

`testdata/usdt_creation.hex` is the verbatim creation input (constructor
bytecode + arguments, 11,900 bytes / 23,800 hex characters, no `0x` prefix)
of the Mainnet TetherToken (USDT) deployment:

- transaction `0x2f1c5c2b44f771e942a8506148e256f94f1a464babc938ae0690c6e34cd79190`
- block 4,634,748 (`0x46b87c`), deployer `0x36928500bc1dcd7af6a2b4008875cc336b927d57`
- extracted 2026-10-09 via archive RPC (`eth_getTransactionByHash` against
  eth.drpc.org, fallback rpc.flashbots.net — public archive endpoints; the
  recipe is in `.planning/phases/02-reliable-canonical-indexing/02-RESEARCH.md`
  A3)

Deploying this input verbatim on any chain recreates the original contract
with identical event semantics — topic0 hashes are keccak of the canonical
signatures and are chain-independent — so the localnet lifecycle test
exercises the production decoder without a compiler or a Mainnet fork.
