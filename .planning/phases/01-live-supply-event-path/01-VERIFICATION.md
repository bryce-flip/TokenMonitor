---
phase: 01-live-supply-event-path
verified: 2026-10-08T12:18:21Z
status: passed
score: 16/16 must-haves verified
covered_files:
  - .planning/phases/01-live-supply-event-path/01-01-PLAN.md
  - .planning/phases/01-live-supply-event-path/01-01-SUMMARY.md
  - .planning/phases/01-live-supply-event-path/01-02-PLAN.md
  - .planning/phases/01-live-supply-event-path/01-02-SUMMARY.md
  - README.md
  - cmd/indexer/main.go
  - config/tokens.json
  - go.mod
  - go.sum
  - internal/config/config.go
  - internal/config/config_test.go
  - internal/indexer/decoder.go
  - internal/indexer/decoder_test.go
  - internal/indexer/live_test.go
  - internal/rpc/client.go
  - internal/rpc/client_test.go
  - internal/storage/clickhouse.go
  - internal/storage/clickhouse_test.go
  - migrations/clickhouse.sql
covered_digest: "v3:sha256:3ec5cf5c6d027c10ccc084db34ea5ca79627d8255e145378f8893b5fde8a9b07"
behavior_unverified: 0 # every behavior-dependent truth was exercised by a test this verifier ran, by prior direct observation that the fixes did not touch, or by this pass's live run of the fixed binary
overrides_applied: 0
re_verification:
  previous_status: human_needed
  previous_score: 16/16
  gaps_closed: [] # the prior report had no failed-truth gaps; the three code-review warnings it flagged (WR-01, WR-02, WR-03) were fixed by c9204df, 8198091, 9d50429 before this pass
  gaps_remaining: []
  regressions: []
deferred:
  - truth: "Provider 10k-result eth_getLogs cap forces ~40-block USDC windows; chunked sub-range fetching absent"
    addressed_in: "Phase 2"
    evidence: "Phase 2 SC1: 'Indexer advances in bounded ranges only through the configured confirmed or finalized head and continues as new eligible blocks arrive' (SYNC-01)"
human_verification:
  - test: "Walk the README runbook end to end as an operator (start ClickHouse, export env, bounded ingest, SQL inspection)"
    expected: "The whole Phase 1 slice is reproducible from README.md alone with no undocumented steps"
    why_human: "Runbook usability is a judgment call (plan 01-02 coverage item D5, explicitly deferred to end-of-phase human verification)"
  - test: "Ratify the accepted-risk decision AR-01 recorded in 01-SECURITY.md: net/http *url.Error chains wrapped with %w through internal/rpc can still print the full credential-bearing RPC URL in runtime error logs (admitted in 01-02-SUMMARY Deviation 4; only test output is scrubbed)"
    expected: "Human confirmation that the Phase 1 acceptance stands (proper redaction lands with Phase 2 RPC error handling), or a request to scrub wrapped errors in production now"
    why_human: "Risk-acceptance ratification; grep proves tracked files clean but cannot prove the runtime log path never leaks. The prohibition 'never embed RPC URLs or credentials in ... logs' is therefore flagged, not silently passed"
  - test: "Confirm or dismiss the open research item A4: locate (or conclude none exists on relevant history) a deployed USDT zero-address Transfer negative fixture on Mainnet"
    expected: "Either a pinned on-chain negative fixture exercised by TestLive, or a documented conclusion that the path is unreachable on chain"
    why_human: "Requires chain archaeology beyond the phase's pinned fixtures; the decoder-level negative is already proven offline and live (every USDT Transfer-topic log at the three fixture blocks yields no row)"
  - test: "Reformat the Phase 1 goal in ROADMAP.md into user-story format (e.g. via /gsd-mvp-phase 1) — the phase is Mode: mvp but the goal 'Operators can ingest and inspect...' fails user-story validation"
    expected: "Goal matches 'As a [role], I want to [capability], so that [outcome].' so future MVP-mode UAT generation works as designed"
    why_human: "Planning-metadata edit that changes the phase contract; developer decision. The equivalent validated story exists verbatim in both PLAN objectives and was used for this verification's User Flow Coverage"
  - test: "New since the WR-01 fix (c9204df): observe or otherwise exercise the reorg-mismatch guard in cmd/indexer/main.go (h.Hash() != log.BlockHash -> slog.Error + exit 1) against a live provider"
    expected: "On hash agreement the ingest proceeds (this pass confirmed the happy path live, exit 0); on mismatch the run fails loudly naming block, header hash, and log block hash"
    why_human: "The mismatch branch is only observable under a live reorg or hostile provider — no offline test can reach it (the fixer's own 01-REVIEW-FIX.md recommendation to human-verify before relying on it)"
