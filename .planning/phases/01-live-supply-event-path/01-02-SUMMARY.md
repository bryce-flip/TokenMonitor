---
phase: 01-live-supply-event-path
plan: "02"
subsystem: verification
tags: [go, ethereum, live-rpc, eth-getlogs, infura-free-tier, rate-limit-retry, clickhouse, runbook, usdt, usdc]

requires:
  - phase: 01-01
    provides: decoder, provider probe, ClickHouse storage path, bounded ingest CLI
provides:
  - internal/indexer/live_test.go — env-gated live proof of the five pinned Mainnet fixtures (exact type/amount/provenance), the paired Mint/Burn no-double-count invariant, and the USDT-Transfer negative path, all through the operator's provider
  - internal/rpc bounded rate-limit retry (429 / -32005 / transient 503) on the probe sample, FetchLogs, and Header (requirements.md §24)
  - README.md operator runbook — prerequisites, ClickHouse startup, env config, bounded ingest rules, FINAL-read inspection SQL, test commands; placeholders only, never a credential
  - executed live smoke evidence — 40-block confirmed window through cmd/indexer into ClickHouse (3,728 fetched USDC Transfers -> 41 exact supply rows, zero ordinary-transfer rows)
affects: [01-live-supply-event-path, 02-reorg-safety, 03-supply-anchoring]

actuals:
  tokens: 7313    # chars/4 over the realized diff (29253 chars, 4 files)
  tasks: 2
  commits: 4      # measured: git rev-list --count e01e23f..HEAD
plan_head_before: e01e23f31f5fd4cd50f132bc93cecccc4cec4e41
plan_head_after: e82855c076508817c37f437b1f1fbfa6fdd7e2ea

tech-stack:
  added: []
  patterns: [env-gated live-fixture tests that skip (never fail) without ETH_RPC_URL, test-side pacing + production-side bounded retry for free-tier rate limits, credential scrubbing of net/http url.Error output]

key-files:
  created:
    - internal/indexer/live_test.go
    - README.md
  modified:
    - internal/rpc/client.go
    - internal/rpc/client_test.go

key-decisions:
  - "Free-tier rate-limit tolerance is a bounded retry inside internal/rpc (shared by probe, fetch, header) — added after live 429/-32005 bursts deterministically blocked the ingest; test-side pacing alone was not enough"
  - "Live USDC row-count assertion is per fixture receipt plus a block-wide 1:1 zero-address-Transfer-to-row invariant — block 0x1638e19 also carries two unrelated USDC mints, so a per-block count would be wrong"
  - "Smoke window sized to the provider result cap: Infura rejects eth_getLogs over 10,000 results, so ~40 blocks is the safe USDC window; documented in README (chunked fetches belong to the continuous-sync phase)"

pattern-links:
  - "live_test.go -> internal/rpc.Client.FetchLogs + internal/indexer.DecodeSupplyEvent: same code path as cmd/indexer, proving the 01-01 surface on real chain data"
  - "cmd/indexer smoke -> migrations/clickhouse.sql stablecoin_events: bounded range stored and inspected with FINAL"

requirements-completed: [EVENT-01, EVENT-02, EVENT-03]

