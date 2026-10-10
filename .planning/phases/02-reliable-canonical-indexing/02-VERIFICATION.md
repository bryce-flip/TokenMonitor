---
phase: 02-reliable-canonical-indexing
verified: 2026-10-10T11:05:00Z
status: passed
score: 21/22 must-haves verified
covered_files:
  - .planning/phases/02-reliable-canonical-indexing/02-01-PLAN.md
  - .planning/phases/02-reliable-canonical-indexing/02-01-SUMMARY.md
  - .planning/phases/02-reliable-canonical-indexing/02-02-PLAN.md
  - .planning/phases/02-reliable-canonical-indexing/02-02-SUMMARY.md
  - .planning/phases/02-reliable-canonical-indexing/02-03-PLAN.md
  - .planning/phases/02-reliable-canonical-indexing/02-03-SUMMARY.md
  - README.md
  - cmd/indexer/main.go
  - config/tokens.json
  - go.mod
  - go.sum
  - internal/config/config.go
  - internal/config/config_test.go
  - internal/indexer/devnet_test.go
  - internal/indexer/reorg_test.go
  - internal/indexer/rewind.go
  - internal/indexer/rewind_integration_test.go
  - internal/indexer/sync.go
  - internal/indexer/sync_test.go
  - internal/rpc/client.go
  - internal/rpc/client_test.go
  - internal/storage/clickhouse.go
  - internal/storage/clickhouse_test.go
  - migrations/clickhouse.sql
  - testdata/usdt_creation.hex
covered_digest: "v3:sha256:d48ec17318bf2a1d32b59ce53d268a175591367c2d33ee5e72fbf1293d1b3514"
behavior_unverified: 1 # the plan's own verification: backstop truth (15-minute continuous Mainnet operator observation) — present + wired + devnet-proven for minutes, but the 15-min Mainnet observation is deliberately manual (02-VALIDATION.md manual table; WINDOWS.md open item)
overrides_applied: 0
behavior_unverified_items:
  - truth: "A continuous Mainnet run follows finalized-head advances for at least 15 minutes without block gaps (manual operator observation per 02-VALIDATION.md manual table)"
    test: "Run the indexer continuously for >= 15 minutes against Mainnet (ETH_RPC_URL via the operator's proxy) and watch the per-window progress logs"
    expected: "Window bounds advance monotonically with no gaps (each window_from = previous checkpoint + 1) as the finalized head moves; no errors; checkpoint advances track the finalized head"
    why_human: "The plan marks this truth verification: backstop — a long-running live Mainnet observation that no automated suite exercises; the verifier must not mutate the operator's real ClickHouse data by starting the run itself"
human_verification:
  - test: "Run the indexer continuously for >= 15 minutes against Mainnet (`go run ./cmd/indexer run` with ETH_RPC_URL exported, per README) and observe the per-window slog progress lines"
    expected: "Checkpoint height follows finalized-head advances in gap-free bounded windows (window_from always = previous checkpoint + 1); no errors; closes the open WINDOWS.md backstop item for Phase 2"
    why_human: "Long-running live provider behavior; the plan explicitly marks this truth verification: backstop and defers it to the phase-gate manual table (02-VALIDATION.md); running it here would write to the operator's real ClickHouse tables"
  - test: "Induced-mismatch operator UX: stop the indexer, corrupt/replace the stored checkpoint hash via the documented harness, restart, then follow README section 5 (rewind procedure) end to end"
    expected: "Loud halt with the typed diagnostic naming block height + expected/observed hashes and exit 1 before any ingest; `go run ./cmd/indexer rewind` prints the before/after report and requires explicit confirmation; recovery re-anchors at the verified ancestor"
    why_human: "Destructive-ish live procedure against real storage (02-VALIDATION.md manual-only table); the deterministic halves are proven by TestResumeMismatchHaltsWithDiagnostic and the rewind suites, but the end-to-end operator walkthrough is manual by design"
---

# Phase 2: Reliable Canonical Indexing Verification Report

**Phase Goal:** Operators can trust continuous event history across RPC failures, restarts, duplicate reads, and short chain reorganizations.
**Verified:** 2026-10-10T11:05:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

