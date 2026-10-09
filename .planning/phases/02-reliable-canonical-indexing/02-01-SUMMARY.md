---
phase: 02-reliable-canonical-indexing
plan: "01"
subsystem: ingestion
tags: [go, ethereum, go-ethereum, clickhouse, clickhouse-go, continuous-sync, checkpointing, finalized-head, replay-idempotence, reorg-detection]

requires:
  - phase: 01-live-supply-event-path
    provides: decoder, provider probe, rate-limit retry, ReplacingMergeTree storage, bounded CLI
provides:
  - internal/indexer/sync.go RunSync continuous window loop + ChainReader/EventStore seams + CheckpointMismatchError{Height, ExpectedHash, ObservedHash}
  - internal/rpc EligibleHead (finalized tag / confirmed latest-minus-depth) with retry and HostOnly wrapping; IN-03 empty-tokens Probe guard
  - internal/storage Store.WriteCheckpoint / Store.ReadCheckpoint (ordered read, never FINAL)
  - migrations/clickhouse.sql indexer_checkpoint ReplacingMergeTree(height) DDL; EnsureSchema now applies multi-statement migrations
  - internal/config head_policy/window_blocks/window_floor/poll_seconds/start_block with D-01/D-04/D-05 defaults, policy-scoped confirmation_blocks, IN-08 duplicate-contract guard
  - cmd/indexer `run` subcommand (default) with -from seed flag, SIGINT-aware context, loud mismatch halt
  - internal/indexer/sync_test.go canned-RPC + fakeEventStore offline suite (six TestRunSync* behaviors)
  - internal/storage ClickHouse integration proofs (ordered checkpoint read, crash-between-writes, replay totals across merges)
  - README continuous-run runbook
affects: [02-reliable-canonical-indexing, 03-supply-anchoring, 04-observability-deployment]

actuals:
  tokens: 16462   # chars/4 over the realized diff (65846 chars, 11 files)
  tasks: 2
  commits: 2      # measured: git rev-list --count e791c46..HEAD

plan_head_before: e791c46ba73ddda7583af055eb407523744ff511
plan_head_after: fc7b7a1c282ad568b72dc4984697dc1bef06df5a

tech-stack:
  added: []       # zero new dependencies (threat T-02-SC: go mod verify clean, go.mod/go.sum untouched)
  patterns: [structural interface seams to break rpc<->indexer import cycle, RLP-derived canned header hashes (go-ethereum v1.17.7 has no stored hash field), advance-after-accept two-write ordering, comment-aware SQL statement splitting for native-protocol migrations]

key-files:
  created:
    - internal/indexer/sync.go
    - internal/indexer/sync_test.go
  modified:
    - internal/config/config.go
    - internal/config/config_test.go
    - config/tokens.json
    - internal/rpc/client.go
    - internal/storage/clickhouse.go
    - internal/storage/clickhouse_test.go
    - migrations/clickhouse.sql
    - cmd/indexer/main.go
    - README.md

key-decisions:
  - "EligibleHead resolves the finalized head via ethclient.HeaderByNumber with the go-ethereum tag constant (the library serializes the tag itself; no hand-rolled JSON-RPC) and the confirmed head as latest minus confirmation_blocks clamped at 0"
  - "RunSync depends on ChainReader/EventStore structural seams because internal/rpc imports internal/indexer (SupplyTopics) and the reverse import would cycle; *rpc.Client and *storage.Store satisfy the seams structurally and the proof suite pins that in external package indexer_test"
  - "Checkpoint reads use ORDER BY height DESC LIMIT 1 — never FINAL — and ReplacingMergeTree(height) makes the merged survivor the greatest height even under out-of-order writes (probe-verified property now pinned by TestCheckpointOrderedReadPreMerge)"
  - "Canned-server header hashes are RLP-derived from deterministic header fields: go-ethereum v1.17.7 Header has no stored hash field, so Hash() is recomputed on unmarshal and the harness must serve field-consistent headers exactly like a real provider"
  - "EnsureSchema splits migrations into single statements before Exec: the ClickHouse native protocol rejects multi-statement requests, and the migration file's comment lines carry semicolons and apostrophes that a naive split would cut mid-comment"

