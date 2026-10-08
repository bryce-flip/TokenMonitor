---
phase: "1"
slug: "live-supply-event-path"
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate)
threats_open: 0
asvs_level: 1
created: "2026-10-08"
---

# Phase 1 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| Operator environment → indexer | `ETH_RPC_URL`, `CLICKHOUSE_URL`, `config/tokens.json` cross into the process | Credentials (env only), public token metadata |
| RPC provider → indexer | Untrusted JSON-RPC responses: logs, headers, chain ID | Untrusted structured data |
| Indexer → ClickHouse | Local native-protocol connection, Phase 1 default account | Exact supply-event rows |
| Go module proxy → build | Pinned dependency downloads (go-ethereum, clickhouse-go) | Build inputs |
| Operator provider → live tests | Real JSON-RPC responses cross into TestLive assertions | Untrusted chain data |
| Repository docs → operator shell | README commands are copy-pasted and executed by a human | Runbook instructions |

---

## Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation | Status |
|-----------|----------|-----------|----------|-------------|------------|--------|
| T-01-01 | Tampering | internal/indexer/decoder.go, internal/rpc/client.go | high | mitigate | D-02 fail-fast probe (chain ID 1 + sample historical getLogs) before ingest; decoder rejects address mismatch, topic-count and data-length errors; full provenance stored for Phase 2 canonicality. Verified: TestProbeRejectsWrongChain, TestProbeRejectsProviderWithoutHistoricalLogs, TestDecodeMalformedAndMismatchAreErrors; verifier-traced end to end. | closed |
| T-01-02 | Information Disclosure | internal/rpc/client.go, cmd/indexer/main.go | medium | accept | Direct error/log output renders URLs through config.HostOnly (scheme://host); URL read from env per D-01, never persisted. Residual accepted: `%w`-wrapped net/http url.Error chains can still carry the full key-bearing URL in runtime error logs (tracked-file grep clean; only test output is scrubbed — 01-02-SUMMARY Deviation 4). See Accepted Risks AR-01. | closed |
| T-01-03 | Tampering | ClickHouse container | medium | mitigate | Documented `docker run` binds 127.0.0.1:9000 only (no 0.0.0.0); Phase 1 local default account; scoped read/write split arrives with Phase 4 (OPER-01). Verified: running tm-clickhouse container loopback-bound. | closed |
| T-01-04 | Information Disclosure | README.md, internal/indexer/live_test.go | medium | mitigate | README and tests carry only the ETH_RPC_URL placeholder name, never a provider URL or key value; verify gate's automated grep fails on any alchemy/infura/quicknode URL in README.md. Verified: repo-wide git grep across all commits clean (verifier + code reviewer independently). | closed |
| T-01-05 | Tampering | internal/indexer/live_test.go | low | accept | Live assertions depend on the operator's provider serving accurate historical logs; a hostile full provider is out of scope for Phase 1 — block-hash provenance stored per row enables Phase 2 canonicality/reorg work to detect it. Accepted at plan time. | closed |
| T-01-SC | Tampering | go.mod / go.sum | high | mitigate | Supply chain: only research-pinned versions (go-ethereum v1.17.7, clickhouse-go/v2 v2.48.0 — no @latest drift); go.sum committed; `go mod verify` all-verified. Verified by executor and verifier. | closed |

*Status: open · closed · open — below high threshold (non-blocking)*
*Severity: critical > high > medium > low — only open threats at or above workflow.security_block_on count toward threats_open*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-01 | T-01-02 | Single-operator local MVP: runtime error logs stay on the operator's machine; tracked files verified credential-free; proper redaction of `%w`-chained url.Error text lands with Phase 2 RPC error handling. | bryce (via /gsd-secure-phase gate) | 2026-10-08 |
| AR-02 | T-01-05 | Hostile-provider accuracy out of scope for Phase 1; per-row block-hash provenance enables Phase 2 detection. | bryce (plan-time disposition) | 2026-10-08 |

*Accepted risks do not resurface in future audit runs.*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-10-08 | 6 | 6 | 0 | gsd orchestrator (L1 grep-depth, ASVS 1) |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-10-08