**Note on mode:** ROADMAP.md marks this phase `Mode: mvp`, but the goal is not in User Story format (no "As a..., I want..., so that..." shape). Per the MVP-mode format guard this is surfaced as a discrepancy (a `/gsd-mvp-phase` reformat would be needed to build a User Flow Coverage table). Consistent with the Phase 1 precedent (01-VERIFICATION.md), verification proceeded with the standard goal-backward methodology against the ROADMAP Success Criteria and PLAN must_haves.

## Goal Achievement

### Observable Truths

Roadmap Success Criteria map onto the plan truths as: SC1 -> truths 1-2 + 10-14; SC2 -> truths 4-6 + 10-12; SC3 -> truths 7-8; SC4 -> truths 9, 15-20. All four SCs are covered by verified truths below.

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Consecutive windows exactly adjacent; catch-up covers every block seed-to-head exactly once (SYNC-01) | ✓ VERIFIED | internal/indexer/sync.go:144-145 (window = [cp+1, min(cp+WindowBlocks, head)]); TestRunSyncFollowsHeadAcrossTwoWindows PASS (offline run, this pass) |
| 2 | Idle at head sleeps poll_seconds without erroring, inserting, or advancing (SYNC-01) | ✓ VERIFIED | sync.go:124-134 (sleep-only branch); TestRunSyncIdleAtHeadSleepsWithoutAdvancing PASS |
| 3 | Empty window still advances checkpoint with window-end canonical hash (SYNC-03) | ✓ VERIFIED | sync.go:234-239 + storage.InsertEvents empty-slice no-op; TestRunSyncEmptyWindowStillAdvancesCheckpoint PASS |
| 4 | Checkpoint advances only after InsertEvents nil; on failure stays put and window retries (SYNC-03) | ✓ VERIFIED | sync.go:234-239 strict order; TestRunSyncCrashBetweenInsertAndCheckpoint PASS (asserts checkpoint unmoved on injected failure, window re-processed, dedup = one entry per identity) |
| 5 | First run seeds at start_block when set, else at eligible head; nothing before seed fetched (SYNC-03/D-04) | ✓ VERIFIED | sync.go:103-122; TestRunSyncSeedsAtStartBlockAndAtHeadPerD04 PASS (asserts first getLogs FromBlock) |
| 6 | Crash between insert and checkpoint: restart re-processes; FINAL reads one logical set pre-merge and post-OPTIMIZE (SYNC-03/SYNC-04) | ✓ VERIFIED | TestCrashBetweenWritesReplayIsIdempotent PASS on real ClickHouse 26.8.20.9 (tm-clickhouse container, this pass) |
| 7 | Replaying a processed range (incl. wider range) leaves FINAL count/sum byte-identical (SYNC-04) | ✓ VERIFIED | TestReplayTotalsStableBeforeAndAfterOptimize + TestReplayDuplicateInsertKeepsFinalCountAtOne PASS on real ClickHouse (this pass) |
| 8 | Resume-time checkpoint hash mismatch returns *CheckpointMismatchError naming height/expected/observed; exits non-zero before any ingest (SYNC-05 seed) | ✓ VERIFIED | sync.go:137-143 + cmd/indexer/main.go:119-124 (errors.As -> slog.Error with all three keys -> exit 1); TestRunSyncMismatchReturnsTypedError PASS |
| 9 | Two-chain conflicting-chain resume halts with the diagnostic, zero getLogs after mismatch (SYNC-05) | ✓ VERIFIED | internal/indexer/reorg_test.go two-chain harness (forkAt flip); TestResumeMismatchHaltsWithDiagnostic PASS |
| 10 | -32005 result-cap/timeout shapes shrink sub-range and retry immediately; throttle shapes bounded backoff same sub-range; neither advances or skips (SYNC-02) | ✓ VERIFIED | internal/rpc/client.go:40-75, 277-316 (cap checked before throttle); TestResultCapHalvesRangeAndSucceeds, TestQueryTimeoutTreatedAsResultCap, TestThrottleBacksOffSameRange, TestRateLimitExceededMessageIsThrottle, TestRateLimitErrMatchesStatusTokenNotDigitRuns all PASS |
| 11 | Cap persisting at window_floor fails closed with named error naming floor + blocked range; checkpoint unmoved (SYNC-02) | ✓ VERIFIED | client.go:297-301; TestFloorTripFailsClosed + TestRunSyncFloorTripStopsRunCheckpointUnmoved PASS (zero checkpoint writes, no later window fetched) |
| 12 | Exhausted bounded backoff on throttled range fails loudly, never skips (SYNC-02) | ✓ VERIFIED | withRateLimitRetry (client.go:79-90) returns the error after 4 bounded attempts; FetchLogsResilient returns it (line 310) — error path, no partial advance; exercised by the throttle/floor suite |
| 13 | Window size resets per outer window; halving state never leaks (D-05) | ✓ VERIFIED | client.go:283 (`size := window` per call); span-descent assertions in TestResultCapHalvesRangeAndSucceeds PASS |
| 14 | RPC errors carry no URL credentials anywhere in the chain (AR-01) | ✓ VERIFIED | client.go:98-141 scrub/scrubURL at all wrap sites incl. masked-userinfo regex; TestWrappedErrorsRedactCredentials PASS (userinfo-bearing dial URL, transport failures, cancellation Is-contract pinned) |
| 15 | Provider rejecting the finalized tag downgrades to confirmed only after a loud warning naming the downgrade (A1); a FAILED probe never downgrades (WR-02) | ✓ VERIFIED | sync.go:80-90; client.go:163-187 (bool, error) with isTagUnsupportedErr classifier; TestRunSyncDowngradesToConfirmedLoudlyWhenTagUnsupported + TestRunSyncFailsLoudlyWhenTagProbeFails + TestSupportsHeadTagTrueAndFalse PASS |
| 16 | Rewind subcommand is the only path that deletes events; requires explicit invocation with --to/--yes; prints FINAL count/sum before and after (D-03) | ✓ VERIFIED | grep: DeleteEventsFrom/DeleteCheckpointAbove called only from internal/indexer/rewind.go, which is called only from cmd/indexer/main.go rewindCommand (stdin "yes" prompt unless --yes); printRewindReport renders before/after totals; TestRewindWalksToCommonAncestorAndExcludesOrphans PASS |
| 17 | After rewind to N: zero FINAL rows above N; checkpoint re-anchors at N with canonical hash; at/below N untouched | ✓ VERIFIED | TestRewindDeleteExcludesOrphansImmediately PASS (real ClickHouse, this pass) + TestRewindReanchorWinsOnRealStore PASS incl. after OPTIMIZE TABLE indexer_checkpoint FINAL (CR-01 proof, this pass) |
| 18 | Re-included tx events (same tx_hash/log_index, new block_hash/amount) after rewind read exactly once with NEW values pre-merge and post-OPTIMIZE | ✓ VERIFIED | TestRewindSameIdentityReinsertNotSwallowed PASS on real ClickHouse (this pass); TestRewindSameIdentityReinsertPath PASS (harness) |
| 19 | Ancestor walk bounded (1000); rewind point is first agreeing height or first row-free height (implemented: row-free terminates only when no stored rows remain at or below — documented deviation, strictly safer) | ✓ VERIFIED | rewind.go:21 (DefaultRewindMaxWalk 1000), :140-177 walk with ErrRewindWalkTooDeep; deviation caught by TestRewindSameIdentityReinsertPath pre-commit and pinned by it |
| 20 | --to override disagreeing with stored rows at that height rejected, non-zero exit, nothing deleted (verified rewind only) | ✓ VERIFIED | rewind.go:179-192 ErrRewindToMismatch; TestRewindToOverrideRejectedOnDisagreement PASS |
| 21 | Devnet: cancelled mid-catch-up run re-runs with gap-free continuity and FINAL-stable totals; real TetherToken bytecode yields classified Issue/Redeem rows surviving replay (D-02) | ✓ VERIFIED | Live re-run this pass against the Kurtosis enclave (chainId 0x301824) + real ClickHouse: TestDevnetFollowsFinalizedHeadAcrossRestarts PASS (4.40s, recordingStore asserts max checkpoint advance <= window_blocks), TestDevnetUSDTLifecycle PASS (802.59s, exit 0) |
| 22 | Continuous Mainnet run follows finalized-head advances >= 15 minutes without block gaps (backstop, manual operator observation) | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Code + loop proven by truths 1-2, 21; the 15-minute live Mainnet observation is the plan's own `verification: backstop` item, tracked OPEN in .planning/WINDOWS.md — needs the operator at the phase gate (see Human Verification) |

