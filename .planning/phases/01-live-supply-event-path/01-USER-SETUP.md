# Phase 1: User Setup Required

**Generated:** 2026-10-08
**Phase:** 01-live-supply-event-path
**Status:** Incomplete

Complete these items for the bounded live ingest and the live fixture proof (plan 01-02) to function. Claude automated everything possible; these items require human access to an external provider account.

## Environment Variables

| Status | Variable | Source | Add to |
|--------|----------|--------|--------|
| [ ] | `ETH_RPC_URL` | Free-tier provider of your choice (Alchemy, Infura, or QuickNode) → dashboard → API keys → Ethereum Mainnet HTTP endpoint | your shell environment (e.g. `~/.zshrc` or the invocation prefix) |

Notes:
- The URL must serve historical `eth_getLogs` on Ethereum Mainnet (D-01/D-02). The indexer probes this fail-fast before ingesting; public endpoints that refuse archive log queries will be rejected with a clear error.
- Never write the URL into source, config files, or logs — the indexer reads it from the environment and redacts it to `scheme://host` in all error output.

## Account Setup

- [ ] **Create an Ethereum RPC provider account** (if needed)
  - Alchemy: https://www.alchemy.com/ ; Infura: https://www.infura.io/ ; QuickNode: https://www.quicknode.com/
  - Skip if: You already have a provider API key with Mainnet access.

## Verification

After completing setup, verify with (from the repo root, with the ClickHouse container running):

```bash
export ETH_RPC_URL="https://<your-provider-endpoint>"
docker start tm-clickhouse 2>/dev/null || docker run -d --name tm-clickhouse -p 127.0.0.1:9000:9000 -e CLICKHOUSE_SKIP_USER_SETUP=1 clickhouse/clickhouse-server:26.8
go run ./cmd/indexer -from 23482390 -to 23482394
```

Expected results:
- `provider probe passed` in the stderr log, then fetched/decoded rows for a range containing the USDT Issue fixture block (0x167ab1a), and a tabular inspection of `stablecoin_events` on stdout.
- A wrong-network or history-limited URL aborts before any ingest with an error naming the redacted endpoint.

---

**Once all items complete:** Mark status as "Complete" at top of file.
