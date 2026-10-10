---
phase: "2"
slug: "reliable-canonical-indexing"
status: verified
threats_open: 0
asvs_level: 1
created: "2026-10-10"
---

# Phase 2 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| RPC provider → indexer | Untrusted head/tags/headers/logs (`finalized`/`safe` tag responses new this phase) | Untrusted structured data |
| Operator environment → indexer | `ETH_RPC_URL`, `CLICKHOUSE_URL`, `DEVNET_RPC_URL`, tokens.json (head_policy/window/poll/start_block) | Credentials (env only), config |
| Indexer → ClickHouse | Checkpoint writes, event inserts, destructive rewind deletes (operator-gated) | Canonical state |
| Devnet (Kurtosis) → tests | Untrusted local dial target, public ethereum-package fixture key | Test-only chain data |
| Go module proxy → build | go.mod/go.sum (unchanged this phase apart from tidy-indirects) | Build inputs |

---

## Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation | Status |
|-----------|----------|-----------|----------|-------------|------------|--------|
| T-02-01 | Tampering | rpc EligibleHead, sync.go | high | mitigate | Fabricated head/headers rejected: window-end hash asserted against fetched header (fail-loud), checkpoint verified per window; probe gates wrong-chain. Verified: TestRunSync* suite, devnet restart test, live UAT run. | closed |
| T-02-02 | Tampering/Spoofing | rewind.go, two-chain harness | high | mitigate | Catastrophic-consensus reorg handled by loud halt (never auto-rewind — structural prohibition verified: zero Rewind refs in sync.go); operator verifies before destructive action. Verified: mismatch halt live (block + both hashes + instruction, exit 1); reorg suite 18/18. | closed |
| T-02-03 | Information Disclosure | rpc error returns | high | mitigate | AR-01 residual CLOSED this phase: scrubbedError renders credential-free text through all %w-wrapped url.Error chains while preserving errors.Is(ctx.Canceled). Verified: TestSupportsHeadTag*/redaction tests; verifier grep clean. | closed |
| T-02-04 | Tampering | storage checkpoint queries | medium | mitigate | Bound parameters only in Read/Write/DeleteCheckpoint. Verified: verifier artifact/link audit; DeleteCheckpointAbove uses bound params (CR-01 fix). | closed |
| T-02-05 | Tampering | rewind subcommand, DeleteEventsFrom | high | mitigate | Destructive deletes operator-gated (explicit subcommand, plan print + --yes + stdin confirmation), strictly above the verified point. Verified: live rewind walk (report table, 29→0 rows, re-anchor, resume); structural delete-predicate audit. | closed |
| T-02-06 | Spoofing/Misuse | DEVNET_RPC_URL, devnet test key | low | accept | Devnet surface is test-only: untrusted local dial target, public ethereum-package fixture key; never used in production paths. See AR-03. | closed |
| T-02-07 | Denial of Service | FetchLogsResilient classification | medium | mitigate | Cap/throttle misroute fixed: -32005 message discrimination (status-token regex after WR-01), cap→halve, throttle→backoff, floor fails closed. Verified: discrimination + floor tests; live dense-window run. | closed |
| T-02-SC | Tampering | go.mod / go.sum | high | mitigate | No new packages; tidy-indirects only (x/crypto, fsnotify via pinned go-ethereum); go mod verify clean. Verified by executor, verifier. | closed |

*Status: open · closed — only open threats at or above workflow.security_block_on (high) count toward threats_open*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-03 | T-02-06 | Devnet dial target and keys are test-only fixtures (public ethereum-package defaults); no production surface; env-gated suite never runs in CI without DEVNET_RPC_URL. | bryce (via Claude delegation, /gsd-secure-phase equivalent) | 2026-10-10 |

*Accepted risks do not resurface in future audit runs.*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-10-10 | 8 | 8 | 0 | gsd orchestrator (L1 grep-depth + live operator-loop evidence, ASVS 1) |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-10-10