**Score:** 21/22 truths verified (1 present, behavior-unverified — the deliberate manual backstop)

### Prohibitions (must-NOT checks — all hold)

| Prohibition | Status | Evidence |
|---|---|---|
| Replay of an already-checkpointed range must never move the checkpoint (02-01) | ✓ HOLDS | sync.go writes checkpoints only at windowEnd > cpHeight after accepted ingest; no lower-height write path exists in the loop; TestRunSyncCrashBetweenInsertAndCheckpoint asserts checkpoint unmoved across replay |
| Provider without finalized-tag support must never be silently treated as finalized-safe (02-02) | ✓ HOLDS | sync.go:80-90 — every finalized run passes the probe; silent path impossible: either the loud downgrade log fires or the run fails; TestRunSyncFailsLoudlyWhenTagProbeFails asserts no silent downgrade |
| A checkpoint hash mismatch must never trigger an automatic rewind (02-03) | ✓ HOLDS | grep: zero `Rewind` references in internal/indexer/sync.go; Rewind reachable only from cmd/indexer/main.go rewindCommand (operator-typed subcommand + confirmation) |
| The rewind must never delete events at or below the verified rewind point (02-03) | ✓ HOLDS | DeleteEventsFrom predicate is exactly `block_number > ?`, DeleteCheckpointAbove `height > ?` (clickhouse.go:189-217); TestRewindWalksToCommonAncestorAndExcludesOrphans + TestRewindReanchorWinsOnRealStore assert rows at/below the point survive |