---

# Phase 1: Live Supply Event Path Verification Report

**Phase Goal:** Operators can ingest and inspect correctly classified, real USDT and USDC supply-changing logs in ClickHouse.
**Verified:** 2026-10-08T12:18:21Z
**Status:** human_needed
**Re-verification:** Yes — after code-review fixes (WR-01 c9204df, WR-02 8198091, WR-03 9d50429; disposition 5ee72a1). The prior report (2026-10-08T11:45:13Z, human_needed, 16/16) predates those fixes and has been overwritten by this one.

## What changed since the prior report, and how it was re-verified

| Fix | Commit | Claimed change | Verified in code | Behavior re-proven |
|-----|--------|----------------|------------------|--------------------|
| WR-01 | c9204df | Rows store `types.Log.BlockHash`; header kept for `block_time`; hash mismatch fails loudly | main.go L121-126 (mismatch -> slog.Error + exit 1), L132 (`BlockHash: tl.log.BlockHash.Hex()`), L133 (`h.Time` for BlockTime) | Live zero-log run of the fixed binary: probe passed, 0 logs, exit 0 (happy path exercises the hash-agreement path); mismatch branch routed to human item 5 |
| WR-02 | 8198091 | `Inspect(ctx, from, to, limit)` range-scoped, newest first, all-bound params | clickhouse.go L106-113 (`WHERE block_number BETWEEN ? AND ?`, `ORDER BY block_number DESC, log_index DESC`, `LIMIT ?` bound); main.go L156 calls `Inspect(ctx, from, to, 20)` | Storage suite 5/5 PASS incl. rewritten ordering test (newest-first, out-of-range row excluded); read-only ranged FINAL query over the live smoke range returned exactly the 41 ingested rows newest-first with correct mint/burn semantics; live zero-log run printed an empty (correctly scoped) table |
| WR-03 | 9d50429 | `Validate` rejects empty/whitespace and unsupported symbols at startup | config.go L50 (`supportedSymbols`), L67-72 (empty + not-supported checks); config_test.go new | `TestValidateSymbolFailFast` 4/4 subtests PASS + valid config still passes (verifier-run, -v) |

Fix-commit scope confirmed via `git show --stat`: exactly the claimed files, nothing else.

## MVP Mode Note (goal format discrepancy — unchanged)

ROADMAP.md declares `Mode: mvp` for this phase, but the goal is not in user-story format. Both PLAN objectives carry the identical story in valid format (`user-story.validate` -> `valid: true` this pass), which this verification used for User Flow Coverage:

> As a **stablecoin operator**, I want to **ingest and inspect correctly classified, real USDT and USDC supply-changing logs in ClickHouse**, so that I can **trust the event data underlying later supply and issuance figures**.

The discrepancy remains escalated as human-verification item 4, not treated as a code gap.

## User Flow Coverage

