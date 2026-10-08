---
status: testing
phase: 01-live-supply-event-path
source: 01-01-SUMMARY.md
started: 2026-10-08T10:31:14Z
updated: 2026-10-08T12:15:00Z
---

## Current Test
<!-- OVERWRITE each test - shows where we are -->

number: 1
name: Cold Start Smoke Test
expected: |
  Kill any running service (docker rm -f tm-clickhouse). Start from scratch: recreate the
  ClickHouse container, run the bounded ingest against a zero-log range
  (env -u ETH_RPC_URL go run ./cmd/indexer -from 100 -to 200), and confirm the indexer
  boots cleanly: it fails fast naming ETH_RPC_URL (D-02 contract), the migrations apply
  idempotently, and go test ./... stays green on a fresh state.
awaiting: user response

## Tests

### 1. Cold Start Smoke Test
expected: Kill any running service, clear ephemeral state, start from scratch — indexer boots without errors, migrations apply, fail-fast probe behaves.
result: pass
source: automated (orchestrator run 2026-10-08: container recreated 26.8.20.9; env -u ETH_RPC_URL run exits 1 naming ETH_RPC_URL; live zero-log range exits 0 with empty range-scoped inspect; SHOW TABLES confirms idempotent schema; go test ./... green)

### 2. Bounded walking-skeleton command (config, probe, ingest)
expected: Bounded command loads/validates config, fails fast without ETH_RPC_URL, runs the D-02 probe before any fetch, zero-log range exits 0.
result: pass
source: automated
coverage_id: D1

### 3. Decoder classifies five pinned Mainnet fixtures exactly
expected: USDT Issue/Redeem/DestroyedBlackFunds and USDC zero-address Transfer classify with exact amounts; USDT zero-address Transfer, USDC ordinary Transfer, and paired Mint/Burn yield no rows; malformed shapes are named errors.
result: pass
source: automated
coverage_id: D2

### 4. Fail-fast provider probe behavior
expected: chain id 2 rejected; confirmed range + working sample getLogs accepted; confirmation boundary exact (head-conf accepted, one past rejected); history-refusing provider rejected.
result: pass
source: automated
coverage_id: D3

### 5. ClickHouse exact storage
expected: UInt256 round trip incl. 2^256-1 as exact decimal strings; duplicate insert collapses to FINAL count 1; deterministic ordering; idempotent schema; empty-slice no-op.
result: pass
source: automated
coverage_id: D4

### 6. README operator runbook walk-through (verifier human item 1)
expected: Follow README.md start to finish as a new operator: prerequisites, env vars, ClickHouse startup, bounded ingest command, inspection SQL, test commands. Every step works as written with no gaps.
result: pass
reported: user approved README walkthrough via UAT gate

### 7. RPC credential in error logs — accept or fix (verifier human item 2)
expected: Decide: `%w`-wrapped net/http error chains in internal/rpc can print the full key-bearing RPC URL in runtime error logs (tracked files are clean; only test output is scrubbed — 01-02-SUMMARY Deviation 4). Accept as-is for Phase 1, or ask for a fix (redact to scheme://host in error rendering).
result: [pending]

### 8. Deployed USDT zero-address Transfer negative fixture (verifier human item 3, research A4)
expected: Decide: the decoder's USDT zero-address-Transfer negative is proven offline (crafted fixture) and live USDT Transfers yield no rows, but no deployed historical zero-address USDT Transfer tx has been pinned as a fixture. Accept the current evidence, or defer to Phase 2 research to locate one on chain.
result: [pending]

### 9. ROADMAP goal user-story format (verifier human item 4)
expected: Decide: ROADMAP Phase 1 goal fails user-story validation (the validated story lives in the PLAN objectives). Optionally run /gsd-mvp-phase 1 to reformat, or accept the roadmap wording as-is.
result: [pending]

### 10. WR-01 reorg-mismatch guard (verifier human item 5, new after fixes)
expected: Decide: the fail-loud h.Hash() != log.BlockHash branch is only observable under a live Mainnet reorg. Accept by code inspection (happy path live-verified), or defer explicit reorg testing to Phase 2 (SYNC-05).
result: [pending]

## Summary

total: 10
passed: 6
issues: 0
pending: 4
skipped: 0
blocked: 0

## Gaps

[none yet]