These are judgment-tier prohibitions with deterministic structural evidence (grep-ex absence + SQL predicate shape) plus wired passing tests; flagged for human confirmation alongside the backstop at the phase gate.

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| internal/indexer/sync.go | RunSync loop, ChainReader/EventStore seams, CheckpointMismatchError | ✓ VERIFIED (wired) | 244 lines; consumed by cmd/indexer + devnet tests; tool check 6/6 pass |
| internal/storage/clickhouse.go | WriteCheckpoint/ReadCheckpoint ordered read | ✓ VERIFIED (wired) | ReadCheckpoint exactly `ORDER BY height DESC LIMIT 1`, no FINAL in checkpoint read |
| migrations/clickhouse.sql | indexer_checkpoint ReplacingMergeTree(height) DDL | ✓ VERIFIED | `ENGINE = ReplacingMergeTree(height) ORDER BY (chain)`; applied by EnsureSchema (idempotence test PASS on real server) |
| internal/rpc/client.go | EligibleHead, FetchLogsResilient, SupportsHeadTag, Close, redaction | ✓ VERIFIED (wired) | 380 lines; all exports present and consumed |
| internal/config/config.go | head_policy/window_blocks/window_floor/poll_seconds/start_block + validation | ✓ VERIFIED | defaults in Load, policy-scoped confirmation_blocks, IN-08 duplicate-contract guard |
| cmd/indexer/main.go | run (default) + rewind subcommands | ✓ VERIFIED | CLI spot-checks PASS (run/rewind/unknown-exit-2, ETH_RPC_URL fail-fast) |
| internal/indexer/rewind.go | Rewind, RewindOpts, RewindReport, RewindStore | ✓ VERIFIED (wired) | 246 lines; called from rewind subcommand |
| internal/indexer/reorg_test.go | two-chain harness + mismatch/rewind tests | ✓ VERIFIED | 4 named tests PASS |
| internal/indexer/devnet_test.go | env-gated devnet suite | ✓ VERIFIED | both tests PASS live this pass |
| testdata/usdt_creation.hex | 23,800 hex chars | ✓ VERIFIED | measured exactly 23,800 |
| COVERAGE.md | Phase 2 API matrix | ✓ VERIFIED | finalized/safe tags INTEGRATE; every OPT-OUT row reasoned |

