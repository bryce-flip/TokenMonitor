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
| 02-01-01 | 01 | 1 | SYNC-01, SYNC-03 | T-02-01 / T-02-04 | mismatch halt + bound params | unit | `env -u ETH_RPC_URL -u CLICKHOUSE_URL go test ./internal/indexer ./internal/config ./internal/rpc ./internal/storage -count=1` | ❌ created by task | ⬜ pending |
| 02-01-02 | 01 | 1 | SYNC-03, SYNC-04 | — | N/A | integration | `CLICKHOUSE_URL=... go test ./internal/storage -run 'Checkpoint\|CrashBetween\|Replay' -v -count=1` | ❌ created by task | ⬜ pending |
| 02-02-01 | 02 | 2 | SYNC-02 | T-02-07 | fail-closed floor | unit | `env -u ETH_RPC_URL -u CLICKHOUSE_URL go test ./internal/rpc -run 'ResultCap\|HalvesRange\|QueryTimeout\|Throttle\|Floor\|RateLimitExceeded' -v -count=1` | ❌ created by task | ⬜ pending |
| 02-02-02 | 02 | 2 | SYNC-02, SYNC-01 | — | N/A | unit | `env -u ETH_RPC_URL -u CLICKHOUSE_URL go test ./internal/indexer -run 'ThrottleRetries\|CapHalves\|FloorTrip' -v -count=1` | ❌ created by task | ⬜ pending |
| 02-02-03 | 02 | 2 | SYNC-01 | T-02-03 | credential redaction (AR-01), loud downgrade | unit | `env -u ETH_RPC_URL -u CLICKHOUSE_URL go test ./internal/rpc ./internal/indexer -run 'Redact\|SupportsHeadTag\|Close\|Downgrades' -v -count=1` | ❌ created by task | ⬜ pending |
| 02-03-01 | 03 | 2 | SYNC-05 | T-02-02 / T-02-05 | operator-gated rewind, never auto | unit | `env -u ETH_RPC_URL -u CLICKHOUSE_URL go test ./internal/indexer -run 'Mismatch\|Rewind' -v -count=1` | ❌ created by task | ⬜ pending |
| 02-03-02 | 03 | 2 | SYNC-05 | T-02-02 | bound-param DELETE | integration | `CLICKHOUSE_URL=... go test ./internal/storage -run 'Rewind\|Reinsert\|RangeTotals' -v -count=1` | ❌ created by task | ⬜ pending |
| 02-03-03 | 03 | 2 | SYNC-05, D-02 | T-02-06 | public fixture key documented | integration (env-gated) | `DEVNET_RPC_URL=... CLICKHOUSE_URL=... go test ./internal/indexer -run 'TestDevnet' -v -count=1 -timeout 20m` | ❌ created by task | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

All Wave 0 test infrastructure is created inside the plans (tests are written alongside the code they prove, per task):

- [ ] `internal/indexer/sync_test.go` — created by 02-01 Task 1 (canned-HTTP helper + fakeEventStore + six TestRunSync* behaviors); extended by 02-02 Tasks 2-3
- [ ] `internal/storage/clickhouse_test.go` extensions — Checkpoint / CrashBetween / Replay-extend in 02-01 Task 2; Rewind / Reinsert / RangeTotals in 02-03 Task 2 (probe recipes in RESEARCH Patterns 2-4 are the oracles)
- [ ] `internal/rpc/client_test.go` extensions — -32005 message discrimination (cap vs throttle vs timeout) + redaction/tag/Close in 02-02 Tasks 1 and 3
- [ ] `internal/indexer/reorg_test.go` — two-chain reorg harness + mismatch/rewind tests in 02-03 Task 1
- [ ] `internal/indexer/devnet_test.go` — env-gated `DEVNET_RPC_URL` suite (restart/resume/replay + real-USDT-bytecode deploy) in 02-03 Task 3
- [ ] `testdata/usdt_creation.hex` — bytecode fixture (extraction recipe in 02-RESEARCH.md: tx `0x2f1c5c2b...` via archive RPC) in 02-03 Task 3
- [x] `go test ./...` green offline baseline (verified 2026-10-09)

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