patterns-established:
  - "Advance-after-accept ordering: InsertEvents nil -> WriteCheckpoint(window-end height, window-end header hash); never per-log inserts"
  - "Pre-window checkpoint verification (resume AND before each window ingest; skipped when idle at head) returning a typed CheckpointMismatchError that names height + both hashes"
  - "D-04 seeding: no checkpoint + start_block -> seed at that block's header hash; no checkpoint + no start_block -> seed at the current eligible head without ingesting it; windows always open at checkpoint+1"

requirements-completed: [SYNC-01, SYNC-03, SYNC-04]

coverage:
  - id: D1
    description: "Continuous loop follows the eligible head in bounded windows capped at head, idles at head without inserting or advancing"
    requirement: SYNC-01
    verification:
      - kind: unit
        ref: "env -u ETH_RPC_URL -u CLICKHOUSE_URL go test ./internal/indexer -count=1 — TestRunSyncFollowsHeadAcrossTwoWindows, TestRunSyncIdleAtHeadSleepsWithoutAdvancing PASS"
      - kind: build
        ref: "go build ./... && go vet ./... exit 0"
        status: pass
    human_judgment: false
  - id: D2
    description: "Durable (height, hash) checkpoint resumes after restart; advances only after the window's InsertEvents is acked; empty windows still advance; crash-between-writes replays collapse to one logical set"
    requirement: SYNC-03
    verification:
      - kind: unit
        ref: "go test ./internal/indexer -count=1 — TestRunSyncEmptyWindowStillAdvancesCheckpoint, TestRunSyncCrashBetweenInsertAndCheckpoint PASS"
      - kind: integration
        ref: "CLICKHOUSE_URL=clickhouse://default@127.0.0.1:9000/default go test ./internal/storage -run 'Checkpoint|CrashBetween|Replay' -v -count=1 — TestCheckpointOrderedReadPreMerge, TestCrashBetweenWritesReplayIsIdempotent PASS on pinned 26.8.20.9"
        status: pass
    human_judgment: false
  - id: D3
    description: "Replaying processed ranges (including wider ones) leaves FINAL count and sum byte-identical, pre-merge and post-OPTIMIZE"
    requirement: SYNC-04
    verification:
      - kind: integration
        ref: "CLICKHOUSE_URL=... go test ./internal/storage -run Replay -v -count=1 — TestReplayTotalsStableBeforeAndAfterOptimize (sub-range and wider-range totals), TestReplayDuplicateInsertKeepsFinalCountAtOne PASS"
        status: pass
    human_judgment: false
  - id: D4
    description: "D-01/D-04 config surface (head_policy enum, policy-scoped confirmation_blocks, window/floor/poll bounds, start_block seeding) and IN-08 duplicate-contract guard"
    requirement: SYNC-01
    verification:
      - kind: unit
        ref: "go test ./internal/config -count=1 — TestValidateHeadPolicyEnum, TestValidateDuplicateContractRejected, TestValidateSymbolFailFast PASS; TestRunSyncSeedsAtStartBlockAndAtHeadPerD04 PASS"
        status: pass
    human_judgment: false
  - id: D5
    description: "Operator surface: `run` subcommand with -from seed, SIGINT-aware stop, loud CheckpointMismatchError halt; README runbook with new keys"
    requirement: SYNC-01
    verification:
      - kind: cli
        ref: "env -u ETH_RPC_URL go run ./cmd/indexer run 2>&1 | grep -q 'ETH_RPC_URL is not set' — fail-fast wiring intact"
      - kind: grep
        ref: "README.md documents head_policy/window_blocks/window_floor/poll_seconds/start_block + run subcommand; ETH_RPC_URL placeholder only; no credential in tracked files"
        status: pass
    human_judgment: false
  - id: D6
    description: "Backstop truth: a continuous Mainnet run follows finalized-head advances for at least 15 minutes without block gaps"
    requirement: SYNC-01
    verification:
      - kind: manual
        ref: "02-VALIDATION.md manual table — operator observation at phase gate (must_haves marks this truth verification: backstop); not automatable in this plan"
        status: pending
    human_judgment: true