Artifact tool verdicts: 02-01 6/6, 02-02 2/2, 02-03 7/7 — all passed, no stubs, no issues.

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| cmd/indexer/main.go | internal/indexer/sync.go | run calls RunSync | ✓ WIRED | Pattern found (tool) + CLI spot-check reaches RunSync |
| internal/indexer/sync.go | internal/storage/clickhouse.go | InsertEvents -> WriteCheckpoint order | ✓ WIRED | Pattern found; ordering pinned by crash test |
| internal/storage/clickhouse.go | migrations/clickhouse.sql | ordered checkpoint read | ✓ WIRED | Pattern found |
| internal/indexer/sync.go | internal/rpc/client.go | FetchLogsResilient through the seam | ✓ WIRED | Pattern found; RunSync signature byte-identical to 02-01 (`func RunSync(ctx context.Context, cr ChainReader, es EventStore, cfg *config.Config) error`) |
| cmd/indexer/main.go | internal/rpc/client.go | tag probe + Close wiring | ✓ WIRED | Tag-probe consumption lives inside RunSync (documented 02-02 deviation for parallel-plan file ownership); Close deferred in both subcommands — loud-downgrade behavior test-proven |
| cmd/indexer/main.go | internal/indexer/rewind.go | rewind subcommand -> Rewind | ✓ WIRED | Pattern found; non-zero exit on failure |
| internal/indexer/rewind.go | internal/storage/clickhouse.go | StoredBlockHashes/DeleteEventsFrom/re-anchor | ✓ WIRED | Pattern found; CR-01 DeleteCheckpointAbove in destructive section |
| internal/indexer/devnet_test.go | internal/indexer/sync.go | devnet drives RunSync | ✓ WIRED | Proven live this pass |

### Data-Flow Trace (Level 4)

Not a rendering phase (no UI). The dynamic-data chains verified instead: checkpoint (provider header hash -> WriteCheckpoint -> ReadCheckpoint -> window bounds), events (FetchLogsResilient -> DecodeSupplyEvent -> InsertEvents -> FINAL reads in Inspect/RangeTotals/StoredBlockHashes), rewind report (RangeTotals before/after -> tabwriter stdout). All chains end in real queries/headers — no static or mocked sources outside test fakes. ✓ FLOWING.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Offline full suite (build+vet+test) | `env -u ETH_RPC_URL -u CLICKHOUSE_URL -u DEVNET_RPC_URL go build/vet/test ./... -count=1` | all ok (indexer 6.7s incl. production-pace backoff proof) | ✓ PASS |
| ClickHouse integration suites | `CLICKHOUSE_URL=... go test ./internal/storage ./internal/indexer -count=1` | 11 storage tests + full indexer suite ok | ✓ PASS |
| CR-01 real-store rewind proof | `go test ./internal/indexer -run TestRewindReanchorWinsOnRealStore -v` | PASS (0.69s) | ✓ PASS |
| Reorg/rewind/sync behavior suite | `go test ./internal/indexer -run 'Mismatch\|Rewind\|RunSync' -v` | 18/18 PASS | ✓ PASS |
| Devnet suite (live enclave, chainId 0x301824) | `DEVNET_RPC_URL=... go test ./internal/indexer -run TestDevnet -v -timeout 20m` | restart 4.40s PASS, lifecycle 802.59s PASS, exit 0 | ✓ PASS |
| CLI fail-fast + subcommand dispatch | `env -u ETH_RPC_URL go run ./cmd/indexer [run\|rewind\|bogus]` | ETH_RPC_URL message, unknown-subcommand exit path | ✓ PASS |
| Fixture gate | `tr -d '\n' < testdata/usdt_creation.hex \| wc -c` | 23800 | ✓ PASS |

### Probe Execution

No `scripts/*/tests/probe-*.sh` probes declared by the plans; verification is the go-test suites above (all executed this pass, not narrated).

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| SYNC-01 | 02-01, 02-02 | Bounded ranges through configured head, continuously | ✓ SATISFIED | Truths 1-2, 5, 10-15 + devnet restart test; the 15-min Mainnet observation is the tracked manual backstop (truth 22) |
| SYNC-02 | 02-02 | Retries without skipping a range or advancing checkpoint | ✓ SATISFIED | Truths 10-13 |
| SYNC-03 | 02-01, 02-03 | Durable checkpoint resumes; advances only after accept | ✓ SATISFIED | Truths 3-6, 21 |
| SYNC-04 | 02-01 | Replay changes nothing, pre-merge included | ✓ SATISFIED | Truths 6-7 |
| SYNC-05 | 02-03 (+02-01 seed) | Mismatch detection + verified rewind excluding orphans | ✓ SATISFIED | Truths 8-9, 16-20. "Rebuilds affected derived data" is vacuous in Phase 2 (no derived tables exist); the Phase 4 seam is documented at rewind.go:231-232 |

