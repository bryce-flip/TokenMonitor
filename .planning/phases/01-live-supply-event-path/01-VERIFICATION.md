---
phase: 01-live-supply-event-path
verified: 2026-10-08T11:45:13Z
status: human_needed
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
  - internal/indexer/decoder.go
  - internal/indexer/decoder_test.go
  - internal/indexer/live_test.go
  - internal/rpc/client.go
  - internal/rpc/client_test.go
  - internal/storage/clickhouse.go
  - internal/storage/clickhouse_test.go
  - migrations/clickhouse.sql
covered_digest: "v3:sha256:4dd55679c2649f2abd78759aa1b5eeead3aa00dabaab1c6493fb6f79627f2777"
behavior_unverified: 0 # every behavior-dependent truth was exercised by a test this verifier ran or by direct observation
human_verification:
  - test: "Walk the README runbook end to end as an operator (start ClickHouse, export env, bounded ingest, SQL inspection)"
    expected: "The whole Phase 1 slice is reproducible from README.md alone with no undocumented steps"
    why_human: "Runbook usability is a judgment call (plan 01-02 coverage item D5, explicitly deferred to end-of-phase human verification)"
  - test: "Decide the disposition of the credential-leak residual: net/http *url.Error chains wrapped with %w through internal/rpc can still print the full credential-bearing RPC URL in error logs (admitted in 01-02-SUMMARY Deviation 4; only the test output is scrubbed)"
    expected: "Either scrub wrapped errors in the production rpc error path, or explicitly accept the Phase 1 residual (prohibition 'never embed RPC URLs in logs' is therefore flagged, not silently passed)"
    why_human: "Risk-acceptance decision; grep proves tracked files clean but cannot prove the runtime log path never leaks"
  - test: "Confirm or dismiss the open research item A4: locate (or conclude none exists on relevant history) a deployed USDT zero-address Transfer negative fixture on Mainnet"
    expected: "Either a pinned on-chain negative fixture exercised by TestLive, or a documented conclusion that the path is unreachable on chain"
    why_human: "Requires chain archaeology beyond the phase's pinned fixtures; decoder-level negative is already proven offline and live (every USDT Transfer-topic log at the three fixture blocks yields no row)"
  - test: "Reformat the Phase 1 goal in ROADMAP.md into user-story format (e.g. via /gsd-mvp-phase 1) — the phase is Mode: mvp but the goal 'Operators can ingest and inspect...' fails user-story validation"
    expected: "Goal matches 'As a [role], I want to [capability], so that [outcome].' so future MVP-mode UAT generation works as designed"
    why_human: "Planning-metadata edit that changes the phase contract; developer decision. The equivalent validated story exists verbatim in both PLAN objectives and was used for this verification's User Flow Coverage"
---

# Phase 1: Live Supply Event Path Verification Report

**Phase Goal:** Operators can ingest and inspect correctly classified, real USDT and USDC supply-changing logs in ClickHouse.
**Verified:** 2026-10-08T11:45:13Z
**Status:** human_needed
**Re-verification:** No — initial verification

## MVP Mode Note (goal format discrepancy)

ROADMAP.md declares `Mode: mvp` for this phase, but the goal is not in user-story format (`user-story.validate` returns `valid: false`). Both PLAN objectives carry the identical story in valid format, which this verification used for User Flow Coverage:

> As a **stablecoin operator**, I want to **ingest and inspect correctly classified, real USDT and USDC supply-changing logs in ClickHouse**, so that I can **trust the event data underlying later supply and issuance figures**.

The discrepancy is escalated as a human-verification item (reformat via `/gsd-mvp-phase 1`), not treated as a code gap.

## User Flow Coverage