duration: 38min
completed: 2026-10-09T10:18:06Z
status: complete
---

# Phase 2 Plan 01: Tracer — Continuous Checkpointed Sync Summary

One continuous, crash-safe ingest path through every layer: config(head_policy) -> EligibleHead(finalized tag) -> durable (height, hash) checkpoint -> bounded window loop with strict advance-after-accept -> typed mismatch halt -> FINAL-idempotent replay proofs on the pinned ClickHouse.

## Performance

- 2 tasks, 2 commits, 38 minutes (estimate 42k tokens / actual ~16.5k tokens over an 11-file, 65,846-char diff; confidence was rated low, the slice came in leaner because Phase 1's fetch/decode/insert machinery lifted into the loop almost verbatim)
- Offline suite: `env -u ETH_RPC_URL -u CLICKHOUSE_URL go test ./... -count=1` green
- ClickHouse integration: 4/4 matching tests PASS on pinned 26.8.20.9
- go vet clean; go mod verify clean; go.mod/go.sum untouched (zero new dependencies)

## Accomplishments

- **SYNC-01** — `RunSync` resolves the eligible head per `head_policy` (D-01: finalized tag via go-ethereum's own tag serialization; confirmed = latest minus confirmation_blocks), ingests windows of `window_blocks` capped at the head, and sleeps `poll_seconds` when caught up without inserting, advancing, or erroring.
- **SYNC-03** — durable checkpoint in `indexer_checkpoint` (ReplacingMergeTree(height), ordered read); advances only after the window's `InsertEvents` returns nil; crash-between-writes leaves the window un-checkpointed and the restart re-processes it exactly once logically.
- **SYNC-04** — replay of processed ranges (and wider superset ranges) leaves FINAL count and sum byte-identical before merges and after `OPTIMIZE TABLE ... FINAL`.
- **SYNC-05 seed** — resume-time and pre-window checkpoint hash verification returns `*indexer.CheckpointMismatchError{Height, ExpectedHash, ObservedHash}` before any ingest; `cmd/indexer` halts loudly (exit 1) instructing the operator to run the rewind subcommand (D-03; the subcommand itself lands in plan 02-03).
- **D-04** — first run seeds at `start_block` (or the `-from` flag) when set, otherwise at the current eligible head, indexing forward only; nothing before the seed is ever fetched.
- **IN-03/IN-08** — Probe guards an empty token list; Validate rejects duplicate contract addresses case-insensitively (double-counting risk).
- README runbook rewritten for the continuous `run` invocation with the new config keys; bounded `-from/-to` mode documented as superseded.

## Task Commits

| Task | Commit | Subject |
|------|--------|---------|
| 1 — tracer: continuous checkpointed sync through every layer | `33a62b6` | feat(02-01): continuous checkpointed sync through the configured head |
| 2 — ClickHouse proof: ordered read, crash recovery, replay totals | `fc7b7a1` | test(02-01): ClickHouse proofs — ordered checkpoint read, crash recovery, replay totals |

## Files

Created: `internal/indexer/sync.go`, `internal/indexer/sync_test.go`.
Modified: `internal/config/config.go`, `internal/config/config_test.go`, `config/tokens.json`, `internal/rpc/client.go`, `internal/storage/clickhouse.go`, `internal/storage/clickhouse_test.go`, `migrations/clickhouse.sql`, `cmd/indexer/main.go`, `README.md`.

## Decisions

(Recorded in frontmatter key-decisions; the load-bearing ones:)
- Structural `ChainReader`/`EventStore` seams break the rpc<->indexer import cycle; the sync loop stays concrete-free and plans 02-02/02-03 extend it without shape changes.
- The offline harness derives header hashes from header fields (RLP), because v1.17.7 recomputes `Hash()` on unmarshal — the canned chain therefore behaves exactly like a real provider, including for the mismatch test (a time-field override yields a genuinely different canonical hash).
- EnsureSchema now applies migrations statement-by-statement (native protocol multi-statement rejection; comments carry `;` and `'`).

## Deviations from Plan

**1. [Rule 3 - Blocking] EnsureSchema could not Exec the two-statement migration file**
- **Found during:** Task 2 (ClickHouse verification)
- **Issue:** the native protocol rejects multi-statement requests ("Multi-statements are not allowed"); additionally the Phase 1 header comment contains both a semicolon and an apostrophe ("ledger value; the spec's"), so a naive semicolon split cut the comment in half and poisoned the parser.
- **Fix:** `EnsureSchema` drops comment lines and splits the DDL on `;`, executing each statement; `migrations/clickhouse.sql` gained statement terminators. The migration convention is now full-line `--` comments only.
- **Files modified:** `internal/storage/clickhouse.go`, `migrations/clickhouse.sql`
- **Verification:** full ClickHouse suite green (schema applies, is idempotent — TestEnsureSchemaIsIdempotent PASS)
- **Commit:** `fc7b7a1`

**2. [Rule 3 - Blocking] Existing config test fixture broke under the new Validate invariants**
- **Found during:** Task 1 (first offline run)
- **Issue:** `validConfig()` in `internal/config/config_test.go` omitted the new required fields, so Validate rejected it (head_policy "" etc.).
- **Fix:** fixture extended with HeadPolicy/WindowBlocks/WindowFloor/PollSeconds; added `TestValidateHeadPolicyEnum` and `TestValidateDuplicateContractRejected` to give the plan's acceptance criteria runnable proof.
- **Files modified:** `internal/config/config_test.go`
- **Verification:** `go test ./internal/config -count=1` PASS
- **Commit:** `33a62b6`

**3. [Rule 1 - Bug] sync_test.go must be an external test package**
- **Found during:** Task 1 (first build)
- **Issue:** the plan placed the compile-time seam assertions and the canned harness in `internal/indexer`'s internal test file, but a same-package test importing `internal/rpc` recreates the rpc->indexer cycle ("import cycle not allowed in test").
- **Fix:** the suite lives in `package indexer_test` (external), qualifying `indexer.RunSync`/seams; local copies of the `word32`/token helpers replace same-package fixtures.
- **Files modified:** `internal/indexer/sync_test.go`
- **Verification:** build + six TestRunSync* PASS
- **Commit:** `33a62b6`

Otherwise the plan executed as written.

## Issues

- None open for this plan. The backstop truth (15-minute continuous Mainnet run) is an operator manual-table item at the phase gate, tracked in coverage D6 as pending — it is not automatable here and does not block 02-02/02-03.

## User Setup

No new setup beyond the README runbook (`ETH_RPC_URL` env placeholder, ClickHouse container, `go run ./cmd/indexer run`). No credentials in any tracked file.

## Next Phase Readiness

Ready for 02-02 and 02-03:
- 02-02 (failure discrimination) extends `internal/rpc` with the `-32005` message split and window halving against `WindowFloor` (already validated and plumbed through config, unused by the loop until halving lands) and reuses the `cannonRPC` harness in `sync_test.go`.
- 02-03 (rewind + devnet) consumes `CheckpointMismatchError`, the `EventStore` seam (add `DeleteEventsFrom`), and the `run`-dispatch shape in `cmd/indexer` for the `rewind` subcommand; the mismatch-halt log line already names the rewind instruction.

## Self-Check: PASSED

- Created files exist: internal/indexer/sync.go, internal/indexer/sync_test.go — FOUND
- Commits are ancestors of HEAD: 33a62b6, fc7b7a1 — FOUND
- SUMMARY commits measured 2 via `git rev-list --count e791c46..HEAD` (matches per-task commits; no uncommitted code changes)