| Step | Expected | Evidence | Status |
|------|----------|----------|--------|
| Configure provider + tokens + confirmation policy | Operator supplies ETH_RPC_URL env and config/tokens.json; no business-logic edits (CHAIN-01) | `internal/config/config.go` (Load/Validate/RPCURL/ClickHouseURL; Validate now also rejects bad symbols per WR-03), `config/tokens.json` (chain_id 1, conf 20, USDT/USDC), README §3 | ✓ |
| Run a bounded Mainnet range through the indexer | cmd/indexer -config/-from/-to runs probe -> fetch -> decode -> insert -> inspect; zero-log range exits 0 | `cmd/indexer/main.go`; **this pass re-ran `-from 3000000 -to 3000000` live through the fixed binary: probe passed, both tokens 0 logs, "stored 0 supply events", exit 0** | ✓ |
| Ingest stores real classified rows | Real chain rows land in stablecoin_events with full provenance (now the log's own BlockHash per WR-01) | Orchestrator-executed 40-block smoke (3,728 fetched USDC Transfers -> 41 stored); this pass's read-only FINAL query re-confirms the 41 rows in range 26147231-26147271, newest-first, 66-char block_hash/tx_hash, every mint from-empty / every burn to-empty | ✓ |
| Inspect stored events via SQL | Copy-paste SELECT over stablecoin_events FINAL returns rows; post-ingest table now shows the just-ingested range (WR-02) | README §5 query; verifier's ranged query above; cmd/indexer post-ingest Inspect is range-scoped and newest-first | ✓ |
| Outcome: trust the data | Classification exact on real chain data; no double counts; no ordinary transfers; exact amounts; replay idempotent | Prior session's direct TestLive run (10/10 subtests PASS through the operator's provider — fixes touched none of those code paths: decoder.go, live_test.go, and client.go fetch paths unchanged since); 2^256-1 round-trips exactly; duplicate insert keeps FINAL count 1 (both re-run this pass against the container) | ✓ |

## Goal Achievement

### Observable Truths

Roadmap success criteria (SC1-SC4) roll up the plan truths below; all four are VERIFIED.

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | SC1: operator configures RPC URL, token metadata, confirmation policy without business-logic edits and runs a bounded Mainnet range into ClickHouse | ✓ VERIFIED | config.go + tokens.json + README; this pass's live zero-log run of the fixed binary exit 0; prior live smoke with 41 stored rows; WR-03 strengthens startup validation |
| 2 | SC2: stored USDT logs show Issue->mint, Redeem and DestroyedBlackFunds as distinct decreases; zero-address Transfer not misclassified as USDT issuance | ✓ VERIFIED | decoder.go `decodeTransfer` returns nil for non-USDC (unchanged by fixes); offline fixture table green this pass; prior verifier-run TestLive (USDT subtests PASS, TestLiveUSDTTransfersYieldNothing 3/3) |
| 3 | SC3: stored USDC logs show zero-address Transfer mint/burn once each, no double-counting paired Mint/Burn | ✓ VERIFIED | Prior verifier-run TestLiveUSDCPairedEventsNotCounted 2/2 PASS (address-only fetches, paired logs present, 1:1 invariant); this pass's read-only query re-confirms 41 genuine rows |
| 4 | SC4: operator can inspect exact raw amount and chain provenance; ordinary transfers do not appear | ✓ VERIFIED | README §5 SQL + verifier's ranged FINAL query; TestExactUint256RoundTrip (2^256-1 exact decimal string) re-run PASS; zero ordinary-transfer rows; block_hash now carries the log's own attested hash (WR-01, stronger provenance) |