| Step | Expected | Evidence | Status |
|------|----------|----------|--------|
| Configure provider + tokens + confirmation policy | Operator supplies ETH_RPC_URL env and config/tokens.json; no business-logic edits (CHAIN-01) | `internal/config/config.go` (Load/Validate/RPCURL/ClickHouseURL), `config/tokens.json` (chain_id 1, conf 20, USDT/USDC contracts), README §3 | ✓ |
| Run a bounded Mainnet range through the indexer | cmd/indexer -config/-from/-to runs probe -> fetch -> decode -> insert -> inspect; zero-log range exits 0 | `cmd/indexer/main.go`; verifier ran `-from 3000000 -to 3000000` live: probe passed, 0 logs fetched, "stored 0 supply events", exit 0 | ✓ |
| Ingest stores real classified rows | Real chain rows land in stablecoin_events with full provenance | Orchestrator-executed 40-block smoke (3,728 fetched USDC Transfers -> 41 stored); verifier's read-only FINAL query confirms 41 USDC rows (21 mint / 20 burn), every mint row with empty from / every burn row with empty to, block_hash and tx_hash 66-char hex, populated block_time/log_index | ✓ |
| Inspect stored events via SQL | Copy-paste SELECT over stablecoin_events FINAL returns rows | README §5 query; verifier executed the equivalent query via `docker exec tm-clickhouse clickhouse-client` — populated output above | ✓ |
| Outcome: trust the data | Classification exact on real chain data; no double counts; no ordinary transfers; exact amounts; replay idempotent | Verifier ran the full TestLive suite through the operator's provider: 10/10 subtests PASS, none skipped (5 fixture classifications, 2 paired-event no-double-count blocks, 3 USDT-transfer-negative blocks); 2^256-1 round-trips exactly; duplicate insert keeps FINAL count 1 | ✓ |

## Goal Achievement

### Observable Truths

Roadmap success criteria (SC1-SC4) roll up the plan truths below; all four are VERIFIED.

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | SC1: operator configures RPC URL, token metadata, confirmation policy without business-logic edits and runs a bounded Mainnet range into ClickHouse | ✓ VERIFIED | config.go + tokens.json + README; live zero-log run exit 0 (verifier); live smoke with 41 stored rows (orchestrator, session-verified) |
| 2 | SC2: stored USDT logs show Issue->mint, Redeem and DestroyedBlackFunds as distinct decreases; zero-address Transfer not misclassified as USDT issuance | ✓ VERIFIED | decoder.go `decodeTransfer` returns nil for non-USDC; offline fixture table + verifier-run TestLive (USDT mint/redeem/destroyed_black_funds subtests PASS; TestLiveUSDTTransfersYieldNothing 3/3 PASS) |
| 3 | SC3: stored USDC logs show zero-address Transfer mint/burn once each, no double-counting paired Mint/Burn | ✓ VERIFIED | TestLiveUSDCPairedEventsNotCounted 2/2 PASS (verifier-run, address-only fetches, pairedSeen>0 asserted, 1:1 zero-address-Transfer-to-row invariant); DB shows 41 genuine rows from 3,728 fetched Transfers |
| 4 | SC4: operator can inspect exact raw amount and chain provenance; ordinary transfers do not appear | ✓ VERIFIED | README §5 SQL + verifier's read-only query; TestExactUint256RoundTrip (2^256-1 exact decimal string) PASS; zero ordinary-transfer rows in table |