Orphaned requirements: none — REQUIREMENTS.md maps exactly SYNC-01..05 to Phase 2 and the three plans' `requirements` fields cover all five (union: 02-01 [SYNC-01,03,04], 02-02 [SYNC-02,01], 02-03 [SYNC-05,03]).

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | - | Zero TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER markers across all 19 phase files; no placeholder text; no stub returns; no credentials tracked (only a docs.infura.io documentation URL in a comment) | - | - |

### Code Review Weighing (CR-01 + WR-01..03 vs must_haves)

All four in-scope findings are fixed at HEAD (commits 1f50cf9, 11dac89, d6a4775, c96420e — all ancestors of HEAD) and re-proven by this pass's own test runs, including the red-checked TestRewindReanchorWinsOnRealStore on the real server. CR-01 in particular closes a real hole in must-have "checkpoint re-anchors at N" (the re-anchor was previously invisible to the ordered read — the recovery path could never complete on the real store); with DeleteCheckpointAbove in the destructive section the truth now holds on real ClickHouse. The 8 skipped Info findings remain open hygiene items (README §4 staleness, CWD-relative paths, zero-value coercion, interrupt-in-backoff reporting, window==0 guard, idle-loop verification skip, plan/execute drift, inspect chain predicate) — none defeats a must-have; the idle-verification skip is the plan's own research-backed resolution (Open Question 1).

### Documented Deviations Weighed

1. Ancestor-walk termination (row-less height terminates only when no rows at or below) — strictly safer than the plan text, pinned by TestRewindSameIdentityReinsertPath. Accepted.
2. Devnet key single-nibble repair — test-only fixture constant, cryptographically derived; no production impact. Accepted.
3. Lifecycle test at confirmed/64 for the 20-minute budget — finalized-following separately proven by the restart test (finalized policy, PASS this pass); the must-have's lifecycle clause does not demand finalized. Accepted.
4. Eligible-head pre-wait in the lifecycle test — test-only. Accepted.
5. go.mod/go.sum indirect entries (x/crypto, fsnotify) — transitive of the pinned go-ethereum v1.17.7; no new modules; consistent with T-02-SC. Accepted.

### Human Verification Required

1. **15-minute continuous Mainnet run (backstop truth — closes WINDOWS.md open item)**
   **Test:** Run `go run ./cmd/indexer run` with ETH_RPC_URL for >= 15 minutes and watch the per-window progress logs.
   **Expected:** Gap-free monotonic window/checkpoint advances tracking the finalized head; no errors; no manual babysitting.
   **Why human:** The plan marks this truth `verification: backstop` (manual operator observation, 02-VALIDATION.md manual table); the verifier cannot start it without writing to the operator's real ClickHouse data.
2. **Induced-mismatch operator UX walkthrough (02-VALIDATION.md manual table)**
   **Test:** Corrupt/replace the stored checkpoint hash via the documented harness, restart the indexer, then follow README section 5's rewind procedure end to end.
   **Expected:** Loud typed halt (height + both hashes, exit 1) before any ingest; `rewind` prints the before/after FINAL totals, requires explicit "yes", and recovery re-anchors at the verified ancestor.
   **Why human:** Destructive-ish live procedure; deterministic halves already test-proven, the operator walkthrough is manual by design.

### Gaps Summary

No failed truths, no missing/stub artifacts, no broken key links, no debt markers, no credentials. All 15 artifacts pass at all four levels; all 8 key links wired; offline, ClickHouse-integration, reorg, and devnet suites green in this verifier's own runs; all four code-review fixes present and re-proven. The single non-verified item is the plan's own declared manual backstop (15-minute Mainnet continuous-run observation), tracked open in .planning/WINDOWS.md — it routes the phase to the human phase-gate checkpoint rather than indicating missing work.

---

_Verified: 2026-10-10T11:05:00Z_
_Verifier: Claude (gsd-verifier)_
