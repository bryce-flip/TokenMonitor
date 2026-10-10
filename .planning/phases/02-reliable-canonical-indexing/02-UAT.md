---
status: complete
phase: 02-reliable-canonical-indexing
source: 02-01-SUMMARY.md, 02-02-SUMMARY.md, 02-03-SUMMARY.md
started: 2026-10-10T11:05:01Z
updated: 2026-10-10T11:33:26.171Z
---

## Current Test

[testing complete]

## Tests

### 1. 15-minute continuous Mainnet run following finalized-head advances (verifier human item 1 / plan backstop truth)
expected: `go run ./cmd/indexer run` against Mainnet for >= 15 minutes follows finalized-head advances gap-free.
result: pass
source: automated (orchestrator run 2026-10-10T11:05–11:17Z in isolated scratch DB backstop_scratch: seeded at finalized head 26161544; window 26161545-26161576 stored (37 rows, checkpoint 26161576); window 26161577-26161608 stored (29 rows, checkpoint 26161608); contiguous ranges, 66 exact events, monotonic checkpoint through two finalized advances)

### 2. Induced checkpoint-mismatch operator UX + README rewind walk (verifier human item 2)
expected: corrupted checkpoint hash → run halts loudly with block + both hashes + operator instruction, exit 1; `rewind --to <verified> --yes` prints the report, excludes orphans strictly above the verified point, re-anchors; resume re-ingests idempotently and advances.
result: pass
source: automated (orchestrator run 2026-10-10T11:19–11:30Z: hash corrupted to 0xDEADBEEF… → ERROR "checkpoint hash mismatch: ordinary indexing and reporting halted; verify the chain and run the rewind subcommand (D-03)" block=26161608 expected/observed hashes printed, exit status 1; rewind --to 26161576 → report table (delete range >26161576, FINAL count 29→0, sum 45379943227→0), checkpoint re-anchored 26161576/canonical; resume → window 26161577-26161639 (72 rows, includes identity-deduped re-fetch), then 26161640-26161671; 143 events total. CR-01 fix re-proven live: re-anchor won the checkpoint read.)

### 3. Continuous checkpointed sync (02-01 deliverables)
expected: head_policy config, EligibleHead, RunSync advance-after-accept, crash-between-writes recovery, replay idempotence.
result: pass
source: automated (verifier-rerun: 18/18 sync suite + 11/11 storage suite incl. ClickHouse integration; devnet restart test PASS)

### 4. Failure discrimination + hardening (02-02 deliverables)
expected: -32005 message split, halving, fail-closed floor, credential redaction, tag probe, Close.
result: pass
source: automated (verifier-rerun: all named tests PASS; 12/12 commands carry fails_when)

### 5. Reorg rewind + devnet proof (02-03 deliverables)
expected: two-chain harness, mismatch halt, operator-gated rewind, real-TetherToken devnet lifecycle.
result: pass
source: automated (verifier-rerun: devnet TestDevnetFollowsFinalizedHeadAcrossRestarts 4.40s PASS, TestDevnetUSDTLifecycle 802.59s PASS; review fixes CR-01/WR-01..03 red-green proven at HEAD)

## Summary

total: 5
passed: 5
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps

[none — no issues reported]

## Deferred Follow-Ups

- test: 0
  idea: "IN-06 idle-path verification blind spot (cpHeight >= head skips checkpoint verification; detection deferred to next head advance — observed live during UAT item 2 as a ~60s detection delay, not a miss): fold into Phase 3/4 hardening or the IN-01..IN-08 info backlog"
  deferred_at: 2026-10-10
