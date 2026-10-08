---
status: complete
phase: 01-live-supply-event-path
source: 01-01-SUMMARY.md
started: 2026-10-08T10:31:14Z
updated: 2026-10-08T12:44:42.104Z
---

## Current Test

[testing complete]

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
result: pass
reported: "user: 7 accept — matches AR-01 accepted risk in 01-SECURITY.md"

### 8. Deployed USDT zero-address Transfer negative fixture (verifier human item 3, research A4)
expected: Decide: the decoder's USDT zero-address-Transfer negative is proven offline (crafted fixture) and live USDT Transfers yield no rows, but no deployed historical zero-address USDT Transfer tx has been pinned as a fixture. Accept the current evidence, or defer to Phase 2 research to locate one on chain.
result: skipped
reason: "Deferred follow-up: user deferred to Phase 2 research — locate and pin a deployed historical zero-address USDT Transfer transaction as a chain-verified negative fixture (research item A4)".

### 9. ROADMAP goal user-story format (verifier human item 4)
expected: Decide: ROADMAP Phase 1 goal fails user-story validation (the validated story lives in the PLAN objectives). Optionally run /gsd-mvp-phase 1 to reformat, or accept the roadmap wording as-is.
result: pass
reported: "user: 9 accept — roadmap goal wording accepted as-is; validated user story lives in both PLAN objectives"

### 10. WR-01 reorg-mismatch guard (verifier human item 5, new after fixes)
expected: Decide: the fail-loud h.Hash() != log.BlockHash branch is only observable under a live Mainnet reorg. Accept by code inspection (happy path live-verified), or defer explicit reorg testing to Phase 2 (SYNC-05).
result: pass
reported: "user: 10 accept by code inspection — happy path live-verified 2026-10-08; explicit reorg exercise arrives with Phase 2 SYNC-05 (Kurtosis localnet proposed as its test environment)"

## Summary

total: 10
passed: 9
issues: 0
pending: 0
skipped: 1
blocked: 0

## Gaps

[none — no issues reported]

## Deferred Follow-Ups

- test: 8
  idea: "Pin a deployed historical zero-address USDT Transfer transaction as a chain-verified negative fixture (research A4)"
  deferred_at: 2026-10-08
- test: 10
  idea: "Kurtosis local Ethereum testnet as Phase 2 reorg/behavior test environment (user-proposed): deploy real TetherToken source for identical event semantics, force competitive-fork reorgs to exercise WR-01 guard and SYNC-05 end-to-end, rate-limit-free RPC for restart/replay tests. Localnet is for behavior tests, never a substitute for Mainnet classification fixtures."
  deferred_at: 2026-10-08
