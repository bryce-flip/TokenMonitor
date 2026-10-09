---
phase: "2"
slug: "reliable-canonical-indexing"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
status: draft
nyquist_compliant: false
wave_0_complete: false
created: "2026-10-09"
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) |
| **Config file** | none — per-package `_test.go`; env-gated integration suites |
| **Quick run command** | `env -u ETH_RPC_URL -u CLICKHOUSE_URL go test ./... -count=1` |
| **Full suite command** | `CLICKHOUSE_URL=clickhouse://default@127.0.0.1:9000/default go test ./... -count=1` (+ opt-in `ETH_RPC_URL` TestLive, `DEVNET_RPC_URL` devnet suite) |
| **Estimated runtime** | ~10s offline; ~60s with ClickHouse; devnet suite ~2-5 min |

---

## Sampling Rate

- **After every task commit:** `env -u ETH_RPC_URL -u CLICKHOUSE_URL go test ./... -count=1`
- **After every plan wave:** `CLICKHOUSE_URL=... go test ./... -count=1`
- **Before `/gsd-verify-work`:** full suite + devnet suite green
- **Max feedback latency:** 60 seconds (offline path)

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 02-01-01 | 01 | 1 | SYNC-01 | T-02-01 / — | N/A | unit | `go test ./internal/rpc ./internal/indexer -run 'Head\|Follows' -count=1` | ❌ W0 | ⬜ pending |
| 02-01-01 | 01 | 1 | SYNC-01 | — | N/A | unit | `go test ./internal/rpc -run Cap -count=1` | ❌ W0 | ⬜ pending |
| 02-01-01 | 01 | 1 | SYNC-02 | — | N/A | unit | `go test ./internal/indexer -run Retry -count=1` | ❌ W0 | ⬜ pending |
| 02-01-02 | 01 | 1 | SYNC-03 | — | N/A | integration | `go test ./internal/storage -run 'Checkpoint\|CrashBetween' -count=1` | ❌ W0 | ⬜ pending |
| 02-01-02 | 01 | 1 | SYNC-04 | — | N/A | integration | `go test ./internal/storage -run Replay -count=1` | ⚠️ extend | ⬜ pending |
| 02-02-01 | 02 | 2 | SYNC-05 | T-02-02 / — | N/A | unit+integration | `go test ./internal/indexer ./internal/storage -run 'Mismatch\|Rewind' -count=1` | ❌ W0 | ⬜ pending |
| 02-02-01 | 02 | 2 | D-02 | — | N/A | integration | `DEVNET_RPC_URL=... go test ./internal/indexer -run TestDevnet -count=1` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/indexer/sync_test.go` — SYNC-01/02 loop behavior against canned/two-chain RPC harness
- [ ] `internal/storage/clickhouse_test.go` extensions — Checkpoint / CrashBetween / Replay-extend / Rewind (probe recipes in RESEARCH Patterns 2-4 are the oracles)
- [ ] `internal/rpc/client_test.go` extensions — two-chain harness + `-32005` message discrimination (cap vs throttle vs timeout)
- [ ] `internal/indexer/devnet_test.go` — env-gated `DEVNET_RPC_URL` suite (restart/resume/replay + real-USDT-bytecode deploy)
- [ ] `testdata/usdt_creation.hex` — bytecode fixture (extraction recipe in 02-RESEARCH.md: tx `0x2f1c5c2b...` via archive RPC)
- [ ] `go test ./...` green offline baseline (verified 2026-10-09)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Continuous-run operator observation | SYNC-01 | Long-running live behavior | Run the indexer continuously for ≥15 min against Mainnet (via proxy env); confirm it follows finalized head advances without gaps in `roadmap`-observable state |
| Induced-mismatch operator UX | SYNC-05 | Destructive-ish live procedure | Follow README rewind section: stop, corrupt/replace checkpoint hash via the documented harness, observe loud stop + `rewind` subcommand recovery |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 60s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
