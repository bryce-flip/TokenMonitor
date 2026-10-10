---
last_mapped_commit: 1ab6dc53859f4533eb1563dc0b95eafe9e4e172e
last_mapped_at: 2026-10-10
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
- Go modules - `go.mod` declares module `TOkenMonitor`.
- Lockfile: `go.sum` committed; `go mod verify` passes.

## Frameworks

**Core:**
- Not detected in `go.mod`; `requirements.md` specifies a Go indexer but no framework.

**Testing:**
- Go standard `testing` package; offline unit/table tests plus a ClickHouse integration suite gated on `CLICKHOUSE_URL`.

**Build/Dev:**
- Go toolchain implied by `go.mod`; no `Makefile`, `Dockerfile`, or build script exists.
- Docker Compose is specified for the MVP in `requirements.md`, with no Compose file present.

## Key Dependencies

**Critical:**
- `github.com/ethereum/go-ethereum v1.17.7` — ethclient (FilterLogs, HeaderByNumber, ChainID), common/types.
- `github.com/ClickHouse/clickhouse-go/v2 v2.48.0` — native-protocol driver (ParseDSN, PrepareBatch, `*big.Int` for UInt256).

**Infrastructure:**
- Planned in `requirements.md`: external Ethereum Mainnet JSON-RPC provider, ClickHouse, and Grafana. None is configured or implemented.
- Planned in `requirements.md`: Docker Compose services for the indexer, ClickHouse, and Grafana. No deployment manifests exist.

## Configuration

**Environment:**
- `ETH_RPC_URL` (required at runtime; errors name it when unset), optional `ETH_WS_URL` still unimplemented (spec proposal).
- `CLICKHOUSE_URL` optional; defaults to `clickhouse://default@127.0.0.1:9000/default` (local container, loopback-bound).
- Token contracts, confirmation depth live in `config/tokens.json` (JSON, not the spec's proposed YAML).

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