### Plan 01-01 Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | build/vet/test green with no ETH_RPC_URL and no CLICKHOUSE_URL (storage skips cleanly) | ✓ VERIFIED | Re-run this pass: `env -u ETH_RPC_URL -u CLICKHOUSE_URL go build ./... && go vet ./... && go test ./... -count=1` -> exit 0, all packages `ok` (incl. the new internal/config suite) |
| 2 | Five pinned Mainnet fixtures classify with exact amounts | ✓ VERIFIED | `TestDecodePinnedMainnetFixtures` green in this pass's offline run (decoder_test.go untouched by the fixes — verified via commit stats); pinned tx hashes/amounts confirmed present in the file |
| 3 | USDT zero-address Transfer / USDC ordinary Transfer / USDC paired Mint-Burn yield no supply row | ✓ VERIFIED | `TestDecodeNegativesYieldNoSupplyRow` green |
| 4 | Both-zero USDC Transfer rejected with an error | ✓ VERIFIED | `TestDecodeMalformedAndMismatchAreErrors` "from and to are both the zero address" green |
| 5 | Wrong data length -> error naming event and expected length, never zero amount | ✓ VERIFIED | Same test: Issue 31-byte and DestroyedBlackFunds 32-byte named-error cases green |
| 6 | Probe aborts on wrong chain / past confirmation boundary (exact) / failing sample getLogs | ✓ VERIFIED | `TestProbeRejectsWrongChain`, `TestProbeConfirmationBoundaryExactness`, `TestProbeRejectsProviderWithoutHistoricalLogs` green (client_test.go untouched by the fixes) |
| 7 | No ETH_RPC_URL -> non-zero exit naming ETH_RPC_URL | ✓ VERIFIED | Re-run this pass: `env -u ETH_RPC_URL go run ./cmd/indexer -from 100 -to 200` -> exit 1, message names ETH_RPC_URL |
| 8 | Zero-log range ingests zero rows, exits 0 | ✓ VERIFIED | Re-run this pass through the FIXED binary live (block 3,000,000): probe passed, 0 logs fetched per token, "stored 0 supply events", exit 0 |
| 9 | Exact round-trip incl. 2^256-1, no float anywhere in the path | ✓ VERIFIED | `TestExactUint256RoundTrip` PASS this pass against the running ClickHouse 26.8 container; *big.Int path intact in code read |
| 10 | Duplicate insert leaves FINAL count at 1 | ✓ VERIFIED | `TestReplayDuplicateInsertKeepsFinalCountAtOne` PASS this pass |
| 11 | Inspect deterministically ordered by block_number then log_index | ✓ VERIFIED | Reinterpreted per the WR-02 code-review fix (documented in 01-REVIEW-FIX.md / 01-REVIEW-DISPOSITION.md, disposition fixed): ordering key is still (block_number, log_index), direction is now DESC (newest first) and scoped to the ingested range — a deliberate operator-UX improvement, not a regression. `TestInspectOrderedByBlockThenLogIndex` asserts newest-first order AND exclusion of out-of-range rows; PASS this pass against the real server; read-only ranged query over the live smoke range confirms newest-first output |

### Plan 01-02 Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Live: five pinned fixtures classify through real eth_getLogs with exact values (backstop) | ✓ VERIFIED | Prior session's direct verifier run: `go test ./internal/indexer -run TestLive -v -count=1` through the operator's provider, 5/5 subtests PASS, none skipped. The three fixes touched none of this path's code (decoder.go, live_test.go, client.go fetch); the fixed binary's live probe/fetch behavior was re-confirmed by this pass's zero-log run |
| 2 | Live: address-only USDC fetch, exactly one supply row per fixture, paired Mint/Burn produce none end-to-end (backstop) | ✓ VERIFIED | Same prior run: 2/2 subtests PASS; test fatals if no paired logs reached decode (non-vacuous) |
| 3 | Live: bounded recent-range run stores rows with all provenance populated, no ordinary-transfer rows (backstop) | ✓ VERIFIED | Orchestrator-executed smoke (3,728 -> 41 rows) + this pass's independent read-only FINAL query over the range confirming all provenance columns populated and correct zero-address semantics |
| 4 | README documents docker run, env vars, ingest flags, copy-paste FINAL inspection | ✓ VERIFIED | README §1-§6 read; unchanged by the fix commits (confirmed via commit stats) |
| 5 | README and live_test.go contain no provider URL or credential values | ✓ VERIFIED | Re-scanned this pass: repo-wide `git grep` at HEAD for the key, proxy, and provider-URL literals -> no tracked file matches; README provider-host regex clean |

**Score:** 16/16 truths verified (0 present-but-behavior-unverified)

**Prohibitions (backstop tier):** USDT zero-address Transfer never classified as supply (decoder rule + offline and live negatives — holds); USDC paired Mint/Burn never inserted (1:1 invariant proven live — holds); never embed provider URLs/credentials in source/config/tests/README (tracked-file grep clean this pass — holds on the static portion; the runtime `%w`-wrapped url.Error residual remains **flagged as unverified-prohibition — human review recommended**, now formally recorded as accepted risk AR-01 in 01-SECURITY.md — human item 2); never convert raw amounts to float (full *big.Int / UInt256 path re-read — holds).

### Deferred Items

| # | Item | Addressed In | Evidence |
|---|------|-------------|----------|
| 1 | Provider 10k-result eth_getLogs cap forces ~40-block USDC windows; chunked sub-range fetching absent | Phase 2 | Phase 2 SC1 "advances in bounded ranges only ... and continues as new eligible blocks arrive" (SYNC-01) |

The prior report's deferred item WR-01 (reorg provenance) was **fixed in-phase** by c9204df and is no longer deferred.

