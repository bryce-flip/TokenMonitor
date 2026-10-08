<!-- GSD:project-start source:PROJECT.md -->

## Project

**Ethereum Stablecoin Monitor**

A monitor for USDT and USDC issuance on Ethereum Mainnet. A Go indexer reads each contract's supply-changing events, records mint and burn activity in ClickHouse, derives supply and issuance metrics, compares supply with each contract's `totalSupply()`, and presents the results in Grafana. The initial user is an operator or analyst who needs to inspect current supply, recent issuance, large events, and data health.

**Core Value:** Give a trustworthy, reproducible view of USDT and USDC supply and mint/burn activity on Ethereum Mainnet.

### Constraints

- **Chain and tokens:** Ethereum Mainnet USDT and USDC only in the MVP, as specified in `requirements.md`.
- **Technology:** Go indexer, ClickHouse storage, Grafana presentation, and Docker Compose deployment, as specified in `requirements.md`.
- **RPC:** Use an externally configured Ethereum JSON-RPC URL; never embed provider credentials or URLs in source.
- **Data correctness:** Event ingestion must tolerate retries, restarts, duplicate logs, and short chain reorganizations without corrupting reported metrics.
- **Configuration:** Keep token contract metadata, confirmation depth, and large-event thresholds configurable.

<!-- GSD:project-end -->

<!-- GSD:stack-start source:codebase/STACK.md -->

## Technology Stack

## Languages

- Go 1.27.1 - Declared by `go.mod`; no `.go` implementation files are present.
- Not detected. The YAML and SQL in `requirements.md` are examples, not project files.

## Runtime

- Go 1.27.1 is the target declared in `go.mod`; no executable exists yet.
- Go modules - `go.mod` declares module `TOkenMonitor` and no dependencies.
- Lockfile: missing (`go.sum` is not present).

## Frameworks

- Not detected in `go.mod`; `requirements.md` specifies a Go indexer but no framework.
- Not detected; there are no test files or test dependencies in `go.mod`.
- Go toolchain implied by `go.mod`; no `Makefile`, `Dockerfile`, or build script exists.
- Docker Compose is specified for the MVP in `requirements.md`, with no Compose file present.

## Key Dependencies

- None declared in `go.mod`. Ethereum RPC client and ClickHouse client choices remain open.
- Planned in `requirements.md`: external Ethereum Mainnet JSON-RPC provider, ClickHouse, and Grafana. None is configured or implemented.
- Planned in `requirements.md`: Docker Compose services for the indexer, ClickHouse, and Grafana. No deployment manifests exist.

## Configuration

- No runtime environment configuration exists. `requirements.md` proposes `ETH_RPC_URL` and optional `ETH_WS_URL`; no `.env` file is present.
- `requirements.md` proposes token contracts, confirmation depth, and alert thresholds in YAML; no configuration file exists.
- `go.mod` is the only toolchain configuration. No build or lint configuration is present.

## Platform Requirements

- A Go toolchain compatible with the `go 1.27.1` directive in `go.mod` is required to build future code.
- Per `requirements.md`, the intended MVP will need an Ethereum RPC endpoint and Docker Compose for ClickHouse and Grafana; these are not current executable prerequisites.
- Not implemented. `requirements.md` proposes a Docker Compose deployment with an external Ethereum RPC provider.

<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->

## Conventions

## Naming Patterns

- No source-file naming pattern is established. The repository contains `go.mod` and `requirements.md`; the proposed `.go` layout in `requirements.md` is a specification, not implemented convention.
- Not detected; there are no Go functions in the repository (`go.mod`).
- Not detected; there are no Go variables in the repository (`go.mod`).
- Not detected; there are no Go type declarations in the repository (`go.mod`).

## Code Style

- Not detected; no source or formatter configuration exists alongside `go.mod`.
- Follow standard `gofmt` formatting when adding the first Go files; no project-specific settings are defined in `requirements.md`.
- Not detected; there is no linter configuration or lint command in `go.mod` or `requirements.md`.

## Import Organization

- Not detected. The module path is `TOkenMonitor` in `go.mod`.

## Error Handling

- No implementation pattern exists (`go.mod`). `requirements.md` requires RPC error retry, rate-limit handling, and clear logs for RPC errors, stalled indexing, and supply mismatches.

## Logging

- No logging implementation exists. `requirements.md` calls for explicit RPC, indexer-stall, and supply-mismatch logs; it does not select a logging library.

## Comments

- Not established in source (`go.mod`). `requirements.md` documents domain behavior, but contains no implemented comment conventions.
- Not applicable to the Go module in `go.mod`.

## Function Design

## Module Design

<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->

## Architecture

## System Overview

```text

```

## Component Responsibilities

| Component | Responsibility | File |
|-----------|----------------|------|
| Go module | Declares module path and Go language version | `go.mod` |
| Requirements | Defines MVP behavior and a proposed module layout | `requirements.md` |
| Application components | Not detected | Not applicable |

## Pattern Overview

- `go.mod` contains no dependencies or executable package.
- `requirements.md` proposes a block-range indexer, ClickHouse event/checkpoint/snapshot tables, and Grafana views.
- No implemented import graph or runtime boundaries can be inferred.

## Layers

- Purpose: Identify the Go module and language version.
- Location: `go.mod`
- Contains: `module TOkenMonitor` and `go 1.27.1`.
- Depends on: No declared external modules.
- Used by: No source packages exist.
- Purpose: Index Ethereum stablecoin mint/burn events and visualize supply and issuance.
- Location: `requirements.md`, sections 9, 18, 20-22.
- Contains: Design only; no implementation files.
- Depends on: Proposed external Ethereum RPC, ClickHouse, and Grafana.
- Used by: Not applicable in current code.

## Data Flow

### Primary Request Path

### Supply Reconciliation

## Key Abstractions

- Purpose: Process confirmed Ethereum blocks and resume from a checkpoint.
- Examples: Proposed `internal/indexer/indexer.go`, `internal/indexer/checkpoint.go` in `requirements.md`, section 22.
- Pattern: Specification only; no concrete interface or API exists.
- Purpose: Keep USDT and USDC contract metadata outside business logic.
- Examples: Proposed `config/tokens.yaml` in `requirements.md`, sections 3 and 22.
- Pattern: Specification only; no parser or config file exists.

## Entry Points

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

### Assuming Storage Semantics Are Enforced

## Error Handling

- `requirements.md`, section 24 requires RPC retry and rate-limit handling, but defines no code pattern.
- No logging or error propagation conventions are established in source files.

## Cross-Cutting Concerns

<!-- GSD:architecture-end -->

<!-- GSD:skills-start source:skills/ -->

## Project Skills

No project skills found. Add skills to any of: `.claude/skills/`, `.agents/skills/`, `.cursor/skills/`, `.github/skills/`, or `.codex/skills/` with a `SKILL.md` index file.
<!-- GSD:skills-end -->

<!-- GSD:workflow-start source:GSD defaults -->

## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `$gsd-fast` for a trivial task inline, with no subagents and no PLAN.md
- `$gsd-quick` for small fixes, doc updates, and ad-hoc tasks
- `$gsd-debug` for investigation and bug fixing
- `$gsd-execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->

<!-- GSD:profile-start -->

## Developer Profile

> Profile not yet configured. Run `$gsd-profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
