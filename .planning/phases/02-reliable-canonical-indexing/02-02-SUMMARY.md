---
phase: 02-reliable-canonical-indexing
plan: "02"
subsystem: ingestion
tags: [go, ethereum, go-ethereum, rpc-retry, error-classification, result-cap, rate-limit, backoff, window-halving, credential-redaction, finalized-tag]

requires:
  - phase: 02-reliable-canonical-indexing
    plan: "01"
    provides: RunSync loop, ChainReader/EventStore seams, EligibleHead, config head_policy/window_blocks/window_floor, checkpoint ordering
provides:
  - internal/rpc isResultCapErr (message-level -32005 result-cap/timeout discrimination) + narrowed isRateLimitErr (throttle shapes only: 429, Too Many Requests, Rate limit exceeded, temporarily unavailable)
  - internal/rpc Client.FetchLogsResilient(ctx, token, topics, from, to, window, floor) — D-05 deterministic sub-range halving, fail-closed window_floor, stateless per-call reset
  - internal/rpc Client.SupportsHeadTag(ctx) bool (A1 probe) + Client.Host() + Client.Close() (IN-08)
  - internal/rpc scrubURL/scrub — AR-01 residual closed: every %w-wrapped provider error renders credential-safe (raw URL and net/http's masked user:xxxxx@ form collapse to scheme://host) while preserving the errors.Is cancellation bit
  - internal/indexer ChainReader.FetchLogsResilient + SupportsHeadTag + Host; RunSync fetch path swapped and loud finalized->confirmed downgrade added (RunSync signature byte-identical to 02-01)
  - span-recording cannedRPC/cannonRPC failure injection (cap, throttle, tag rejection) + nine new discrimination/redaction/probe tests
affects: [02-reliable-canonical-indexing, 03-supply-anchoring]

actuals:
  tokens: 10961   # chars/4 over the realized diff (43844 chars, 4 files)
  tasks: 3
  commits: 3      # measured: git rev-list --count 3a67ea1..HEAD

plan_head_before: 3a67ea103eefb95ca4f7faaa317ae7676962c052
plan_head_after: 0849aa9f00e0b104f35165f3ddab2776400a207e

tech-stack:
  added: []       # zero new dependencies; stdlib regexp/net-url only
  patterns: [message-level error classification ordered cap-before-throttle, deterministic halving walk with fail-closed floor, text-rewriting error scrub that preserves the errors.Is cancellation signal via a custom Is method]

key-files:
  created: []
  modified:
    - internal/rpc/client.go
    - internal/rpc/client_test.go
    - internal/indexer/sync.go
    - internal/indexer/sync_test.go

key-decisions:
  - "-32005 is discriminated by message text only (Infura documents the result cap and the query timeout under the same code): \"more than 10000 results\" / \"query timeout exceeded\" halve the sub-range; \"429\" / \"Too Many Requests\" / \"Rate limit exceeded\" / \"temporarily unavailable\" keep bounded backoff — the bare -32005 disjunct is gone from isRateLimitErr so a cap can never land in backoff anywhere"
  - "Halving keeps the SAME starting block and no backoff (the query was rejected, not throttled); the sub-range size persists within one FetchLogsResilient call and resets to the passed window on every call (D-05 stateless)"
  - "The floor fails closed with a named error carrying window_floor N and the blocked sub-range — a range that cannot be fetched is never skipped, and RunSync stops rather than fetching any later window"
  - "scrubURL rewrites error TEXT (raw URL -> HostOnly; scheme://userinfo@host regex catches net/http's masked user:xxxxx@ form) and returns a scrubbedError whose Is method carries only the cancellation bit, so cmd/indexer's errors.Is(err, context.Canceled) SIGINT contract survives redaction"
  - "SupportsHeadTag is consumed inside RunSync (not cmd/indexer/main.go, owned by parallel plan 02-03); the loud slog.Error names the provider host via a new ChainReader.Host() seam"

patterns-established:
  - "Cap-before-throttle classification order at every fetch error site (T-02-07)"
  - "Fail-closed boundary errors name the policy knob and the blocked range (mirrors CheckpointMismatchError's loud-diagnostic style)"
  - "Canned-RPC harnesses record (fromBlock, toBlock) per eth_getLogs so retry/halving behavior is assertable offline"

requirements-completed: [SYNC-02, SYNC-01]

coverage:
  - id: D1
    description: "The three -32005 shapes are discriminated by message with cap-before-throttle ordering; halving is deterministic and resets per call; the floor fails closed with a named error; throttles retry the same range under bounded backoff"
    requirement: SYNC-02
    verification:
      - kind: unit
        ref: "env -u ETH_RPC_URL -u CLICKHOUSE_URL go test ./internal/rpc -run 'ResultCap|HalvesRange|QueryTimeout|Throttle|Floor|RateLimitExceeded' -v -count=1 — TestResultCapHalvesRangeAndSucceeds, TestQueryTimeoutTreatedAsResultCap, TestThrottleBacksOffSameRange, TestFloorTripFailsClosed, TestRateLimitExceededMessageIsThrottle RUN and PASS"
      - kind: build
        ref: "go vet ./... exit 0; grep shows 'more than 10000 results' only inside isResultCapErr and no bare -32005 disjunct in isRateLimitErr"
    status: pass
    human_judgment: false
  - id: D2
    description: "Sync loop retries never skip or advance: throttle completes the window with identical bounds and one checkpoint write; caps halve inside the window and still advance; floor trip stops the run with zero checkpoint writes and no later window fetched"
    requirement: SYNC-02
    verification:
      - kind: unit
        ref: "env -u ETH_RPC_URL -u CLICKHOUSE_URL go test ./internal/indexer -run 'ThrottleRetries|CapHalves|FloorTrip' -v -count=1 — TestRunSyncThrottleRetriesSameWindowWithoutAdvancing (real 2s+4s production backoffs), TestRunSyncCapHalvesWithinWindowAndAdvances, TestRunSyncFloorTripStopsRunCheckpointUnmoved RUN and PASS"
      - kind: build
        ref: "RunSync signature byte-identical to 02-01 (func RunSync(ctx context.Context, cr ChainReader, es EventStore, cfg *config.Config) error); full offline suite exit 0"
    status: pass
    human_judgment: false
  - id: D3
    description: "AR-01 residual closed: RPC errors returned to callers contain no URL credentials anywhere in the wrapped chain; un-credenentialed host rendering covers the constructed message and inner *url.Error text"
    requirement: SYNC-02
    verification:
      - kind: unit
        ref: "env -u ETH_RPC_URL -u CLICKHOUSE_URL go test ./internal/rpc -run Redact -v -count=1 — TestWrappedErrorsRedactCredentials dials a userinfo-bearing URL, forces transport failures on getLogs/header, asserts host present and userinfo/full-URL/masked-password absent; cancellation errors.Is contract pinned"
    status: pass
    human_judgment: false
  - id: D4
    description: "A1 handled: missing finalized-tag support downgrades to confirmed only after a loud startup warning naming the provider host (never silent); IN-08 Close is safe including double close"
    requirement: SYNC-01
    verification:
      - kind: unit
        ref: "env -u ETH_RPC_URL -u CLICKHOUSE_URL go test ./internal/rpc -run 'SupportsHeadTag|Close' and ./internal/indexer -run DowngradesToConfirmedLoudly -v -count=1 — TestSupportsHeadTagTrueAndFalse (params carry the finalized tag string), TestCloseIsSafe, TestRunSyncDowngradesToConfirmedLoudlyWhenTagUnsupported (captured slog handler asserts the loud downgrade; silent fails) RUN and PASS"
    status: pass
    human_judgment: false
  - id: D5
    description: "Whole-suite regression: the full offline suite stays green with the loop switched onto FetchLogsResilient and redaction applied at every wrap site"
    requirement: SYNC-02
    verification:
      - kind: unit
        ref: "env -u ETH_RPC_URL -u CLICKHOUSE_URL go test ./... -count=1 exit 0 (config, indexer incl. 02-01 suite, rpc incl. Phase 1 suite, storage)"
      - kind: build
        ref: "go build ./... && go vet ./... exit 0; no credential in tracked files (diff scan clean)"
    status: pass
    human_judgment: false

duration: 33min
completed: 2026-10-09T11:01:00Z
status: complete
---

# Phase 2 Plan 02: Failure Discrimination — Fetch Hardening Summary

One-liner: message-level -32005 discrimination (result-cap/timeout halve deterministically to a fail-closed floor; throttles keep bounded backoff on the same range) wired through RunSync's resilient fetch, plus the AR-01 wrapped-error credential scrub and the A1 finalized-tag probe with a loud confirmed downgrade.

## Performance

- 3 tasks, 3 commits, 33 minutes (estimate 32k tokens / actual ~11k tokens over a 4-file, 43,844-char diff; low-confidence estimate again came in lean — the 02-01 seams and harnesses lifted into the new tests almost verbatim)
- Offline suite: `env -u ETH_RPC_URL -u CLICKHOUSE_URL go test ./... -count=1` green (indexer suite ~6.7s of that is the deliberately unshortened production backoff proof)
- go vet clean; go.mod/go.sum untouched (zero new dependencies — stdlib regexp/net/url only)

## Accomplishments

- **SYNC-02** — the three `-32005` shapes are now distinct: `"more than 10000 results"` and `"query timeout exceeded"` (Infura documents both under that code) shrink the sub-range with an immediate same-start retry; `"429"`/`"Too Many Requests"`/`"Rate limit exceeded"`/`"temporarily unavailable"` keep the Phase 1 bounded backoff on the identical range; neither classification ever advances or skips a window.
- **D-05 exactly** — `FetchLogsResilient` walks `[from, to]` in sub-ranges from `min(window, remaining)`, halves on caps down to `window_floor`, fails closed there with a named error carrying the floor value and the blocked range, and resets the size per call (no halving state leaks across outer windows).
- **Loop integration** — `RunSync` fetches through `FetchLogsResilient(ctx, token, topics, from, to, cfg.WindowBlocks, cfg.WindowFloor)`; signature and advance-after-accept ordering are byte-identical to 02-01 (parallel plan 02-03 unaffected); floor trip stops the run with zero checkpoint writes and no later window fetched.
- **AR-01 residual closed** — `scrubURL` renders every `%w`-wrapped provider error credential-safe at all eight wrap sites in `internal/rpc`; the raw URL and net/http's masked `user:xxxxx@` form both collapse to `scheme://host`; the cancellation signal survives via `scrubbedError.Is` so the SIGINT contract (`errors.Is(err, context.Canceled)`) still holds.
- **A1** — `SupportsHeadTag` probes the finalized tag at RunSync start; unsupported providers downgrade to confirmed only after a loud `slog.Error` naming the provider host (via the new `ChainReader.Host()` seam) — never silently.
- **IN-08 (client half)** — `Client.Close()` releases the transport and tolerates double close; the run-command wiring belongs to plan 02-03, which owns `cmd/indexer/main.go` this wave.

## Task Commits

| Task | Commit | Subject |
|------|--------|---------|
| 1 — -32005 split + FetchLogsResilient with fail-closed floor | `952becc` | feat(02-02): -32005 discrimination + resilient windowed fetch with fail-closed floor |
| 2 — sync loop onto FetchLogsResilient | `6d3c7f5` | feat(02-02): sync loop fetches through FetchLogsResilient — retries never skip or advance |
| 3 — redaction + tag probe + Close | `0849aa9` | feat(02-02): rpc hardening — credential redaction, finalized-tag probe, Close |

## Files

Modified: `internal/rpc/client.go`, `internal/rpc/client_test.go`, `internal/indexer/sync.go`, `internal/indexer/sync_test.go`.

## Decisions

(Recorded in frontmatter key-decisions; the load-bearing ones:)
- Discrimination is message-text-only because the JSON-RPC code is shared across all three failure modes; ordering cap-before-throttle is enforced structurally (cap messages can no longer match `isRateLimitErr` anywhere), not just at one call site.
- `scrubbedError` keeps the redacted text but carries a cancellation bit instead of the wrap chain — flattening to `errors.New` alone broke `errors.Is(err, context.Canceled)`, which `cmd/indexer`'s SIGINT handling depends on.
- The tag downgrade lives inside `RunSync`, not `cmd/indexer/main.go`, because 02-03 owns that file in this wave; `ChainReader` gained `Host()` so the loud log can name the provider without breaking the import-cycle seam.

## Deviations from Plan

**1. [Rule 1 - Bug] Error flattening in scrub broke the cancellation contract**
- **Found during:** Task 3 (full-suite run)
- **Issue:** the plan's string-transform returns a chain-less error; `errors.Is(err, context.Canceled)` then fails, so `cmd/indexer` would exit 1 with an error on every SIGINT instead of logging a clean stop (caught by TestRunSyncCapHalvesWithinWindowAndAdvances / TestRunSyncDowngradesToConfirmedLoudlyWhenTagUnsupported failing in the full suite).
- **Fix:** `scrubbedError` carries only a cancellation bit (from `errors.Is` on the pre-scrub error) exposed through an `Is` method; redacted text unchanged. Pinned by new assertions in TestWrappedErrorsRedactCredentials.
- **Files modified:** internal/rpc/client.go, internal/rpc/client_test.go
- **Verification:** full offline suite green; cancellation and non-cancellation cases asserted
- **Commit:** `0849aa9`

**2. [Rule 3 - Blocking] Canned `null` header made the tag probe always read unsupported**
- **Found during:** Task 3 (first run of TestSupportsHeadTagTrueAndFalse)
- **Issue:** `ethclient.HeaderByNumber` turns a JSON `null` result into a not-found error, and a minimal marshaled header missing `difficulty` fails `types.Header.UnmarshalJSON` ("missing required field 'difficulty'").
- **Fix:** the cannedRPC `eth_getBlockByNumber` branch now serves a real marshaled `types.Header` (UncleHash/TxHash/ReceiptHash/Difficulty/Number/GasLimit/Time), mirroring sync_test's cannonHeader.
- **Files modified:** internal/rpc/client_test.go
- **Verification:** TestSupportsHeadTagTrueAndFalse PASS (true path serves the tag; false path errors the method)
- **Commit:** `0849aa9`

**3. [In passing] gofmt alignment nit from 02-01**
- **Found during:** Task 3 (gofmt -l flagged internal/indexer/sync.go)
- **Issue:** `CheckpointMismatchError` struct field alignment was not gofmt-clean (pre-existing from 02-01; no hook caught it).
- **Fix:** `gofmt -w` on the file (whitespace only, no logic change).
- **Files modified:** internal/indexer/sync.go
- **Verification:** `gofmt -l internal/` clean; full suite green
- **Commit:** `0849aa9`

Otherwise the plan executed as written.

## Threat Mitigations Landed

- **T-02-03 (Information Disclosure, high)** — mitigate: `scrubURL` at all eight `%w` wrap sites; proven by TestWrappedErrorsRedactCredentials against a userinfo-bearing dial URL with forced transport failures (raw URL, userinfo pair, and masked-password forms all asserted absent; host asserted present).
- **T-02-07 (Denial of Service, medium)** — mitigate: `isResultCapErr` checked before `isRateLimitErr` with the bare `-32005` disjunct removed; deterministic halving with the fail-closed floor; proven by the Task 1 discrimination suite.
- **T-02-06 (low, accept)** — unchanged: devnet surface is 02-03's test-only scope.

## Issues

- None open. The plan's must_haves truth about a provider that "rejects the finalized tag" is proven against the canned rejecting server; the live-provider confirmation remains the phase-gate manual backstop already tracked for 02-01's D6 in 02-VALIDATION.md.

## User Setup

None. No new config keys, no credentials anywhere in tracked files (the `user:secretpass@` string in tests is a localhost canned-server fixture for the redaction proof, not a credential).

## Next Phase Readiness

Ready for 02-03 (rewind + devnet):
- `ChainReader` now exposes `FetchLogsResilient`/`SupportsHeadTag`/`Host` and `RunSync`'s signature is unchanged, so 02-03's RunSync calls and the `rewind` subcommand land without shape changes here.
- `Client.Close()` is available for the run command's defer (02-03 owns `cmd/indexer/main.go`).
- The span-recording harnesses and throttle/cap/tag-rejection injection extend directly to 02-03's two-chain reorg harness.

## Self-Check: PASSED

- Modified files exist: internal/rpc/client.go, internal/rpc/client_test.go, internal/indexer/sync.go, internal/indexer/sync_test.go — FOUND
- Commits are ancestors of HEAD: 952becc, 6d3c7f5, 0849aa9 — FOUND
- Commits measured 3 via `git rev-list --count 3a67ea1..HEAD` (matches per-task commits; no uncommitted code changes)