### Advisory (New Scope, Unevidenced)

Re-verification ran; no new unevidenced blockers found. The eight info-tier code-review findings (IN-01..IN-08) remain formally dispositioned `skipped` (info severity, outside critical_warning fix scope) in 01-REVIEW-DISPOSITION.md; IN-07 was incidentally resolved by WR-02's bound-parameter rewrite. None break a must-have.

| # | Finding | Category | Why Advisory |
|---|---------|----------|--------------|
| 1 | None new. Open info-tier items (IN-01..IN-06, IN-08) are tracked with recorded dispositions | other | dispositioned info severity; no deterministic failure evidence; does not affect any must-have |

### Required Artifacts

All 14 artifacts exist, are substantive (full file reads this pass), and are wired. `verify.artifacts` parses 0/0 because these plans declare artifacts as plain string lists rather than `{path, provides}` objects — verification performed manually.

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `cmd/indexer/main.go` | Bounded CLI wiring the full path | ✓ VERIFIED | 179 lines; Load->RPCURL->Probe->FetchLogs->Header->hash-assert->Decode->InsertEvents->ranged Inspect, all present and ordered |
| `config/tokens.json` | Public token metadata | ✓ VERIFIED | chain_id 1, conf 20, USDT/USDC contracts verbatim from requirements |
| `internal/config/config.go` | Config + env lookups + redaction + startup validation | ✓ VERIFIED | 114 lines; Validate enforces all plan invariants plus WR-03 symbol checks |
| `internal/config/config_test.go` | WR-03 proof (new since fixes) | ✓ VERIFIED | 53 lines; table-driven, 4/4 failure cases + valid-config pass, verifier-run green |
| `internal/rpc/client.go` | Probe/FetchLogs/Header + rate-limit retry | ✓ VERIFIED | 145 lines; HostOnly on every constructed error |
| `internal/indexer/decoder.go` | Strict-shape classifier | ✓ VERIFIED | 140 lines; pinned topic0s, exact per-token rules; untouched by fixes |
| `internal/storage/clickhouse.go` | Store: EnsureSchema/InsertEvents/ranged Inspect | ✓ VERIFIED | 134 lines; *big.Int end to end, FINAL reads, bound parameters everywhere |
| `migrations/clickhouse.sql` | stablecoin_events DDL | ✓ VERIFIED | ReplacingMergeTree(created_at), ORDER BY full event identity, raw_amount UInt256 |
| `internal/indexer/decoder_test.go` | Fixture/negative/error proofs | ✓ VERIFIED | 358 lines; five pinned fixtures with exact values |
| `internal/rpc/client_test.go` | Probe behavior proofs | ✓ VERIFIED | 189 lines; canned JSON-RPC server incl. throttle injector |
| `internal/storage/clickhouse_test.go` | Exactness/idempotence/ordering proofs | ✓ VERIFIED | 171 lines; env-gated; ordering test rewritten for WR-02 and green against the real server |
| `internal/indexer/live_test.go` | Live chain proofs | ✓ VERIFIED | 362 lines; env-gated, paced, scrubbed |
| `README.md` | Operator runbook | ✓ VERIFIED | 133 lines; all six required sections |
| `go.mod`/`go.sum` | Pinned deps | ✓ VERIFIED | go-ethereum v1.17.7, clickhouse-go/v2 v2.48.0 committed |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| cmd/indexer/main.go | full pipeline | Load->RPCURL->Probe->FetchLogs(SupplyTopics)->Header->DecodeSupplyEvent->InsertEvents->Inspect->stdout | ✓ WIRED | Code read: probe at L70 precedes first FetchLogs at L88; ranged inspect table printed L163-171; behavior confirmed by this pass's live zero-log run |
| internal/storage | migrations/clickhouse.sql | EnsureSchema (CREATE TABLE IF NOT EXISTS) before first insert | ✓ WIRED | main.go L148 EnsureSchema precedes L152 InsertEvents; TestEnsureSchemaIsIdempotent PASS this pass |
| internal/rpc errors | config.HostOnly | every error message renders scheme://host | ⚠️ PARTIAL | Every *constructed* message prefixes HostOnly, but `%w`-wrapped net/http `*url.Error` transport failures can still embed the full credential-bearing URL — unchanged by the fixes; now formally accepted risk AR-01 (01-SECURITY.md); escalated to human item 2 |
| live_test.go | rpc.Client.FetchLogs + DecodeSupplyEvent | same code path as cmd/indexer | ✓ WIRED | Package indexer_test breaks the import cycle; prior verifier-run TestLive exercised it |
| bounded smoke | stablecoin_events table | cmd/indexer main flow end to end | ✓ WIRED | 41 rows in the table with full provenance (verifier-queried this pass) |

