---
last_mapped_commit: 1ab6dc53859f4533eb1563dc0b95eafe9e4e172e
last_mapped_at: 2026-10-10
---
# External Integrations

**Analysis Date:** 2026-10-08

No external integration is implemented. `go.mod` contains no dependencies, and `requirements.md` is the source of the planned integrations below.

## APIs & External Services

**Ethereum Mainnet (planned):**
- External Ethereum JSON-RPC provider - `requirements.md` specifies block height, filtered transfer logs, block timestamps, and ERC-20 `totalSupply()` reads.
  - SDK/Client: Not selected in `go.mod`; `requirements.md` calls for `eth_blockNumber`, `eth_getLogs`, `eth_getBlockByNumber`, and `eth_call`.
  - Auth: Provider URL through proposed `ETH_RPC_URL`; optional WebSocket URL through proposed `ETH_WS_URL`. No values or client code exist.
- USDT and USDC contracts - Addresses and six-decimal token metadata are specified in `requirements.md`; no token config file exists.

**Notifications (future only):**
- Telegram, Discord, email, and outgoing webhooks are mentioned as later options in `requirements.md`; no implementation or credentials exist.

## Data Storage

**Databases:**
- ClickHouse (planned) - `requirements.md` proposes `stablecoin_events`, `indexer_checkpoint`, `stablecoin_supply_snapshot`, and optional `stablecoin_daily_metrics` tables.
  - Connection: No connection variable or file is defined in `go.mod` or `requirements.md`.
  - Client: Not selected in `go.mod`; no migrations or database code exist.

**File Storage:**
- Not detected; `requirements.md` proposes database storage, not object or file storage.

**Caching:**
- Not detected in `go.mod` or `requirements.md`.

## Authentication & Identity

**Auth Provider:**
- Not applicable to the specified MVP in `requirements.md`; no application authentication is implemented.
  - Implementation: Not detected. Ethereum RPC provider authentication, if needed, belongs in its configured URL or provider setup; neither is present.

## Monitoring & Observability

**Error Tracking:**
- Not detected. `requirements.md` requires clear logs for RPC errors, stopped indexing, and supply mismatch.

**Logs:**
- No logging code exists. `requirements.md` specifies operational logging but no framework.
- Prometheus metrics are optional in `requirements.md`; no exporter or metrics dependency exists in `go.mod`.

## CI/CD & Deployment

**Hosting:**
- Not configured. `requirements.md` proposes Docker Compose with indexer, ClickHouse, and Grafana services.

**CI Pipeline:**
- Not detected; no workflow files or CI configuration exist.

## Environment Configuration

**Required env vars:**
- None currently consumed. `requirements.md` proposes `ETH_RPC_URL` for the MVP and `ETH_WS_URL` for optional WebSocket RPC.
- ClickHouse and Grafana connection settings are not specified in `requirements.md` and have no config files.

**Secrets location:**
- Not detected. No `.env` file or secret store configuration is present; `requirements.md` says not to hard-code RPC URLs.

## Webhooks & Callbacks

**Incoming:**
- None specified for the MVP in `requirements.md`; no server code exists.

**Outgoing:**
- None implemented. `requirements.md` lists webhook notification as a possible later extension.

---

*Integration audit: 2026-10-08*
