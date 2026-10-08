# Stack Research

**Domain:** Ethereum Mainnet USDT/USDC issuance monitoring
**Researched:** 2026-10-08
**Confidence:** MEDIUM (official documentation and release pages cross-checked; `classify-confidence --provider websearch --verified` returned MEDIUM)

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| Go | 1.27.1 | Indexer and reconciliation worker | Already declared in `go.mod`; current released patch on the research date. `context`, `math/big`, `encoding/json`, `log/slog`, and `net/http` cover most application needs. [Go release history](https://go.dev/doc/devel/release) |
| `github.com/ethereum/go-ethereum` | v1.17.7 | Ethereum JSON-RPC client and log types | `ethclient` exposes `ChainID`, `BlockNumber`, `HeaderByNumber`, `FilterLogs`, and `CallContract`; use one HTTP RPC connection with bounded polling. No node process or WebSocket is required for the MVP. [API](https://pkg.go.dev/github.com/ethereum/go-ethereum/ethclient), [releases](https://github.com/ethereum/go-ethereum/releases) |
| ClickHouse Server | 26.8 LTS line | Raw events, checkpoints, supply snapshots, analytics | The required analytical store; the 26.8 line is an LTS release. Pin a tested 26.8 patch image when implementing Compose. [Release](https://presentations.clickhouse.com/2026-release-26.8/), [supported versions](https://github.com/ClickHouse/ClickHouse/security) |
| Grafana OSS | 13.2.2 | Operator dashboards | Current verified release, with file-provisioned data source and dashboards. [Release](https://github.com/grafana/grafana/releases/tag/v13.2.2), [provisioning](https://grafana.com/docs/grafana/latest/administration/provisioning/) |
| Docker Compose | Current V2 CLI | Run indexer, ClickHouse, and Grafana | Required deployment shape; `healthcheck` plus `depends_on: condition: service_healthy` handles ClickHouse readiness. No separate orchestrator for MVP. [Startup ordering](https://docs.docker.com/compose/how-tos/startup-order/) |

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/ClickHouse/clickhouse-go/v2` | v2.48.0 | Native ClickHouse client | Use `Open`, `PrepareBatch`, `Append`, and `Send` for event/snapshot writes and queries; batch bounded ranges, then advance the checkpoint only after successful writes. [Go integration](https://clickhouse.com/integrations/go), [releases](https://github.com/ClickHouse/clickhouse-go/releases) |
| `grafana-clickhouse-datasource` | 4.22.1 | Grafana to ClickHouse queries | Install as a pinned Grafana plugin; provision a read-only ClickHouse account for it. Plugin page lists Grafana >=11.6.0, so 13.2.2 satisfies that stated minimum. [Plugin](https://grafana.com/grafana/plugins/grafana-clickhouse-datasource/installation/), [configuration](https://grafana.com/docs/plugins/grafana-clickhouse-datasource/latest/configure/) |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| `go test`, `go vet`, `gofmt` | Validate indexer code | Standard Go toolchain; test event classification, checkpoint recovery, and decimal math. |
| `docker compose` | Local integration smoke test | Use named ClickHouse/Grafana volumes and a ClickHouse healthcheck; keep the external RPC URL configurable. |
| `log/slog` | Structured indexer logs | Standard library handles RPC errors, lag, and supply mismatch messages without a logging dependency. [Go docs](https://pkg.go.dev/log/slog) |

## Installation

```bash
go get github.com/ethereum/go-ethereum@v1.17.7
go get github.com/ClickHouse/clickhouse-go/v2@v2.48.0
go mod tidy
```

Pin the server images and plugin in Compose, for example `grafana/grafana:13.2.2` and `GF_PLUGINS_PREINSTALL=grafana-clickhouse-datasource@4.22.1`. Select and verify a concrete ClickHouse 26.8 LTS patch image during implementation; the official package feed listed `26.8.20.9` on 2026-10-07, but its container tag was not independently verified. [Grafana Docker plugin configuration](https://grafana.com/docs/grafana/latest/setup-grafana/installation/docker/), [ClickHouse package feed](https://packages.clickhouse.com/).

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| `ethclient` HTTP polling | WebSocket subscriptions | Add only if a latency requirement cannot be met with confirmed-head polling; recovery still needs range-based `eth_getLogs`. |
| Native `clickhouse-go/v2` client | `database/sql` ClickHouse driver | Use if the application later standardizes on `database/sql`; native batches are the direct documented path for this writer. |
| Grafana ClickHouse plugin | Custom Go API and web UI | Add only for workflows Grafana cannot express; direct SQL dashboards meet the MVP requirement. |
| Standard-library JSON config plus environment-supplied secrets | YAML config parser | The spec shows YAML as an example, not a hard format requirement. Add YAML only if operators need that exact format; JSON avoids another dependency. |
| SQL views/queries over raw events | Materialized daily rollup | Add a rollup after measured dashboard query cost warrants it; raw events must remain the rebuildable source. |

## What Not to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| Floating-point token amounts | Six-decimal amounts and `uint256` event values need exact arithmetic for reconciliation. | `math/big` for raw units; ClickHouse exact Decimal columns with a verified conversion path. |
| Assuming `ReplacingMergeTree` is an immediate uniqueness constraint | Replacement happens in background merges; duplicates can appear in ordinary reads. | Stable event key plus `SELECT ... FINAL` or `argMax` for correctness-sensitive reads; handle orphaned blocks separately. [ClickHouse guidance](https://clickhouse.com/resources/engineering/clickhouse-optimize-table-final) |
| A Prometheus server solely for the MVP | The required Compose stack already includes ClickHouse and Grafana; operational state can be stored/queryable there and exposed through a small health endpoint. | Add Prometheus only when scrape-based alerting or shared infrastructure requires it. |
| Hosted Ethereum node or generated ABI binding framework | Both add operations/code without serving the two-token Transfer filter and one `totalSupply()` call. | External HTTP RPC, `ethclient`, fixed event signature, and minimal ABI encoding. |

## Stack Patterns

1. Validate `ChainID == 1` at startup. Query only the configured USDT/USDC contract addresses and Transfer topic through `FilterLogs`; process a bounded confirmed range. Keep the RPC endpoint and confirmation depth configurable. [ethclient API](https://pkg.go.dev/github.com/ethereum/go-ethereum/ethclient)
2. Write raw events in batches, then record the durable checkpoint. Replayed ranges must remain logically idempotent by `(chain, block_hash, transaction_hash, log_index)` or another proven canonical identity, and downstream queries must use a deduplicated view. On a reorg, compare stored block hashes with the canonical chain and exclude or supersede orphaned events; key-based deduplication alone cannot do this. ClickHouse's merge behavior is not a transaction across event and checkpoint tables. [Go integration](https://clickhouse.com/integrations/go), [ReplacingMergeTree](https://clickhouse.com/resources/engineering/clickhouse-optimize-table-final)
3. Call `totalSupply()` at the same confirmed block selected for reconciliation. An absolute calculated supply needs either complete historical mint/burn coverage or a verified supply anchor at the backfill start; that choice is a prerequisite for claiming a zero difference. This follows from the project's supply equation and is an implementation inference, not a library feature claim.
4. Provision dashboards and the ClickHouse data source from files. Give Grafana a read-only database user because the plugin executes submitted SQL; store its password and RPC credentials outside source control. [Grafana configuration](https://grafana.com/docs/plugins/grafana-clickhouse-datasource/latest/configure/), [Compose secrets](https://docs.docker.com/compose/how-tos/use-secrets/)

## Version Compatibility and Open Checks

| Pair | Status | Check Before Implementation |
|------|--------|-----------------------------|
| Go 1.27.1 + go-ethereum v1.17.7 | Compatible by published requirements: go-ethereum release binaries use Go 1.27, and the project already targets 1.27.1. | Run `go mod tidy` and build on the actual development platform. |
| Grafana 13.2.2 + ClickHouse plugin 4.22.1 | Plugin states Grafana >=11.6.0; version minimum is satisfied. | Smoke-test plugin loading and provisioned data source in Compose. |
| ClickHouse 26.8 LTS + clickhouse-go v2.48.0 | Both are current supported lines; no explicit compatibility matrix was found. | Execute native connection, batch insert, exact-amount round trip, and query smoke tests against the pinned server image. |
| Existing `go.mod` module `TOkenMonitor` | No Go source imports it yet. | Choose a stable canonical module path before internal package imports; do not churn it after code lands. |

## Sources and Confidence

Research used the GSD research-plan seam. Its documentation questions selected Context7, but neither Context7 MCP nor the `ctx7` CLI was available; the fallback was current official project documentation and release pages. The GSD confidence classifier returned **MEDIUM** for verified web-search findings. All version and API claims above therefore carry MEDIUM confidence, with explicit smoke-test checks for unverified binary compatibility.

- [Go release history](https://go.dev/doc/devel/release), [go-ethereum API](https://pkg.go.dev/github.com/ethereum/go-ethereum/ethclient), [go-ethereum releases](https://github.com/ethereum/go-ethereum/releases)
- [ClickHouse Go integration](https://clickhouse.com/integrations/go), [driver releases](https://github.com/ClickHouse/clickhouse-go/releases), [ClickHouse 26.8 LTS](https://presentations.clickhouse.com/2026-release-26.8/), [ReplacingMergeTree guidance](https://clickhouse.com/resources/engineering/clickhouse-optimize-table-final)
- [Grafana release](https://github.com/grafana/grafana/releases/tag/v13.2.2), [ClickHouse plugin](https://grafana.com/grafana/plugins/grafana-clickhouse-datasource/installation/), [provisioning](https://grafana.com/docs/grafana/latest/administration/provisioning/)
- [Compose startup order](https://docs.docker.com/compose/how-tos/startup-order/), [Compose secrets](https://docs.docker.com/compose/how-tos/use-secrets/)

---
*Stack research for: Ethereum Stablecoin Monitor*
*Researched: 2026-10-08*