Note on the plan 01-01 key link wording ("Header per block for timestamp/hash"): after WR-01 the header is fetched for `block_time` while `block_hash` comes from the log's own attested `BlockHash` — a deliberate strengthening of the link's intent (provenance correctness), documented in 01-REVIEW-FIX.md. The link holds in substance.

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|-------------------|--------|
| cmd/indexer inspect output | inspected []EventRow | `SELECT ... FROM stablecoin_events FINAL WHERE block_number BETWEEN ? AND ?` | Yes | ✓ FLOWING (range-scoped since WR-02) |
| stablecoin_events.raw_amount | EventRow.RawAmount (*big.Int) | eth_getLogs data -> SetBytes -> batch.Append | Yes | ✓ FLOWING (exact decimal round-trip incl. 2^256-1, re-run this pass) |
| block_hash | types.Log.BlockHash (since WR-01; previously header hash) | the provider-attested emitting block per log | Yes | ✓ FLOWING (66-char hashes confirmed in stored rows) |
| block_time | types.Header.Time | HeaderByNumber per distinct fetched block | Yes | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Clean-env build/vet/test | `env -u ETH_RPC_URL -u CLICKHOUSE_URL go build ./... && go vet ./... && go test ./... -count=1` | exit 0, all `ok` (incl. internal/config) | ✓ PASS |
| Fail-fast without RPC URL | `env -u ETH_RPC_URL go run ./cmd/indexer -from 100 -to 200` | exit 1, stderr names ETH_RPC_URL | ✓ PASS |
| WR-03 startup symbol validation | `go test ./internal/config -v -count=1` | 4/4 subtests PASS + valid config passes | ✓ PASS |
| ClickHouse integration suite (rewritten ordering test) | `CLICKHOUSE_URL=clickhouse://default@127.0.0.1:9000/default go test ./internal/storage -v -count=1` | 5/5 PASS, 0 SKIP | ✓ PASS |
| Fixed binary live end-to-end (zero-log) | `go run ./cmd/indexer -from 3000000 -to 3000000` (operator provider + proxy env prefix) | probe passed, 0 logs/token, "stored 0 supply events", empty scoped inspect table, exit 0 | ✓ PASS |
| Ranged Inspect over live ingested rows | `docker exec tm-clickhouse clickhouse-client --query "SELECT ... FINAL WHERE block_number BETWEEN 26147231 AND 26147271 ORDER BY block_number DESC, log_index DESC LIMIT 5"` + range count | 41 rows in range; newest-first; mint rows from-empty, burn rows to-empty; 66-char hashes | ✓ PASS |

Live TestLive suite not re-run this pass: the three fixes touched none of its code paths (decoder.go, live_test.go, client.go fetch), and the prior session's direct 10/10 PASS run remains valid evidence; the fixed binary's live probe/fetch/header behavior was re-confirmed via the zero-log run above.

### Probe Execution

No `probe-*.sh` scripts are declared by the plans or exist in the repo (`scripts/` absent); the phase's runnable checks are the Go suites above. N/A.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| CHAIN-01 | 01-01, 01-02(D4) | External RPC URL + token metadata + confirmation policy configurable without business-logic changes | ✓ SATISFIED | env-only ETH_RPC_URL (tracked-file grep clean), tokens.json, Validate (now fail-fast on bad symbols per WR-03), probe enforces the policy; README documents all of it |
| EVENT-01 | 01-01, 01-02 | USDT Issue->mint, Redeem/DestroyedBlackFunds distinct, verified against deployed transactions | ✓ SATISFIED | Offline fixture table green this pass + prior verifier-run live classifications of all three pinned USDT receipts with exact amounts |
| EVENT-02 | 01-01, 01-02 | USDC zero-address Transfer mint/burn without double-counting paired Mint/Burn | ✓ SATISFIED | Offline negatives green + prior verifier-run live address-only proofs (paired logs present, zero rows from them) |
| EVENT-03 | 01-01, 01-02 | Inspectable stored events with exact raw amount and provenance; ordinary transfers excluded | ✓ SATISFIED | Schema columns, ranged Inspect/README SQL, verifier's read-only query this pass, 41 genuine rows / zero ordinary transfers; block_hash provenance strengthened by WR-01 |

