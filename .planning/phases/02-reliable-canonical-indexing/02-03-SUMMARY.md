---
phase: 02-reliable-canonical-indexing
plan: "03"
subsystem: ingestion
tags: [go, ethereum, go-ethereum, clickhouse, reorg, rewind, lightweight-delete, kurtosis, devnet, tether-token, operator-runbook, api-coverage]

requires:
  - phase: 02-reliable-canonical-indexing
    plan: "01"
    provides: RunSync loop, ChainReader/EventStore seams, CheckpointMismatchError, checkpoint ordering, EligibleHead
  - phase: 02-reliable-canonical-indexing
    plan: "02"
    provides: FetchLogsResilient, SupportsHeadTag/Host, Client.Close, credential scrub
provides:
  - internal/indexer/rewind.go Rewind(ctx, cr, rs, cfg, opts) with the bounded ancestor walk (DefaultRewindMaxWalk 1000), RewindStore/RewindOpts/RewindReport/RangeTotalsResult, named errors (ErrRewindNoCheckpoint/ErrRewindWalkTooDeep/ErrRewindToMismatch/ErrRewindUnconfirmed), dry-run plan + Yes-gated destructive steps strictly above the verified point
  - internal/storage Store.DeleteEventsFrom (exact lightweight DELETE, bound params), Store.StoredBlockHashes (FINAL, GROUP BY block), Store.RangeTotals (Pattern 3 oracle shape, *big.Int)
  - cmd/indexer rewind subcommand (--to/--yes, stdin confirmation, tabwriter before/after report, non-zero on verification failure); rpc Client.Close wired in both subcommands
  - internal/indexer/reorg_test.go two-chain httptest harness (chain B diverges at forkAt, atomic flip) + four behavior proofs
  - internal/indexer/devnet_test.go env-gated suite — restart/resume/replay continuity against the real finalized head (recordingStore proves the gap-free checkpoint sequence) + real TetherToken lifecycle (deploy verbatim bytecode, issue/redeem, exact-amount classification, full-range replay unchanged)
  - testdata/usdt_creation.hex 11,900-byte Mainnet TetherToken creation bytecode fixture (first testdata file; provenance in README section 8)
  - README sections 5/7/8 (rewind procedure, Kurtosis localnet runbook, fixture provenance); Phase 2 COVERAGE.md API matrix
affects: [02-reliable-canonical-indexing, 03-supply-anchoring, 04-observability-deployment]

actuals:
  tokens: 27450   # chars/4 over the realized diff (109802 chars, 11 files)
  tasks: 3
  commits: 3      # measured: git rev-list --count 87b8b75..HEAD

plan_head_before: 87b8b75c48fe047767ee2d24350b119db155757a
plan_head_after: 6d216684bb803e547395d9b69607a3639e5763b5

