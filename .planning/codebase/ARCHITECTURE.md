---
last_mapped_commit: 56e39614d8217817733b2b97293be83160ebbf2e
last_mapped_at: 2026-10-08
---
<!-- refreshed: 2026-10-08 -->

# Architecture

**Analysis Date:** 2026-10-08

## System Overview

```text
Current repository
  `go.mod`           Go module declaration
  `requirements.md`  product and proposed-system specification
  .planning/codebase/ analysis documents

No executable, source package, database migration, deployment, or dashboard exists.
```

The intended Ethereum RPC -> Go indexer -> ClickHouse -> Grafana flow is specified in `requirements.md`, sections 4-25. It is not implemented in this repository.

## Component Responsibilities

| Component | Responsibility | File |
|-----------|----------------|------|
| Go module | Declares module path and Go language version | `go.mod` |
| Requirements | Defines MVP behavior and a proposed module layout | `requirements.md` |
| Application components | Not detected | Not applicable |

## Pattern Overview

**Overall:** Specification-only Go project skeleton.

**Key Characteristics:**
- `go.mod` contains no dependencies or executable package.
- `requirements.md` proposes a block-range indexer, ClickHouse event/checkpoint/snapshot tables, and Grafana views.
- No implemented import graph or runtime boundaries can be inferred.

## Layers

**Implemented module metadata:**
- Purpose: Identify the Go module and language version.
- Location: `go.mod`
- Contains: `module TOkenMonitor` and `go 1.27.1`.
- Depends on: No declared external modules.
- Used by: No source packages exist.

**Proposed ingestion and presentation layers:**
- Purpose: Index Ethereum stablecoin mint/burn events and visualize supply and issuance.
- Location: `requirements.md`, sections 9, 18, 20-22.
- Contains: Design only; no implementation files.
- Depends on: Proposed external Ethereum RPC, ClickHouse, and Grafana.
- Used by: Not applicable in current code.

## Data Flow

### Primary Request Path

1. Not implemented; there is no entry point under `cmd/` or elsewhere.
2. `requirements.md`, section 9 specifies `eth_getLogs` retrieval, decoding, mint/burn filtering, storage, and checkpoint updates.
3. `requirements.md`, sections 18-20 specify ClickHouse-backed metrics and Grafana output.

### Supply Reconciliation

1. `requirements.md`, sections 12-14 specify deriving supply from mint/burn history.
2. The specification calls for `eth_call` to `totalSupply()` and snapshot comparison.
3. No call, snapshot writer, or alert implementation exists.

**State Management:** No runtime state exists. `requirements.md`, section 10 proposes a persistent `indexer_checkpoint` table.

## Key Abstractions

**Block-range indexer:**
- Purpose: Process confirmed Ethereum blocks and resume from a checkpoint.
- Examples: Proposed `internal/indexer/indexer.go`, `internal/indexer/checkpoint.go` in `requirements.md`, section 22.
- Pattern: Specification only; no concrete interface or API exists.

**Token configuration:**
- Purpose: Keep USDT and USDC contract metadata outside business logic.
- Examples: Proposed `config/tokens.yaml` in `requirements.md`, sections 3 and 22.
- Pattern: Specification only; no parser or config file exists.

## Entry Points

**Indexer:**
- Location: Proposed `cmd/indexer/main.go` in `requirements.md`, section 22; absent today.
- Triggers: Not implemented.
- Responsibilities: Proposed polling, decoding, storage, and checkpoint orchestration.

## Architectural Constraints

- **Threading:** Not determined; `go.mod` has no executable code.
- **Global state:** None detected; there are no Go source files.
- **Circular imports:** Not applicable; there are no imports.
- **Module path:** `go.mod` declares `TOkenMonitor`; use that exact spelling until a deliberate rename.
- **Data scope:** `requirements.md`, sections 2-3 constrain the proposed MVP to Ethereum Mainnet USDT and USDC.

## Anti-Patterns

### Treating Proposed Files as Existing Contracts

**What happens:** The layout in `requirements.md`, section 22 can be mistaken for implemented package boundaries.
**Why it's wrong:** No `cmd/`, `internal/`, `config/`, or `migrations/` directory exists.
**Do this instead:** Inspect actual files before importing packages or assigning ownership; use `requirements.md` only as a starting design.

### Assuming Storage Semantics Are Enforced

**What happens:** The proposed deduplication and checkpoint schemas in `requirements.md`, sections 7-10 may be treated as operational guarantees.
**Why it's wrong:** No migrations or ingestion code enforce them.
**Do this instead:** Define and test write/checkpoint behavior when implementing `internal/storage/` and `internal/indexer/`.

## Error Handling

**Strategy:** Not implemented.

**Patterns:**
- `requirements.md`, section 24 requires RPC retry and rate-limit handling, but defines no code pattern.
- No logging or error propagation conventions are established in source files.

## Cross-Cutting Concerns

**Logging:** Not detected; only observability requirements appear in `requirements.md`, section 24.
**Validation:** Not implemented; token configuration and RPC inputs are specified in `requirements.md`, sections 3-4.
**Authentication:** Not implemented; `requirements.md`, section 4 expects an externally configured RPC URL.

---

*Architecture analysis: 2026-10-08*