coverage:
  - id: D1
    description: "Five pinned Mainnet fixtures classify through the operator's real provider with exact event type, raw amount, tx hash, log index, and address sides"
    requirement: EVENT-01
    verification:
      - kind: integration
        ref: "ETH_RPC_URL=<provider> go test ./internal/indexer -run TestLiveFixtureClassification -v -count=1 — 5/5 subtests PASS, none skipped"
        status: pass
    human_judgment: false
  - id: D2
    description: "Paired USDC Mint/Burn logs arriving in address-only fetches produce no rows (1:1 zero-address-Transfer invariant; fixture receipt yields exactly one row); every USDT Transfer-topic log yields no row while pinned supply events still decode"
    requirement: EVENT-02
    verification:
      - kind: integration
        ref: "ETH_RPC_URL=<provider> go test ./internal/indexer -run 'TestLiveUSDCPairedEventsNotCounted|TestLiveUSDTTransfersYieldNothing' -v -count=1 — 5/5 subtests PASS"
        status: pass
    human_judgment: false
  - id: D3
    description: "Bounded recent-range full-path smoke: cmd/indexer over a confirmed 40-block window stores real rows with all provenance columns populated and no ordinary-transfer rows"
    requirement: EVENT-03
    verification:
      - kind: integration
        ref: "go run ./cmd/indexer -config config/tokens.json -from 26147231 -to 26147271 — probe passed, USDT 0 logs, USDC 3728 fetched / 41 stored; docker exec clickhouse-client SELECT ... FINAL shows mint rows with empty from and burn rows with empty to"
        status: pass
    human_judgment: false
  - id: D4
    description: "README documents storage (docker run, 127.0.0.1 bind), configuration (ETH_RPC_URL placeholder), bounded ingest (confirmation rule, result caps, idempotent retry), inspection (SELECT ... FINAL, event_type vocabulary), and test commands — with no credential or provider URL anywhere"
    requirement: CHAIN-01
    verification:
      - kind: unit
        ref: "go test ./... green; grep ETH_RPC_URL README.md > 0; ! grep -Eq 'https?://[a-z0-9.-]*(alchemy|infura|quicknode)[a-z0-9./_-]*' README.md; repo-wide git grep for the key/proxy literal: empty"
        status: pass
      - kind: integration
        ref: "README inspection SQL executed verbatim via docker exec tm-clickhouse clickhouse-client — returns rows with all 12 columns populated"
        status: pass
    human_judgment: false
  - id: D5
    description: "Operator can reproduce the whole slice from README alone (start ClickHouse, export env, bounded ingest, SQL inspection)"
    verification: []
    human_judgment: true
    rationale: "Runbook walk-through usability is a human judgment; automated checks cover structure, credential-freeness, and SQL validity. Collected at end-of-phase /gsd-verify-work with the operator's own terminal."

duration: 26min
completed: 2026-10-08
status: complete
---

# Phase 1 Plan 02: Live Proof and Operator Runbook Summary

**Live chain verification of the offline-proven classification — five pinned Mainnet fixtures plus paired-event and USDT-Transfer negative paths all pass through the operator's Infura endpoint, a bounded 40-block ingest stores 41 exact supply rows in ClickHouse, and a credential-free README makes the slice reproducible by a human.**

## Performance
- **Duration:** 26 min (10:47-11:13 UTC)
- **Started/Completed:** 2026-10-08T10:47:23Z / 2026-10-08T11:13:14Z
- **Tasks:** 2/2
- **Files modified:** 4 (2 created, 2 modified)

## Accomplishments
- All five researched Mainnet receipts classify correctly through real `eth_getLogs` responses from the operator's provider: USDT Issue -> mint (1,000,000 USDT), Redeem -> redeem (3,000,000 USDT), DestroyedBlackFunds -> destroyed_black_funds (200,000 USDT, blacklisted address pinned), USDC zero-from Transfer -> mint, USDC zero-to Transfer -> burn — each with exact raw amount, tx hash, log index, and address sides (EVENT-01, EVENT-02 chain-verified; assumption A3 closed for this provider).
- No-double-count proof runs end to end on live data: address-only fetches at the USDC fixture blocks deliver the paired Mint/Burn logs to the decode step and produce rows exactly for the zero-address Transfers; block 0x1638e19's two unrelated extra mints correctly produce their own rows. Every USDT Transfer-topic log — zero-address or ordinary — yields no supply row while the pinned supply events in the same blocks still decode.
- Full-path smoke executed live: probe -> fetch -> decode -> header enrichment -> ClickHouse insert -> inspection over a confirmed 40-block window (3,728 fetched USDC Transfer logs -> 41 stored supply rows; every row a genuine mint or burn; zero rows from ordinary transfers) (EVENT-03).
- Bounded rate-limit retry added to the shared RPC client after free-tier throttling deterministically blocked the ingest; driven by a genuine TDD RED (classifier verdict RED_EVIDENCE_OK) -> GREEN cycle.
- README runbook documents the whole operator procedure with placeholders only; repo-wide scans prove no credential, provider URL, or proxy address in any tracked file (D-01, T-01-04).

## Task Commits
1. **Task 1 (tdd): live fixture proof** — `148e482` (test)
2. **Task 2: operator runbook + smoke** — `e82855c` (docs)
**Deviation-driven production fix (Rule 2/3):** — `54b6df9` (test: RED) + `f4214dc` (feat: GREEN)
**Plan metadata:** final docs commit below.

