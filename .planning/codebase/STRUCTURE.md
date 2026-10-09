---
last_mapped_commit: b219d36
last_mapped_at: 2026-10-09
---
# Codebase Structure

**Analysis Date:** 2026-10-09

## Directory Layout

```text
TokenMonitor/
├── go.mod, go.sum         # Go module + pinned dependency lockfile
├── requirements.md        # MVP specification
├── cmd/
│   └── indexer/           # continuous ingest CLI (`run` subcommand, default)
├── config/
│   └── tokens.json        # chain_id, head policy, window/poll sizing, USDT/USDC contracts
├── internal/
│   ├── config/            # JSON load/validate with defaults, env lookups, URL redaction
│   ├── indexer/           # supply-event decoder + RunSync continuous loop (+ tests)
│   ├── rpc/               # ethclient wrapper: Probe/FetchLogs/Header/EligibleHead (+ tests)
│   └── storage/           # ClickHouse native store: events + checkpoint (+ integration tests)
├── migrations/
│   └── clickhouse.sql     # stablecoin_events + indexer_checkpoint DDL (ReplacingMergeTree)
└── .planning/
    └── codebase/          # Codebase analysis documents
```

`deployments/` (Docker Compose, Grafana) remains a Phase 4 proposal only.

## Directory Purposes

**Repository root:**
- Purpose: Module declaration, lockfile, and project requirements.
- Contains: `go.mod`, `go.sum`, `requirements.md`.
- Key files: `go.mod`, `go.sum`, `requirements.md`.

**`.planning/codebase/`:**
- Purpose: Holds generated codebase mapping documents.
- Contains: `ARCHITECTURE.md`, `STRUCTURE.md`, and other focus-area maps.
- Key files: `.planning/codebase/ARCHITECTURE.md`, `.planning/codebase/STRUCTURE.md`.

## Key File Locations

**Entry Points:**
- `cmd/indexer/main.go` — continuous sync command (`run` subcommand, default; `-config`, optional `-from` seed).

**Configuration:**
- `config/tokens.json`: chain_id, head_policy, confirmation_blocks, window_blocks, window_floor, poll_seconds, optional start_block, token contracts/decimals.
- `go.mod`/`go.sum`: module `TOkenMonitor`, pinned go-ethereum and clickhouse-go.
- Runtime env: `ETH_RPC_URL` (required), `CLICKHOUSE_URL` (defaults to `clickhouse://default@127.0.0.1:9000/default`).

**Core Logic:**
- `internal/indexer/decoder.go`: pinned topic0 constants, `SupplyTopics`, `DecodeSupplyEvent`.
- `internal/indexer/sync.go`: `RunSync` continuous window loop over the `ChainReader`/`EventStore` seams; `CheckpointMismatchError`.
- `internal/rpc/client.go`: `Probe` (D-02 fail-fast), `FetchLogs`, `Header`, `EligibleHead` (finalized tag / confirmed latest-minus-depth).
- `internal/storage/clickhouse.go`: `EnsureSchema` (statement-splitting), `InsertEvents`, `Inspect`, `WriteCheckpoint`, `ReadCheckpoint`.

**Testing:**
- `internal/indexer/decoder_test.go`, `internal/indexer/sync_test.go` (external package, canned JSON-RPC + fake store), `internal/rpc/client_test.go` (offline); `internal/storage/clickhouse_test.go` (integration, gated on `CLICKHOUSE_URL`).

## Naming Conventions

**Files:**
- No implementation naming convention is established by source files. Follow Go's `*_test.go` convention when tests are added.
- `requirements.md`, section 22 proposes lowercase Go filenames such as `indexer.go` and `checkpoint.go`.

**Directories:**
- No implemented package directory convention exists.
- `requirements.md`, section 22 proposes `cmd/indexer/` for the executable and domain-focused packages under `internal/`.

## Where to Add New Code

**New Feature:**
- Primary code: Start with the relevant proposed package in `requirements.md`, section 22, once that package is created; current repository has no implementation path.
- Tests: Co-locate Go `*_test.go` files with each new package; no existing test layout constrains this.

**New Component/Module:**
- Implementation: Use proposed `cmd/indexer/main.go` for the executable and `internal/` for application code only if the design in `requirements.md`, section 22 fits the feature.

**Utilities:**
- Shared helpers: No shared utility package exists. Put a helper in the package that uses it until multiple packages need it.

## Special Directories

**`.planning/codebase/`:**
- Purpose: Maps current repository state for planning.
- Generated: Yes, by codebase mapping.
- Committed: Repository policy not detected.

**Proposed `migrations/` and `deployments/`:**
- Purpose: SQL schema and Docker/Grafana deployment assets in `requirements.md`, section 22.
- Generated: Not applicable; directories do not exist.
- Committed: Not applicable in current repository.

---

*Structure analysis: 2026-10-08*
