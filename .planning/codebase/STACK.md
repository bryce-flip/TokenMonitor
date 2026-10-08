---
last_mapped_commit: 56e39614d8217817733b2b97293be83160ebbf2e
last_mapped_at: 2026-10-08
---
# Technology Stack

**Analysis Date:** 2026-10-08

## Languages

**Primary:**
- Go 1.27.1 - Declared by `go.mod`; no `.go` implementation files are present.

**Secondary:**
- Not detected. The YAML and SQL in `requirements.md` are examples, not project files.

## Runtime

**Environment:**
- Go 1.27.1 is the target declared in `go.mod`; no executable exists yet.

**Package Manager:**
- Go modules - `go.mod` declares module `TOkenMonitor` and no dependencies.
- Lockfile: missing (`go.sum` is not present).

## Frameworks

**Core:**
- Not detected in `go.mod`; `requirements.md` specifies a Go indexer but no framework.

**Testing:**
- Not detected; there are no test files or test dependencies in `go.mod`.

**Build/Dev:**
- Go toolchain implied by `go.mod`; no `Makefile`, `Dockerfile`, or build script exists.
- Docker Compose is specified for the MVP in `requirements.md`, with no Compose file present.

## Key Dependencies

**Critical:**
- None declared in `go.mod`. Ethereum RPC client and ClickHouse client choices remain open.

**Infrastructure:**
- Planned in `requirements.md`: external Ethereum Mainnet JSON-RPC provider, ClickHouse, and Grafana. None is configured or implemented.
- Planned in `requirements.md`: Docker Compose services for the indexer, ClickHouse, and Grafana. No deployment manifests exist.

## Configuration

**Environment:**
- No runtime environment configuration exists. `requirements.md` proposes `ETH_RPC_URL` and optional `ETH_WS_URL`; no `.env` file is present.
- `requirements.md` proposes token contracts, confirmation depth, and alert thresholds in YAML; no configuration file exists.

**Build:**
- `go.mod` is the only toolchain configuration. No build or lint configuration is present.

## Platform Requirements

**Development:**
- A Go toolchain compatible with the `go 1.27.1` directive in `go.mod` is required to build future code.
- Per `requirements.md`, the intended MVP will need an Ethereum RPC endpoint and Docker Compose for ClickHouse and Grafana; these are not current executable prerequisites.

**Production:**
- Not implemented. `requirements.md` proposes a Docker Compose deployment with an external Ethereum RPC provider.

---

*Stack analysis: 2026-10-08*