## TDD Gate Compliance
- Task 1 (`tdd="true"`) is a **test-only task against the plan 01-01 surface** — the plan itself declares "No new production-code symbols — this plan exercises the plan 01-01 surface end to end". All asserted behavior (decoder classification, rpc fetch) already existed, so a RED for TestLive\* could only have been manufactured (sabotaged expectations / infrastructure crashes), which #3770 classifies as INVALID_RED. The suite was written, run offline (skip path verified), then run live: first execution confirmed the classification assertions green on the pre-existing surface. Deviation documented below (item 5).
- The deviation-driven rate-limit retry **did** run a genuine RED -> GREEN cycle: `TestFetchLogsRetriesRateLimits` failed on its own retry assertion (`54b6df9`), evidence machine-validated as `RED_EVIDENCE_OK` / `target_test_failed` (record: `.planning/research/.cache/tdd-red-01-02-rpc.json`, TAP projection via the 01-01 converter), then passed after implementation (`f4214dc`). Gate greps find both `test(01-02)` and `feat(01-02)` commits. No refactor needed.

## Files Created/Modified
- `internal/indexer/live_test.go` — created: TestLiveFixtureClassification, TestLiveUSDCPairedEventsNotCounted, TestLiveUSDTTransfersYieldNothing (package indexer_test; ETH_RPC_URL-gated; walk-up config resolution; free-tier pacing with backoff; URL-scrubbed failure output)
- `README.md` — created: Prerequisites, Storage, Configuration, Bounded Ingest, Inspection, Tests sections
- `internal/rpc/client.go` — modified: withRateLimitRetry + isRateLimitErr, applied to probe sample getLogs, FetchLogs, Header
- `internal/rpc/client_test.go` — modified: cannedRPC gains a throttle injector and attempt counter; TestFetchLogsRetriesRateLimits added

## Decisions Made
- Rate-limit tolerance lives in the shared RPC client (bounded backoff retry on 429 / -32005 / transient 503) rather than per call site or test-only pacing: one fix covers probe, fetch, and the header burst, and requirements.md §24 mandates it (see Deviation 1).
- Live USDC row assertion is per fixture receipt plus a block-wide 1:1 zero-address-Transfer-to-row invariant, matching must-have truth #2's "exactly one supply row emerges per fixture" wording and chain reality (see Deviation 2).
- Smoke window follows the provider result cap (40 blocks for USDC on Infura) per the plan's run-time discretion clause; chunked sub-range fetching is deferred to the continuous-sync phase (see Deviation 3).

## Deviations from Plan

**1. [Rule 2 - Missing critical functionality] Bounded rate-limit retry in internal/rpc**
- **Found during:** Task 2 smoke run (also degraded the first Task 1 live run)
- **Issue:** Free-tier throttling (HTTP 429 / JSON-RPC -32005) deterministically killed the ingest: probe + two fetches burst ~5 calls in 1.3 s and every wait-and-retry reproduced the same burst — wait-and-retry alone could not recover, blocking must-have truth #3. requirements.md §24 and AGENTS.md require rate-limit handling; 01-01 shipped none.
- **Fix:** `withRateLimitRetry` (2s/4s/8s/16s, context-aware) wrapping the probe sample `eth_getLogs`, `FetchLogs`, and `Header` — all idempotent historical reads. Test-side pacing kept in live_test.go to avoid tripping limits in the first place. TDD: RED `54b6df9` (RED_EVIDENCE_OK) -> GREEN `f4214dc`.
- **Files modified:** internal/rpc/client.go, internal/rpc/client_test.go
- **Verification:** TestFetchLogsRetriesRateLimits PASS; full offline suite green; smoke run subsequently succeeded; final live suite re-run all PASS.
- **Commit:** 54b6df9, f4214dc

**2. [Rule 1 - Plan assumption vs chain reality] "Exactly one supply row per block" is false for one fixture block**
- **Found during:** Task 1 (pre-assertion live probe of the two USDC blocks)
- **Issue:** Task 1's behavior text says "decoding all returned logs must yield exactly one supply row per block"; live probe showed block 0x1638e19 contains the pinned burn **plus two unrelated USDC mints** (tx 0xf48467ac..., log indexes 309/317) — the literal per-block assertion would be wrong. Must-have truth #2's own wording ("exactly one supply row emerges per **fixture**", "paired Mint/Burn logs in the same **receipts**") is per-receipt and matches reality.
- **Fix:** Test asserts per fixture receipt (exactly one row from the pinned tx+logIndex) plus the block-wide invariant: paired Mint/Burn logs present and never producing rows, and supply rows == zero-address Transfers 1:1.
- **Files modified:** internal/indexer/live_test.go
- **Verification:** TestLiveUSDCPairedEventsNotCounted PASS on both blocks (0x1632fcb: 1 row; 0x1638e19: 3 rows, all genuine).
- **Commit:** 148e482