Orphaned requirements: none — REQUIREMENTS.md maps exactly CHAIN-01, EVENT-01..03 to Phase 1 and the plans claim exactly those (all four marked Complete in the traceability table, matching the verified state).

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| internal/rpc/client.go | 65,79,86,93,107,128,142 | `%w`-wrapped transport errors can carry the credential-bearing URL past the HostOnly prefix | ⚠️ Warning | Unchanged residual; accepted risk AR-01 (01-SECURITY.md); human item 2 |
| cmd/indexer/main.go | 122-126 | WR-01 reorg-mismatch guard has no offline-reachable test path | ⚠️ Warning | Fail-loud branch only observable under a live reorg; happy path live-verified this pass; human item 5 |
| (01-REVIEW-DISPOSITION.md) | — | IN-01..IN-08 info-tier findings, disposition `skipped` (IN-07 incidentally resolved by WR-02) | ℹ️ Info | Quality advisories with recorded dispositions; none break a must-have |

Debt markers (TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER): none in any phase file (grep this pass). Disabled/skipped tests: none (skips are env-gates that verifiably run when env is set — storage suite ran 5/5 with CLICKHOUSE_URL set). Circular tests: none — expected values originate from independent research (block-explorer capture) and were re-verified against live chain responses. Credential scan of tracked files at HEAD (API key, proxy address, provider URL literals): clean.

### Decision Coverage

2/2 CONTEXT.md decisions honored (D-01 env-only credentials, D-02 fail-fast probe) — gate message: "All trackable CONTEXT.md decisions are honored by shipped artifacts."

### Human Verification Required

1. **README walk-through (plan-declared D5)** — reproduce the slice from README.md alone.
2. **Credential-leak residual / AR-01 ratification** — the `%w`-wrapped RPC error residual is now formally recorded as accepted risk AR-01 in 01-SECURITY.md (accepted by bryce, 2026-10-08, redaction lands with Phase 2). The prohibition "never embed RPC URLs or credentials in ... logs" is therefore **flagged as unverified-prohibition — human review recommended** (tracked-file portion verified clean by grep; runtime-log portion has a documented leak path).
3. **Research item A4** — locate or rule out a deployed USDT zero-address Transfer negative fixture on chain.
4. **MVP goal format** — reformat the ROADMAP goal into user-story format (`/gsd-mvp-phase 1`) so MVP-mode UAT generation works as designed.
5. **New — WR-01 reorg-mismatch guard** — exercise or observe the `h.Hash() != log.BlockHash` fail-loud branch against a live provider before relying on it in anger (the fixer's own recommendation; the happy path was live-verified this pass).

### Gaps Summary

No must-have truth, artifact, or key link failed. All 16 plan truths and all 4 roadmap success criteria hold over the fixed tree, with behavior-level evidence: this pass re-ran the offline gates, the new WR-03 config suite, the full ClickHouse integration suite (including the rewritten WR-02 ordering test), a live zero-log end-to-end run of the fixed binary through the operator's provider, and read-only ranged queries over the previously ingested live rows; the prior session's directly observed TestLive results remain valid because none of the three fixes touched those code paths. All three code-review warnings (WR-01, WR-02, WR-03) are verified fixed in code and behavior; the prior deferred item WR-01 is closed in-phase, and the 10k-cap chunking item remains deferred to Phase 2. The `human_needed` status comes from five escalation items — the plan-deferred README usability judgment, the AR-01 credential-leak acceptance ratification, the open A4 research item, the ROADMAP goal-format discrepancy under MVP mode, and the new live-only reorg-guard observation — none of which block the goal, all of which need a human decision.

---

_Verified: 2026-10-08T12:18:21Z_
_Verifier: Claude (gsd-verifier)_