### Plan 01-01 Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | build/vet/test green with no ETH_RPC_URL and no CLICKHOUSE_URL (storage skips cleanly) | ✓ VERIFIED | Verifier ran `env -u ETH_RPC_URL -u CLICKHOUSE_URL go build ./... && go vet ./... && go test ./... -count=1` -> exit 0, all packages `ok` (storage skips are env-gated, so server state is irrelevant to the skip) |
| 2 | Five pinned Mainnet fixtures classify with exact amounts | ✓ VERIFIED | `TestDecodePinnedMainnetFixtures` (5 subtests, exact big.Int comparisons) green in verifier's offline run; re-confirmed on-chain by verifier's TestLive run |
| 3 | USDT zero-address Transfer / USDC ordinary Transfer / USDC paired Mint-Burn yield no supply row | ✓ VERIFIED | `TestDecodeNegativesYieldNoSupplyRow` (4 subtests) green |
| 4 | Both-zero USDC Transfer rejected with an error | ✓ VERIFIED | `TestDecodeMalformedAndMismatchAreErrors` "from and to are both the zero address" green |
| 5 | Wrong data length -> error naming event and expected length, never zero amount | ✓ VERIFIED | Same test: Issue 31-byte and DestroyedBlackFunds 32-byte cases assert named errors |
| 6 | Probe aborts on wrong chain / past confirmation boundary (exact) / failing sample getLogs | ✓ VERIFIED | `TestProbeRejectsWrongChain`, `TestProbeConfirmationBoundaryExactness` (980 accepted, 981 rejected), `TestProbeRejectsProviderWithoutHistoricalLogs` green |
| 7 | No ETH_RPC_URL -> non-zero exit naming ETH_RPC_URL | ✓ VERIFIED | Verifier ran `env -u ETH_RPC_URL go run ./cmd/indexer -from 100 -to 200` -> exit 1, message names ETH_RPC_URL |
| 8 | Zero-log range ingests zero rows, exits 0 | ✓ VERIFIED | Verifier ran block 3,000,000 (pre-deployment era) live: exit 0, "stored 0 supply events" (first attempt hit a transient provider throttle and exited 1 — consistent with README's documented sustained-throttling limit; paced retry succeeded) |
| 9 | Exact round-trip incl. 2^256-1, no float anywhere in the path | ✓ VERIFIED | `TestExactUint256RoundTrip` PASS against the running ClickHouse 26.8 container; float grep over the ingest path clean (only the `Decimals` field name) |
| 10 | Duplicate insert leaves FINAL count at 1 | ✓ VERIFIED | `TestReplayDuplicateInsertKeepsFinalCountAtOne` PASS |
| 11 | Inspect deterministically ordered by block_number then log_index | ✓ VERIFIED | `TestInspectOrderedByBlockThenLogIndex` PASS |

### Plan 01-02 Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Live: five pinned fixtures classify through real eth_getLogs with exact values (backstop) | ✓ VERIFIED | Verifier ran `go test ./internal/indexer -run TestLive -v -count=1` through the operator's provider: 5/5 subtests PASS, none skipped — directly observed, not SUMMARY-sourced |
| 2 | Live: address-only USDC fetch, exactly one supply row per fixture, paired Mint/Burn produce none end-to-end (backstop) | ✓ VERIFIED | 2/2 subtests PASS in the same run; the test fatals if no paired logs reached decode (non-vacuous proof) |
| 3 | Live: bounded recent-range run stores rows with all provenance populated, no ordinary-transfer rows (backstop) | ✓ VERIFIED | Orchestrator-executed smoke this session (3,728 -> 41 rows) plus verifier's independent read-only FINAL query confirming the 41 rows with all provenance columns populated and correct zero-address semantics |
| 4 | README documents docker run, env vars, ingest flags, copy-paste FINAL inspection | ✓ VERIFIED | README §2-§5 read: 127.0.0.1-bind docker run, ETH_RPC_URL/CLICKHOUSE_URL, -config/-from/-to with confirmation rule, SELECT ... FINAL |
| 5 | README and live_test.go contain no provider URL or credential values | ✓ VERIFIED | Repo-wide `git grep` for provider names/key/proxy literals: only placeholder prose (provider names as words, no URLs, no keys); live_test.go gates on env and scrubs error output |

**Score:** 16/16 truths verified (0 present-but-behavior-unverified)

### Deferred Items

| # | Item | Addressed In | Evidence |
|---|------|-------------|----------|
| 1 | WR-01: stored block_hash comes from a by-number header fetch, not the log's own BlockHash — provenance can mismatch under reorg | Phase 2 | Phase 2 goal "reorg safety"; SC4 "checkpoint hash mismatch stops ordinary reporting until a verified rewind excludes orphaned events" (SYNC-05) |
| 2 | Provider 10k-result eth_getLogs cap forces ~40-block USDC windows; chunked sub-range fetching absent | Phase 2 | Phase 2 SC1 "advances in bounded ranges only ... and continues as new eligible blocks arrive" (SYNC-01) |

### Required Artifacts

All 13 declared artifacts exist, are substantive (full file reads), and are wired. Note: `verify.artifacts` parsed 0/0 because these plans declare artifacts as plain string lists rather than `{path, provides}` objects — verification was performed manually instead.

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `cmd/indexer/main.go` | Bounded CLI wiring the full path | ✓ VERIFIED | 174 lines; Load->RPCURL->Probe->FetchLogs->Header->Decode->InsertEvents->Inspect all present and ordered |
| `config/tokens.json` | Public token metadata | ✓ VERIFIED | chain_id 1, conf 20, USDT/USDC verbatim from requirements |
| `internal/config/config.go` | Config + env lookups + redaction | ✓ VERIFIED | 100 lines; Validate enforces all five plan invariants |
| `internal/rpc/client.go` | Probe/FetchLogs/Header + rate-limit retry | ✓ VERIFIED | 145 lines; HostOnly on every constructed error |
| `internal/indexer/decoder.go` | Strict-shape classifier | ✓ VERIFIED | 140 lines; pinned topic0s, exact per-token rules |
| `internal/storage/clickhouse.go` | Store: EnsureSchema/InsertEvents/Inspect | ✓ VERIFIED | 133 lines; *big.Int end to end, FINAL reads |
| `migrations/clickhouse.sql` | stablecoin_events DDL | ✓ VERIFIED | ReplacingMergeTree(created_at), ORDER BY full event identity, raw_amount UInt256 |
| `internal/indexer/decoder_test.go` | Fixture/negative/error proofs | ✓ VERIFIED | 358 lines; wired to real decoder |
| `internal/rpc/client_test.go` | Probe behavior proofs | ✓ VERIFIED | 189 lines; canned JSON-RPC server incl. throttle injector |
| `internal/storage/clickhouse_test.go` | Exactness/idempotence proofs | ✓ VERIFIED | 166 lines; env-gated skip with documented docker command |
| `internal/indexer/live_test.go` | Live chain proofs | ✓ VERIFIED | 362 lines; env-gated, paced, scrubbed |
| `README.md` | Operator runbook | ✓ VERIFIED | 133 lines; all six required sections |
| `go.mod`/`go.sum` | Pinned deps | ✓ VERIFIED | go-ethereum v1.17.7, clickhouse-go/v2 v2.48.0 committed |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| cmd/indexer/main.go | full pipeline | Load->RPCURL->Probe->FetchLogs(SupplyTopics)->Header->DecodeSupplyEvent->InsertEvents->Inspect->stdout | ✓ WIRED | Code read: probe at L70 precedes first FetchLogs at L88; inspect table printed L158-166; behavior confirmed by verifier's live runs |
| internal/storage | migrations/clickhouse.sql | EnsureSchema (CREATE TABLE IF NOT EXISTS) before first insert | ✓ WIRED | main.go L143 EnsureSchema precedes L147 InsertEvents; TestEnsureSchemaIsIdempotent PASS |
| internal/rpc errors | config.HostOnly | every error message renders scheme://host | ⚠️ PARTIAL | Every *constructed* message prefixes HostOnly, but `%w`-wrapped net/http `*url.Error` transport failures can still embed the full credential-bearing URL — admitted in 01-02-SUMMARY Deviation 4; only the test side is scrubbed. Escalated to human verification (credential-leak residual) |
| live_test.go | rpc.Client.FetchLogs + DecodeSupplyEvent | same code path as cmd/indexer | ✓ WIRED | Package indexer_test breaks the import cycle; verifier's TestLive run exercised it |
| bounded smoke | stablecoin_events table | cmd/indexer main flow end to end | ✓ WIRED | 41 rows in table with full provenance (verifier-queried) |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|-------------------|--------|
| cmd/indexer inspect output | inspected []EventRow | `SELECT ... FROM stablecoin_events FINAL` | Yes | ✓ FLOWING |
| stablecoin_events.raw_amount | EventRow.RawAmount (*big.Int) | eth_getLogs data -> SetBytes -> batch.Append | Yes | ✓ FLOWING (exact decimal round-trip incl. 2^256-1) |
| block_hash/block_time | types.Header | HeaderByNumber per distinct fetched block | Yes | ✓ FLOWING |
| event_type/from/to | DecodeSupplyEvent | real log topics/data | Yes | ✓ FLOWING (live-verified against 5 pinned receipts) |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Clean-env build/vet/test | `env -u ETH_RPC_URL -u CLICKHOUSE_URL go build ./... && go vet ./... && go test ./... -count=1` | exit 0, all `ok` | ✓ PASS |
| Fail-fast without RPC URL | `env -u ETH_RPC_URL go run ./cmd/indexer -from 100 -to 200` | exit 1, stderr names ETH_RPC_URL | ✓ PASS |
| Live fixture classification | `go test ./internal/indexer -run TestLive -v -count=1` (operator provider) | 10/10 subtests PASS, 0 SKIP | ✓ PASS |
| Zero-log bounded run exits 0 | `go run ./cmd/indexer -from 3000000 -to 3000000` | exit 0, "stored 0 supply events" | ✓ PASS (first attempt hit a transient throttle, exit 1 — documented sustained-throttling limit, retry succeeded) |
| ClickHouse integration suite | `CLICKHOUSE_URL=clickhouse://default@127.0.0.1:9000/default go test ./internal/storage -v -count=1` | 5/5 PASS, 0 SKIP | ✓ PASS |
| Stored rows inspectable read-only | `docker exec tm-clickhouse clickhouse-client --query "SELECT ... FINAL ..."` | 41 USDC rows (21 mint/20 burn) + 7 storage-test rows, full provenance | ✓ PASS |

### Probe Execution

No `probe-*.sh` scripts are declared by the plans or exist in the repo (`scripts/` absent); the phase's runnable checks are the Go suites above. N/A.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| CHAIN-01 | 01-01, 01-02(D4) | External RPC URL + token metadata + confirmation policy configurable without business-logic changes | ✓ SATISFIED | env-only ETH_RPC_URL (never in source — grep clean), tokens.json, Validate, probe enforces the policy; README documents all of it |
| EVENT-01 | 01-01, 01-02 | USDT Issue->mint, Redeem/DestroyedBlackFunds distinct, verified against deployed transactions | ✓ SATISFIED | Offline fixture table + verifier-run live classifications of all three pinned USDT receipts with exact amounts |
| EVENT-02 | 01-01, 01-02 | USDC zero-address Transfer mint/burn without double-counting paired Mint/Burn | ✓ SATISFIED | Offline negatives + verifier-run live address-only proofs (paired logs present, zero rows from them) |
| EVENT-03 | 01-01, 01-02 | Inspectable stored events with exact raw amount and provenance; ordinary transfers excluded | ✓ SATISFIED | Schema columns, Inspect/README SQL, verifier's read-only query, 41 genuine rows / zero ordinary transfers |

Orphaned requirements: none — REQUIREMENTS.md maps exactly CHAIN-01, EVENT-01..03 to Phase 1 and the plans claim exactly those.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| internal/rpc/client.go | 79,87,93,107,128,142 | `%w`-wrapped transport errors can carry the credential-bearing URL past the HostOnly prefix | ⚠️ Warning | Credential may reach stderr logs on transport failures (D-01/T-01-02 residual); escalated to human decision |
| cmd/indexer/main.go | 151 | Post-ingest Inspect(20) shows the 20 oldest table rows, not the just-ingested range (review WR-02) | ⚠️ Warning | Operator UX: a fresh range's rows may not appear when older rows exist; deterministic ordering itself holds |
| internal/rpc Probe | 96 | cfg.Tokens[0] indexed without a guard (review IN-03); Validate guarantees >=1 token, but an empty-symbol token is skipped at fetch time rather than rejected at startup (review WR-03) | ⚠️ Warning | A typo'd symbol in tokens.json silently ingests nothing for that token |
| (code review 01-REVIEW.md) | — | 3 warnings + 8 info, all disposition `open` in 01-REVIEW-DISPOSITION.md | ℹ️ Info | WR-01 (reorg block_hash) deferred to Phase 2; remainder are quality advisories, none break a must-have |

Debt markers (TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER): none in any phase file. Disabled/skipped tests: none (skips are env-gates that verifiably run when env is set). Circular tests: none — expected values originate from independent research (block-explorer capture in 01-RESEARCH.md) and are re-verified against live chain responses, an external oracle. Credential scan of all tracked files: clean (only placeholder provider-name prose).

### Decision Coverage

2/2 CONTEXT.md decisions honored (D-01 env-only credentials, D-02 fail-fast probe) — gate message: "All trackable CONTEXT.md decisions are honored by shipped artifacts."

### Human Verification Required

1. **README walk-through (plan-declared D5)** — reproduce the slice from README.md alone.
2. **Credential-leak residual** — decide whether to scrub `%w`-wrapped RPC errors in production or accept the Phase 1 residual. The prohibition "never embed RPC URLs or credentials in ... logs" is therefore **flagged as unverified-prohibition — human review recommended** (tracked-file portion verified clean by grep; runtime-log portion has a documented leak path).
3. **Research item A4** — locate or rule out a deployed USDT zero-address Transfer negative fixture on chain.
4. **MVP goal format** — reformat the ROADMAP goal into user-story format (`/gsd-mvp-phase 1`) so MVP-mode UAT generation works as designed.

### Gaps Summary

No must-have truth, artifact, or key link failed. All 16 plan truths and all 4 roadmap success criteria are verified with behavior-level evidence the verifier ran itself or observed directly (live provider tests, live zero-log run, ClickHouse integration suite, read-only table inspection). The phase goal is achieved in the codebase. The `human_needed` status comes from four escalation items: the plan-deferred README usability judgment, the flagged credential-leak residual in wrapped RPC error logs, the open A4 research item, and the ROADMAP goal-format discrepancy under MVP mode — none of which block the goal, all of which need a human decision.

---

_Verified: 2026-10-08T11:45:13Z_
_Verifier: Claude (gsd-verifier)_
