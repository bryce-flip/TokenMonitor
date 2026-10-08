# Phase 1: Live Supply Event Path - Pattern Map

**Mapped:** 2026-10-08
**Files analyzed:** 8
**Analogs found:** 0 / 8 — greenfield repository (only `go.mod`, `requirements.md`, `AGENTS.md` are tracked; no Go source exists)

Every file below is new. No codebase analog exists, so per-file pattern assignments point to the closest **convention source**: `requirements.md`'s proposed layout/schema, `01-RESEARCH.md`'s verified code examples, and the pinned library APIs. Planner must reference these instead of non-existent in-repo files.

## File Classification

| New/Modified File | Role | Data Flow | Convention Source | Match Quality |
|-------------------|------|-----------|-------------------|---------------|
| `cmd/indexer/main.go` | entry point (CLI) | batch | `requirements.md:765-800` proposed tree; RESEARCH "Standard Stack" | none — greenfield |
| `internal/config/config.go` | config | transform | `requirements.md:66-116` (token table, YAML example, `ETH_RPC_URL`); RESEARCH recommends stdlib `encoding/json` + `os.Getenv` | none — greenfield |
| `internal/indexer/decoder.go` | service (decoder) | transform | RESEARCH "Deployed Contract Evidence" topic0 table + "Code Examples" length-check decode sketch | none — greenfield |
| `internal/rpc/client.go` (reader: getLogs + headers) | service | batch / request-response | RESEARCH `ethereum.FilterQuery` sketch (lines 134-142) | none — greenfield |
| `internal/storage/clickhouse.go` | service (writer) | batch insert | `requirements.md:230-261` `stablecoin_events` DDL; RESEARCH clickhouse-go `PrepareBatch`/`Append(*big.Int)` pattern | none — greenfield |
| `migrations/clickhouse.sql` | migration | — | `requirements.md:230-261` verbatim DDL | none — greenfield |
| `internal/indexer/decoder_test.go` | test | transform | RESEARCH "Validation Architecture" fixture table tests | none — greenfield |
| `internal/storage/clickhouse_test.go` (or smoke cmd) | test (integration) | batch | RESEARCH "Validation Architecture" EVENT-03 row | none — greenfield |

Note: `requirements.md:773-789` proposes `internal/indexer/`, `internal/token/`, `internal/storage/`, `internal/rpc/`-style layout (exact tree at lines 765-800). That tree is a proposal, not an established convention — planner may simplify (fewer files) per CONTEXT.md discretion, but should keep `cmd/indexer/main.go` as the entry point name.

## Pattern Assignments

Each new file copies from external convention sources, not in-repo code.

### `internal/config/config.go` (config, transform)

**Source:** `requirements.md` lines 66-90 (token metadata + YAML example), lines 109-116 (`ETH_RPC_URL` env var, never hardcode). RESEARCH override: use Go stdlib JSON instead of YAML (see "Don't Hand-Roll", RESEARCH line 160) unless planner chooses otherwise.

Config must carry (CHAIN-01): per-token symbol/contract/decimals, confirmation depth (default 20), chain_id 1, plus bounded `[from,to]` block range for this phase.

```yaml
# requirements.md:75-90 — semantic content to carry (encode as JSON per research)
tokens:
  - symbol: USDT
    contract: "0xdAC17F958D2ee523a2206206994597C13D831ec7"
    decimals: 6
```

Validation (fail-fast, RESEARCH V5): refuse missing `ETH_RPC_URL`, invalid address hex, invalid block range; validate chain ID 1 via `ethclient.ChainID` (D-02 probe).

### `internal/indexer/decoder.go` (service, transform)

**Source:** RESEARCH lines 82-88 (verified topic0 table) and lines 134-149 (decode sketch). No in-repo analog.

```go
// RESEARCH:135-141
q := ethereum.FilterQuery{
    FromBlock: new(big.Int).SetUint64(from),
    ToBlock:   new(big.Int).SetUint64(to),
    Addresses: []common.Address{tokenAddress},
    Topics:    [][]common.Hash{eventTopics},
}
```