**3. [Rule 3 - Blocker] Provider result cap forced a smaller smoke window**
- **Found during:** Task 2 smoke run
- **Issue:** Infura rejects `eth_getLogs` returning >10,000 results; a 200-block USDC Transfer range (~14-28k logs) fails with a named sub-range suggestion, so the plan's "roughly 200 blocks" window is infeasible on this provider.
- **Fix:** Used a 40-block confirmed window (26147231-26147271) per the plan's own run-time discretion clause ("exact numbers computed at run time"); README documents the cap and window guidance. Chunked range fetching is continuous-sync (Phase 2) work.
- **Files modified:** README.md
- **Verification:** smoke run stored 41 rows; README gate green.
- **Commit:** e82855c

**4. [Rule 2 - Security hygiene] Credential-bearing URL scrubbed from test failure output**
- **Found during:** Task 1 live run (proxy misconfiguration exposed it)
- **Issue:** net/http `*url.Error` messages embed the complete request URL including the API key; the test printed such errors verbatim (D-01 hygiene gap in the new file — production errors prefix `HostOnly` but `%w` chains can still carry the raw URL).
- **Fix:** `scrub()` helper in live_test.go replaces the endpoint with scheme://host in every failure path.
- **Files modified:** internal/indexer/live_test.go
- **Verification:** failure output now shows only `https://mainnet.infura.io`; repo-wide key scan clean.
- **Commit:** 148e482

**5. [Process] Task 1 RED inapplicable — documented immediate GREEN**
- **Found during:** Task 1 TDD step
- **Issue:** The task's planned behavior (decode/fetch surface) was fully implemented by plan 01-01; no honest RED exists for TestLive\* without manufacturing an INVALID_RED.
- **Fix:** Ran the suite offline (skip path) then live (all PASS on pre-existing surface); documented here and under TDD Gate Compliance. The deviation-driven retry fix carried the plan's genuine RED->GREEN evidence.
- **Verification:** TDD gate greps find test(01-02) and feat(01-02) commits.
- **Commit:** n/a (documentation)

**Total deviations:** 5 (1 missing-critical auto-fix with TDD, 1 plan-assumption correction, 1 environment blocker, 1 security hygiene, 1 process documentation)
**Impact on plan:** all must-have truths and acceptance criteria verified as specified or stronger (per-receipt + 1:1 invariant); no planned behavior weakened.

## Authentication Gates
None — the ETH_RPC_URL credential was supplied by the orchestrator (user_setup satisfied) and passed as command-prefix environment only; the provider never returned an auth failure.

## Issues Encountered
- Transient zsh word-splitting and a proxy-URL typo (`/7897/` vs `:7897/`) caused one failed probe round early in Task 1 — fixed in the invocation, no file changes.
- First live test run hit 429 bursts before pacing was added (3 subtests failed on transport, not assertions) — resolved by Deviation 1's pacing + retry.
- USDT fetched 0 supply logs in both smoke windows (plausible: recent USDT Issue/Redeem activity is sparse); USDC provided the live rows. Not a defect — the fixture tests already prove USDT classification on-chain at the pinned blocks.

## Known Stubs
None — no placeholder values, no untested logic paths, no unwired components.

## User Setup Required
`ETH_RPC_URL` remains the single operator input for live suites (see `.planning/phases/01-live-supply-event-path/01-USER-SETUP.md`). It was satisfied for this run via command-prefix env; future live runs need the same variable plus working proxy egress from this machine.

## Next Phase Readiness
Phase 1 complete — both plans executed. The full offline suite is green, the live suite is green against the operator's provider, and the smoke path is proven into ClickHouse. Ready for end-of-phase verification (`/gsd-verify-work`), which should collect the D5 human judgment (operator walks the README) and the A4 open item (deployed USDT zero-address Transfer negative fixture).

---
*Phase: 01-live-supply-event-path*
*Completed: 2026-10-08*

## Self-Check: PASSED
All 4 changed production files and the SUMMARY exist on disk; all 4 task commits (148e482, 54b6df9, f4214dc, e82855c) are ancestors of HEAD (plan_head_after e82855c).