tech-stack:
  added: []       # no new modules; go.mod surfaced golang.org/x/crypto + fsnotify as indirect (transitive of the pinned go-ethereum via its crypto package, imported by the devnet test); go mod verify clean
  patterns: [row-less ancestor-walk heights pass through while stored rows remain below (orphan coverage), dry-run-plan + confirmed-execute split with the prompt in the CLI, per-test fresh ClickHouse database as the isolation namespace (RunSync's chain key is a constant), recording EventStore wrapper to observe the exact checkpoint-write sequence (ReplacingMergeTree merges destroy the physical history), deterministic CreateAddress deployments from the public package test key]

key-files:
  created:
    - internal/indexer/rewind.go
    - internal/indexer/reorg_test.go
    - internal/indexer/devnet_test.go
    - testdata/usdt_creation.hex
    - .planning/phases/02-reliable-canonical-indexing/COVERAGE.md
  modified:
    - internal/storage/clickhouse.go
    - cmd/indexer/main.go
    - README.md
    - go.mod
    - go.sum

key-decisions:
  - "The ancestor walk terminates a row-less height only when no stored rows remain AT OR BELOW it: a naive first reading (stop at the first height with no rows) re-anchors at the checkpoint itself whenever the checkpoint block carries no events, leaving orphaned events below it undeleted — TestRewindSameIdentityReinsertPath caught this before commit"
  - "Rewind with Yes=false is a pure dry run returning the plan + ErrRewindUnconfirmed; the confirmation prompt lives in the CLI (plan-specified split), which prints the plan, prompts on stdin for an explicit yes, and re-invokes with Yes — Rewind itself never prompts"
  - "DeleteEventsFrom is exactly `DELETE FROM stablecoin_events WHERE chain = ? AND block_number > ?` (lightweight, never ALTER): immediately FINAL-visible, merge-safe, re-insert-safe — all three now pinned by tests on the pinned server"
  - "The devnet suite isolates per test run in a fresh ClickHouse database (tm_devnet_<unixnano>): RunSync keys checkpoints under the constant ethereum chain, so the database is the only namespace boundary; a control connection owns CREATE/DROP and the replay TRUNCATE"
  - "The lifecycle test syncs at head_policy confirmed with confirmation_blocks 64 (documented deviation, see below): the enclave's beacon finality covers a tip block in ~20 minutes (measured 2026-10-09 19.2 min, 2026-10-10 ~20 min, batched epoch jumps), which cannot fit the plan's pinned 20-minute go-test budget; real finalized-head following is proven by the restart test, which runs finalized and passes"
  - "The prefunded-key constant in 02-RESEARCH was one hex nibble short; the single-nibble repair that derives exactly the funded address 0x8943...a776 was found cryptographically (1024 candidates) and is documented at the constant"

patterns-established:
  - "Destructive recovery is a two-phase UX: compute-and-report (pure, no mutation) then confirm-and-execute; every verification failure leaves stored data untouched and exits non-zero"
  - "Env-gated integration suites isolate with disposable databases/chain namespaces rather than shared-table cleanup, so a killed run can never corrupt the operator's real tables"
  - "Deterministic test contracts: deploy verbatim Mainnet creation bytecode from the public ethereum-package test key — the deployed address is a function of (key, nonce), making deployments reproducible across chain re-provisions"

requirements-completed: [SYNC-05, SYNC-03]

coverage:
  - id: D1
    description: "Mismatch halt proven against a genuinely conflicting chain: two-chain httptest harness, typed diagnostic naming height + both hashes, zero getLogs after the mismatch"
    requirement: SYNC-05
    verification:
      - kind: unit
        ref: "env -u ETH_RPC_URL -u CLICKHOUSE_URL go test ./internal/indexer -run 'Mismatch|Rewind' -v -count=1 — TestResumeMismatchHaltsWithDiagnostic RUN and PASS"
      - kind: build
        ref: "go build ./... && go vet ./... exit 0"
    status: pass
    human_judgment: false
  - id: D2
    description: "Operator-gated verified rewind: bounded ancestor walk (MaxWalk 1000) to the first agreeing/row-free height, orphan exclusion strictly above the verified point, checkpoint re-anchor with the canonical hash, before/after FINAL totals, rejected unverified --to overrides"
    requirement: SYNC-05
    verification:
      - kind: unit
        ref: "same command — TestRewindWalksToCommonAncestorAndExcludesOrphans, TestRewindToOverrideRejectedOnDisagreement, TestRewindSameIdentityReinsertPath RUN and PASS; rewind.go carries no code path deleting at or below the verified point"
      - kind: cli
        ref: "cmd/indexer dispatches rewind with --to/--yes and exits non-zero on Rewind error; unknown-subcommand smoke exits 2"
    status: pass
    human_judgment: false
  - id: D3
    description: "Storage rewind recipe on the pinned server: lightweight DELETE immediately FINAL-visible, same-identity re-insert (new block_hash + amount) reads exactly once with the new values before and after OPTIMIZE TABLE ... FINAL, RangeTotals equals the raw oracle query with exact integers"
    requirement: SYNC-05
    verification:
      - kind: integration
        ref: "CLICKHOUSE_URL=clickhouse://default@127.0.0.1:9000/default go test ./internal/storage -run 'Rewind|Reinsert|RangeTotals' -v -count=1 — all three RUN and PASS on 26.8.20.9"
    status: pass
    human_judgment: false
  - id: D4
    description: "Devnet restart/resume/replay continuity against the real consensus finalized head: cancelled mid-catch-up, resumed with a strictly-increasing, gap-free (max advance = one window) checkpoint sequence across the restart boundary, FINAL totals over the covered range unchanged"
    requirement: SYNC-03
    verification:
      - kind: integration
        ref: "DEVNET_RPC_URL=http://127.0.0.1:32769 CLICKHOUSE_URL=... go test ./internal/indexer -run TestDevnet -v -count=1 -timeout 20m — TestDevnetFollowsFinalizedHeadAcrossRestarts PASS (2.40s)"
    status: pass
    human_judgment: false
  - id: D5
    description: "Real TetherToken (verbatim Mainnet creation bytecode) on the localnet: classified Issue->mint / Redeem->redeem rows with the exact amounts sent, surviving a full-range replay through the real sync pipeline with FINAL totals and row set unchanged (D-02)"
    requirement: SYNC-05
    verification:
      - kind: integration
        ref: "same command — TestDevnetUSDTLifecycle PASS (798.72s): deploy at 0x8F03...952c, issue/redeem driven via ABI-packed txs, first sync rows=1+1, replay window re-ingests rows=2 collapsed"
    status: pass
    human_judgment: false
  - id: D6
    description: "Operator surface and coverage record: rewind procedure + Kurtosis localnet + fixture provenance in README; Phase 2 API matrix with finalized/safe tags INTEGRATE and reasoned OPT-OUTs; no credential value in any tracked file"
    requirement: SYNC-05
    verification:
      - kind: grep
        ref: "test -f COVERAGE.md && grep -q finalized COVERAGE.md && ! grep -Eq '(alchemy|infura|quicknode) URLs' README.md — exit 0; fixture gate 23,800 hex chars exit 0"
      - kind: unit
        ref: "env -u ETH_RPC_URL -u CLICKHOUSE_URL -u DEVNET_RPC_URL go test ./... -count=1 exit 0 (devnet suite skips cleanly)"
    status: pass
    human_judgment: false

duration: 132min   # active execution across a session interruption (2026-10-09 11:09-12:08 UTC, 2026-10-10 07:00-08:12 UTC)
completed: 2026-10-10T08:12:03Z
status: complete
---

# Phase 2 Plan 03: Reorg Rewind + Devnet Proof Summary

One-liner: operator-gated verified rewind (bounded ancestor walk, lightweight-DELETE orphan exclusion strictly above the verified point, checkpoint re-anchor, before/after report) proven against a deterministic two-chain harness and the pinned ClickHouse, plus the Kurtosis devnet suite — restart continuity under the real finalized head and a verbatim-bytecode TetherToken whose issue/redeem events classify exactly and survive replay unchanged.

## Performance

- 3 tasks, 3 commits, ~132 active minutes across a session interruption (estimate 48k tokens / actual ~27.5k tokens over an 11-file, 109,802-char diff; the third low-confidence estimate in this phase to come in lean — the 02-01/02-02 seams again lifted almost verbatim)
- Offline suite (all three env gates unset): exit 0 — devnet tests skip with a message naming DEVNET_RPC_URL and the kurtosis commands
- Reorg suite, ClickHouse rewind suite, devnet suite (pinned commands): all exit 0 with every named test RUN and PASS
- go build/vet/gofmt clean; go mod verify clean; no new modules (two indirect lines surfaced, see tech-stack)

## Accomplishments

- **SYNC-05 end to end** — the mismatch halt (02-01) is now proven against a genuinely conflicting chain: the two-chain harness flips the served chain at a fork point, `RunSync` returns the typed diagnostic naming height + both hashes before any getLogs, and the checkpoint is untouched.
- **D-03 exactly** — `rewind` is the only code path that deletes events; `Rewind` computes a pure plan (expected/observed hashes, walked point, before-totals), the CLI prompts for an explicit `yes` unless `--yes`, the delete is strictly `block_number > verified_point` (never at or below), the checkpoint re-anchors with the provider-canonical hash, and any verification failure — including an unverified `--to` override or a walk past 1000 blocks — exits non-zero having deleted nothing.
- **Probe-verified recipe pinned** — on the pinned 26.8.20.9 server: lightweight DELETE is FINAL-visible immediately, a same-identity re-insert (the re-included-transaction shape) reads exactly once with the NEW values, and `OPTIMIZE TABLE ... FINAL` changes nothing.
- **D-02 delivered in scope** — the localnet suite proves restart/resume/replay continuity under real consensus finality and drives a real TetherToken (the actual Mainnet creation bytecode, no solc) through the production decoder; the deterministic reorg path lives in the httptest harness per the scope guard; the deferred A4 zero-address fixture idea stayed out.
- **Operator surface** — README gains the reorg-safety/rewind runbook, the Kurtosis localnet section, and the fixture provenance record; COVERAGE.md records the Phase 2 API matrix (finalized/safe tags INTEGRATE, WebSocket/eth_call/receipt OPT-OUT with reasons, deployment TEST-ONLY).

## Task Commits

| Task | Commit | Subject |
|------|--------|---------|
| 1 — verified rewind + rewind subcommand + two-chain harness | `1ddb1e9` | feat(02-03): verified rewind — ancestor walk, orphan exclusion, re-anchor, rewind subcommand |
| 2 — ClickHouse rewind proofs | `dc45b2e` | test(02-03): ClickHouse rewind proofs — immediate orphan exclusion, same-identity re-insert survival, OPTIMIZE invariance |
| 3 — devnet proof, runbook, coverage matrix | `6d21668` | feat(02-03): Kurtosis devnet proof with real TetherToken bytecode, operator runbook, API coverage matrix |

## Files

Created: `internal/indexer/rewind.go`, `internal/indexer/reorg_test.go`, `internal/indexer/devnet_test.go`, `testdata/usdt_creation.hex`, `.planning/phases/02-reliable-canonical-indexing/COVERAGE.md`.
Modified: `internal/storage/clickhouse.go`, `cmd/indexer/main.go`, `README.md`, `go.mod`, `go.sum`.

## Decisions

(Recorded in frontmatter key-decisions; the load-bearing ones:)
- The ancestor walk's row-less termination must check rows AT OR BELOW the height — the naive reading re-anchors above undetected orphans (caught by TestRewindSameIdentityReinsertPath before commit).
- The lifecycle devnet test runs confirmed-64 instead of finalized: the enclave's finality covers a tip block in ~20 minutes, which cannot coexist with the deploy + replay inside the plan's pinned 20-minute suite budget; finality-following is the restart test's subject and it passes under finalized.
- Per-test fresh ClickHouse databases are the isolation namespace (the chain key is a compile-time constant), so a killed test run can never touch operator data.

## Deviations from Plan

**1. [Rule 1 - Bug] Ancestor walk terminated at the first row-less height, leaving orphans below it undeleted**
- **Found during:** Task 1 (first run of TestRewindSameIdentityReinsertPath)
- **Issue:** the plan's "first height with no stored rows" read literally stops at the checkpoint itself whenever the checkpoint block carries no events — orphaned events at lower heights survive the rewind.
- **Fix:** a row-less height terminates the walk only when no stored rows remain at or below it; row-less heights above stored rows are passed through (no header fetch needed there).
- **Files modified:** internal/indexer/rewind.go
- **Verification:** all four reorg tests PASS; the re-insert test now sees the rewind land at the true common ancestor with DeleteEventsFrom bound exactly there.
- **Commit:** `1ddb1e9`

**2. [Rule 3 - Blocking] The prefunded-key constant in 02-RESEARCH was one hex nibble short**
- **Found during:** Task 3 (first devnet run — "invalid hex data for private key")
- **Issue:** the research artifact's key copy dropped one character (63 hex chars), so the documented fixture key could not even parse.
- **Fix:** brute-forced the 64 positions x 16 nibbles against the known funded address; exactly one candidate derives 0x8943545177806ED17B9F23F0a21ee5948eCaa776 — documented at the constant.
- **Files modified:** internal/indexer/devnet_test.go
- **Verification:** deployment + issue/redeem transactions mined with status 1 on the enclave.
- **Commit:** `6d21668`

**3. [Rule 3 - Blocking] The lifecycle test cannot wait for beacon finality inside the pinned 20-minute suite budget**
- **Found during:** Task 3 (devnet runs 2-4, measured twice)
- **Issue:** the enclave finalizes a tip block in ~20 minutes (batched epoch jumps: 19.2 min on 2026-10-09, ~20 min on 2026-10-10); the plan's pinned verify command (`-timeout 20m`) cannot contain deploy + finality wait + sync + replay.
- **Fix:** the lifecycle test syncs at head_policy `confirmed` with confirmation_blocks 64 (2 epochs of depth, the D-01 policy-space margin), with the rationale and both measurements in a comment; the finalized-head behavior itself is proven by TestDevnetFollowsFinalizedHeadAcrossRestarts, which runs finalized and passes.
- **Files modified:** internal/indexer/devnet_test.go, README.md (localnet budget note)
- **Verification:** the pinned devnet command exits 0; the full suite runs in ~13.4 minutes.
- **Commit:** `6d21668`

**4. [Rule 1 - Bug] The lifecycle test started RunSync before the eligible head covered the seed (D-04 immediate error, silently swallowed)**
- **Found during:** Task 3 (runs 4-5: zero window logs after deploy)
- **Issue:** RunSync exits instantly with "start_block above the eligible head" when seeded at a just-deployed tip block; the test polled an exited loop for the full budget without surfacing the error.
- **Fix:** wait for the eligible head to cover the seed BEFORE starting the loop; cancel-and-await the sync goroutine right after each wait so an early exit fails fast with its real error.
- **Files modified:** internal/indexer/devnet_test.go
- **Verification:** TestDevnetUSDTLifecycle PASS — first sync windows rows=1+1, replay window re-ingests rows=2 collapsed, totals unchanged.
- **Commit:** `6d21668`

**5. [Rule 3 - Blocking] go.mod/go.sum needed `go mod tidy` after the devnet test imported go-ethereum's crypto package**
- **Found during:** Task 3 (first build)
- **Issue:** `go vet` refused to run ("updates to go.mod needed"); the devnet test's imports (go-ethereum crypto, clickhouse-go driver) surface two already-pinned transitive modules (golang.org/x/crypto, fsnotify) as indirect requires.
- **Fix:** `go mod tidy` — no version changes, no new modules; `go mod verify` clean.
- **Files modified:** go.mod, go.sum
- **Verification:** build/vet/offline suite green.
- **Commit:** `6d21668`

Otherwise the plan executed as written (environment interruption aside: the session was cut overnight during a devnet wait and resumed the next morning with the enclave re-provisioned by the operator-side restart; the suite was re-run from scratch against the fresh chain).

## Issues

- None open for this plan. The environment note above (devnet finality cadence vs the 20m budget) is fully mitigated by deviation 3 and documented in README and the test; the 02-01 D6 backstop manual run in WINDOWS.md is unchanged.

## User Setup

Nothing new: same env placeholders (ETH_RPC_URL / CLICKHOUSE_URL), plus the optional DEVNET_RPC_URL gate documented in README section 7. No credential value in any tracked file.

## Next Phase Readiness

Phase 2 complete — ready for verification (`/gsd-verify-work`; the phase gate re-runs the devnet suite per 02-VALIDATION.md). Phase 3 (supply anchoring) can consume `RangeTotals` for rollup health checks and the rewind seam for its Phase 4 derived-rollup rebuild hook.

## Self-Check: PASSED

- Created files exist: internal/indexer/rewind.go, internal/indexer/reorg_test.go, internal/indexer/devnet_test.go, testdata/usdt_creation.hex, COVERAGE.md — FOUND
- Commits are ancestors of HEAD: 1ddb1e9, dc45b2e, 6d21668 — FOUND
- Commits measured 3 via `git rev-list --count 87b8b75..HEAD` (matches per-task commits; no uncommitted code changes)