Decode rules (RESEARCH line 144, verified against live receipts):
- `len(log.Data) == 32` for USDT `Issue`/`Redeem` and USDC `Transfer`; `== 64` for USDT `DestroyedBlackFunds` (amount in `Data[32:64]`).
- Require topic count 1 for USDT events, 3 for USDC Transfer; mismatch is an **error**, not zero.
- USDC direction: zero `topics[1]` only = mint; zero `topics[2]` only = burn; both nonzero = ignore.
- USDT: zero-address `Transfer` is **never** a supply row (PITFALLS + CONTEXT specifics).
- Amount: `new(big.Int).SetBytes(...)` — exact integers end to end, no float.
- USDC paired `Mint`/`Burn` topic0s are corroboration only — never inserted.

### `internal/storage/clickhouse.go` (service, batch insert)

**Source:** `requirements.md` lines 230-261 DDL; RESEARCH line 151 clickhouse-go pattern.

```sql
-- requirements.md:230-261 (extend event_type to carry REASON per EVENT-01:
-- mint / redeem / destroyed_black_funds / burn)
CREATE TABLE stablecoin_events (
    chain String, token String, contract_address String,
    block_number UInt64, block_hash String, block_time DateTime,
    tx_hash String, log_index UInt32,
    event_type LowCardinality(String),
    from_address String, to_address String,
    raw_amount UInt256,
    ...
) ENGINE = ReplacingMergeTree(created_at)
ORDER BY (chain, token, block_number, tx_hash, log_index);
```

Insert via `conn.PrepareBatch(ctx, "INSERT INTO stablecoin_events")` → `batch.Append(...)` with `*big.Int` for `raw_amount` → `batch.Send()`. Never pass float for raw units. If `ReplacingMergeTree` is kept, all reads use `FINAL` (RESEARCH line 128).

### `cmd/indexer/main.go` (entry point, batch)

**Source:** `requirements.md:765-800` tree; D-02 probe sequence. Flow per RESEARCH lines 104-121: validate config → fail-fast provider probe (`ChainID()==1`, `eth_blockNumber`, one sample historical `eth_getLogs`) → per-token filtered getLogs → decode → header lookup for hash+timestamp → ClickHouse insert → print inspection summary. Logging: stdlib `log/slog`.

### `internal/rpc/client.go` (service, request-response)

**Source:** RESEARCH lines 134-142 sketch; thin wrapper over `go-ethereum/ethclient` (`FilterLogs`, `HeaderByNumber`, `ChainID`). RPC URL from env only; redact URL in logs (RESEARCH V2).

## Shared Patterns

### Error handling
No in-repo pattern. Convention: Go stdlib error wrapping (`fmt.Errorf("...: %w", err)`); fail-fast on config/probe errors; decode mismatches are errors, not silent zeros. Mark log rows with full provenance so later phases can re-verify.

### Exact-integer discipline
`math/big.Int` in Go, `UInt256` in ClickHouse, everywhere between. Verify `2^256-1` round trip in tests.

### Credential handling
`ETH_RPC_URL` (and ClickHouse creds) from environment only — never in source or config files (AGENTS.md:15, requirements.md:109-116).

### Testing
Go stdlib `testing` only (RESEARCH "Validation Architecture", line 199): offline table tests for decoder fixtures; live-RPC and ClickHouse integration tests opt-in via env guard. Pinned fixture tx hashes are in RESEARCH lines 92-98.

## No Analog Found

All 8 files — repository is greenfield (verified: `git ls-files` shows only `go.mod`, `requirements.md`, planning docs). Planner must build from RESEARCH.md code examples and requirements.md DDL/config conventions above, not from in-repo code.

## Metadata

**Analog search scope:** repo root, `git ls-files` — 0 Go source files tracked.
**Files scanned:** 4 tracked non-planning files.
**Pattern extraction date:** 2026-10-08
