# Phase 1: Live Supply Event Path - Research

**Researched:** 2026-10-08
**Domain:** Ethereum Mainnet contract logs to exact ClickHouse supply events
**Confidence:** MEDIUM (contract receipts and log filters verified live; dependency install and database round trip remain execution checks)

## User Constraints

No phase CONTEXT.md exists; the orchestrator reports that the user approved planning from requirements and research. [VERIFIED: .planning/ROADMAP.md:16-25] Phase 1's exact goal is: "Operators can ingest and inspect correctly classified, real USDT and USDC supply-changing logs in ClickHouse." The requirements assigned are `CHAIN-01`, `EVENT-01`, `EVENT-02`, `EVENT-03`. [VERIFIED: .planning/ROADMAP.md:16-25]

From AGENTS.md, honor Ethereum Mainnet USDT/USDC only; Go indexer, ClickHouse, Grafana, and eventual Docker Compose; externally configured RPC without embedded credentials; retry/restart/duplicate/reorg correctness; and configurable contract metadata, confirmation depth, and large-event thresholds. [VERIFIED: AGENTS.md:11-17] Phase 1 proves the bounded event path; continuous sync, checkpoints, replay/reorg repair, supply anchors, and Grafana panels have their own later phases. [VERIFIED: .planning/ROADMAP.md:16-47]

<phase_requirements>
## Phase Requirements

| ID | Description (verbatim) | Research Support |
|----|------------------------|------------------|
| CHAIN-01 | Operator can configure an external Ethereum Mainnet HTTP RPC endpoint and the USDT/USDC contract addresses, decimals, and confirmation policy without changing business logic. | Standard-library JSON plus `ETH_RPC_URL`; validate addresses/decimals and inspect eligible head. |
| EVENT-01 | Indexer recognizes USDT `Issue` as mint and `Redeem` and `DestroyedBlackFunds` as distinct supply-decrease reasons, verified against deployed-contract transactions. | Exact topics, data layout, and three live Mainnet receipts below. |
| EVENT-02 | Indexer recognizes USDC mint and burn from zero-address `Transfer` logs, verified against deployed-contract transactions, without also counting paired `Mint`/`Burn` logs. | Two live receipts contain both paired event families; select only Transfer. |
| EVENT-03 | Operator can inspect each stored supply event's token, source event, direction/reason, exact raw amount, block number/hash/time, transaction hash, and log index; ordinary transfers do not affect issuance. | `UInt256` amount, block header timestamp, log provenance, and inspection query. |
</phase_requirements>

These four descriptions are copied verbatim from `.planning/REQUIREMENTS.md` lines 12 and 17-19. [VERIFIED: .planning/REQUIREMENTS.md:10-19]

## Summary

The issuer contracts need two explicit decoder paths. Tether's source changes supply and emits `Issue(uint256)`, `Redeem(uint256)`, or `DestroyedBlackFunds(address,uint256)`; the verified deployed-address receipts below contain those exact logs. Circle documents paired `Mint`/`Burn` and zero-address `Transfer` events; deployed USDC receipts confirm each pair, so only the Transfer member enters the supply ledger. [CITED: https://github.com/tethercoin/USDT/blob/main/TetherToken.sol] [CITED: https://github.com/circlefin/stablecoin-evm/blob/master/doc/tokendesign.md] [VERIFIED: Mainnet eth_getTransactionReceipt and eth_getLogs probes, 2026-10-08]

Use bounded `eth_getLogs` filters by configured contract and relevant topic, decode raw `uint256` into Go `math/big.Int`, fetch canonical block headers for timestamps, and persist inspectable event rows in ClickHouse `UInt256`. The exact driver-to-server round trip is a Phase 1 implementation gate, because neither client dependencies nor ClickHouse is installed in this repository. [CITED: https://pkg.go.dev/github.com/ethereum/go-ethereum/ethclient] [CITED: https://clickhouse.com/docs/integrations/language-clients/go/data-types] [VERIFIED: go.mod:1-3]

**Primary recommendation:** Build one bounded, operator-invoked Mainnet range command and one contract-specific decoder per token; test each fixture block separately before broad historical ranges. [ASSUMED: recommended implementation scope]

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|--------------|----------------|-----------|
| Config and bounded range command | Go indexer | Operator environment | The process validates operator input before RPC/storage work. [ASSUMED: architecture recommendation] |
| Fetch and classify logs | Go indexer | Ethereum RPC | RPC returns evidence; token-specific code assigns the supply meaning. [CITED: https://ethereum.org/developers/docs/apis/json-rpc/] |
| Exact event persistence and inspection | ClickHouse | Go writer | The database stores raw units and chain provenance; SQL makes them inspectable. [CITED: https://clickhouse.com/docs/reference/data-types/int-uint] |
| Historical fixture verification | Live Ethereum RPC | Go tests | The selected provider must serve the old fixture blocks; explorer pages alone do not prove RPC compatibility. [VERIFIED: provider probes, 2026-10-08] |

## Project Constraints (from AGENTS.md)

- Mainnet USDT and USDC only; no extra chain or token path. [VERIFIED: AGENTS.md:13-13]
- Keep Go/ClickHouse/Grafana/Compose as the required overall stack. [VERIFIED: AGENTS.md:14-14]
- Supply the RPC endpoint externally; do not put URLs or credentials in source. [VERIFIED: AGENTS.md:15-15]
- Preserve correctness across retries, restarts, duplicate logs, and short reorgs as later phases extend the path. [VERIFIED: AGENTS.md:16-16]
- Keep token metadata, confirmation depth, and large-event thresholds configurable. [VERIFIED: AGENTS.md:17-17]
- Standard `gofmt`; the exact module declaration is `module TOkenMonitor` and `go 1.27.1`, with no current dependencies or Go source. [VERIFIED: go.mod:1-3] [VERIFIED: AGENTS.md:25-54]
- Begin file-changing work through a GSD command; this research was delegated by the active plan-phase workflow. [VERIFIED: AGENTS.md:193-203]

## Standard Stack

| Component | Version | Use | Evidence / caveat |
|-----------|---------|-----|-------------------|
| Go | 1.27.1 | Indexer, `encoding/json`, `math/big`, `context`, `log/slog` | Exact directive: `go 1.27.1`; local `go version go1.27.1 linux/amd64`. [VERIFIED: go.mod:3] [VERIFIED: local go version, 2026-10-08] |
| `github.com/ethereum/go-ethereum` | Pin v1.17.7 for this phase | `ethclient.FilterLogs`, `HeaderByNumber`, `ChainID`; `ethereum.FilterQuery`, `types.Log` | Version published Sep 30, 2026 and official Go package API checked. The package page says this version is not latest; deliberately pin this researched version and rerun module resolution. [CITED: https://pkg.go.dev/github.com/ethereum/go-ethereum@v1.17.7/ethclient] |
| ClickHouse Server | Pin a concrete 26.8 LTS patch at execution | Exact raw event table and inspection | Prior project stack research selected the 26.8 LTS line; server/image availability and driver compatibility still need a smoke test. [VERIFIED: .planning/research/STACK.md:13-22] |
| `github.com/ClickHouse/clickhouse-go/v2` | Pin v2.48.0 for this phase | Native batch insert and `UInt256`/`big.Int` round trip | Version published Aug 4, 2026 and the official client docs show `*big.Int` for `UInt256`. The version page says this is not latest; pinning avoids untested drift. [CITED: https://pkg.go.dev/github.com/ClickHouse/clickhouse-go/v2@v2.48.0] [CITED: https://clickhouse.com/docs/integrations/language-clients/go/data-types] |

Use `go get github.com/ethereum/go-ethereum@v1.17.7 github.com/ClickHouse/clickhouse-go/v2@v2.48.0` then `go mod tidy`, with a reachable Go module proxy. The attempted `go list -m -json ...@latest` requests timed out against `proxy.golang.org` here, so local package resolution and compilation are unverified. [VERIFIED: local go list probe, 2026-10-08] [ASSUMED: exact install command pending module proxy access]

### Package Legitimacy Audit

The GSD legitimacy command accepts only `npm|pypi|crates`; an actual `--ecosystem go` probe returned its usage error. Do not manufacture an `OK` verdict for Go modules. Both package paths are linked from the official Ethereum/ClickHouse sources above and resolve on pkg.go.dev; the executor must check `go mod download`, `go mod verify`, and the checked-in `go.sum` when network access works. [VERIFIED: local package-legitimacy probe, 2026-10-08] [CITED: https://pkg.go.dev/github.com/ethereum/go-ethereum@v1.17.7/ethclient] [CITED: https://clickhouse.com/integrations/go]

## Deployed Contract Evidence

The in-repo contract metadata and proposed policy value are quoted verbatim below. These are configurable defaults, not values embedded in decoder logic. [VERIFIED: requirements.md:66-69] [VERIFIED: requirements.md:385-391]

```text
| USDT  | Tether | `0xdAC17F958D2ee523a2206206994597C13D831ec7` |        6 |
| USDC  | Circle | `0xA0b86991c6218b36c1d19d4a2e9eb0ce3606eb48` |        6 |
confirmation_blocks = 20
```

The source specification also names the environment key verbatim as `ETH_RPC_URL=`. [VERIFIED: requirements.md:112-112]

Event `topic0` is Keccak-256 of the canonical event signature. `web3_sha3` on a Mainnet RPC returned the hashes below; source declarations establish field layout, and deployed receipts establish that the configured contracts emit them. [VERIFIED: Mainnet web3_sha3 probes, 2026-10-08] [CITED: https://github.com/tethercoin/USDT/blob/main/TetherToken.sol] [CITED: https://github.com/circlefin/stablecoin-evm/blob/master/doc/tokendesign.md]

| Contract event | topic0 | Decode / supply action |
|----------------|--------|------------------------|
| USDT `Issue(uint256)` | `0xcb8241adb0c3fdb35b70c24ce35c5eb0c17af7431c99f827d44a445ca624176a` | Exactly one 32-byte non-indexed amount word; mint. [VERIFIED: Mainnet Issue receipt below] |
| USDT `Redeem(uint256)` | `0x702d5967f45f6513a38ffc42d6ba9bf230bd40e8f53b16363c7eb4fd2deb9a44` | Exactly one 32-byte non-indexed amount word; redeem supply decrease. [VERIFIED: Mainnet Redeem receipt below] |
| USDT `DestroyedBlackFunds(address,uint256)` | `0x61e6e66b0d6339b2980aecc6ccc0039736791f0ccde9ed512e789a7fbdd698c6` | Two non-indexed 32-byte words: blacklisted address then raw balance; distinct destroyed-funds supply decrease. [VERIFIED: Mainnet Destroyed receipt below] |
| USDC `Transfer(address,address,uint256)` | `0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef` | `topics[1]` = indexed `from`, `topics[2]` = indexed `to`, data = 32-byte amount. Zero `from` only = mint; zero `to` only = burn; ordinary transfer = ignore. [VERIFIED: Mainnet USDC receipts below] |
| USDC paired `Mint(address,address,uint256)` / `Burn(address,uint256)` | `0xab8530f87dc9b59234c4623bf917212bb2536d647574c8e7e5da92c2ede0c9f8` / `0xcc16f5dbb4873280815c1ee09dbd06736cffcc184412cf7a71a0fdb75d397ca5` | Evidence only; do not insert as additional supply changes. [VERIFIED: Mainnet USDC receipts below] |

These fixture rows were read directly from successful Mainnet `eth_getTransactionReceipt` responses on 2026-10-08. `eth_getLogs` at their exact blocks independently returned the USDT `Issue`, `Redeem`, and `DestroyedBlackFunds` logs and the USDC zero-address Transfer logs; an initial public endpoint rate-limited one query, which succeeded on another endpoint. Fixture data below is historical chain evidence, not a claim that every future provider will serve the blocks. [VERIFIED: live RPC receipt and log probes, 2026-10-08]

| Expected row | Block | Transaction hash | Log index | Raw amount | Corroboration |
|--------------|-------|------------------|-----------|------------|---------------|
| USDT Issue, mint | `0x167ab1a` | `0x3a296e7b8289666b5ea54e48d3da404fc0b337cc4d419276b5675395244b115c` | `0x249` | `1000000000000000` | Receipt has one USDT Issue log. [VERIFIED: Mainnet receipt/log probes, 2026-10-08] |
| USDT Redeem, supply decrease | `0x1726ef7` | `0x8642010ea16fda150d6db81b7e5a377453d825021452cc076a2fb6fbda524e1c` | `0x2ca` | `3000000000000000` | Receipt has one USDT Redeem log. [VERIFIED: Mainnet receipt/log probes, 2026-10-08] |
| USDT DestroyedBlackFunds, distinct decrease | `0x15aac45` | `0x06abbfaa405699132123d74e1cdf712e42bf88fff00ed83b8a9a0a9f112c6055` | `0x277` | `200000000000` | Data first word is `0x532961b23a28a9e5a8b1769bdb3e8906fc7c7a01`; second word is amount. [VERIFIED: Mainnet receipt/log probes, 2026-10-08] |
| USDC mint Transfer | `0x1632fcb` | `0xc7066b502fa8321a090fbff8842f69d136e2d4c27849a04cbc1ed02cac98963e` | `0x2ad` | `2000000000000` | Same receipt also contains `Mint` at `0x2ac` with the same amount; `topics[1]` on Transfer is zero. [VERIFIED: Mainnet receipt/log probes, 2026-10-08] |
| USDC burn Transfer | `0x1638e19` | `0x4f52e25d39e006606f26de5a496eb7f9198c8b8467686f0a994d07d7a2a4ca9f` | `0x162` | `1383937060000` | Same receipt also contains `Burn` at `0x161` with the same amount; `topics[2]` on Transfer is zero. [VERIFIED: Mainnet receipt/log probes, 2026-10-08] |

The USDT fixture receipts include no USDT Transfer alongside those three supply changes. This corroborates the source-level rule for those transactions only; a separate zero-address USDT Transfer fixture was not proved during this research. Require a decoder test with a crafted zero-address USDT Transfer, and seek a deployed transaction fixture before calling that negative case chain-verified. [VERIFIED: Mainnet receipt probes, 2026-10-08] [ASSUMED: deployed zero-address USDT Transfer fixture]

## Architecture Patterns

```text
operator config + bounded [from,to]
          |
          v
Go command validates config and eligible head
          |
          v
Ethereum HTTP RPC eth_getLogs, token address + topic0 filters
          |
          +--> USDT Issue/Redeem/DestroyedBlackFunds decoder
          +--> USDC Transfer decoder -> zero-address decision -> ignore ordinary
          |
          v
header lookup: hash + timestamp; preserve tx hash + log index
          |
          v
ClickHouse exact raw-event insert -> SQL inspection of stored rows
```

Use a small `cmd/indexer` entry point for configuration and a bounded run, a token decoder, an Ethereum reader, and a ClickHouse writer. Keep the current `TOkenMonitor` module spelling until a deliberate module rename; the source specification's file tree is proposed, not an implemented API. [VERIFIED: go.mod:1-3] [VERIFIED: requirements.md:776-832] [ASSUMED: recommended minimal package structure]

1. Query separately by contract and token topic set. Ethereum RPC `address` and `topics` filters narrow results; OR alternatives in the first topic position select relevant event signatures. Never query all ordinary USDT transfers merely to infer supply. [CITED: https://ethereum.org/developers/docs/apis/json-rpc/] [CITED: https://github.com/tethercoin/USDT/blob/main/TetherToken.sol]
2. Validate `Address`, topic count, exact data length, and the zero-address direction before constructing a row. Preserve `topic0`, source event, direction/reason, raw amount, contract, block number/hash, transaction hash, log index, and block timestamp. The RPC log already carries provenance, while the block header supplies timestamp portably. [CITED: https://pkg.go.dev/github.com/ethereum/go-ethereum/core/types] [CITED: https://ethereum.org/developers/docs/apis/json-rpc/]
3. Store amount as ClickHouse `UInt256`; Go `*big.Int` is documented for the driver. Store direction/reason separately so raw unsigned amounts remain nonnegative. The source SQL example already has `raw_amount UInt256` beside `amount Decimal(38, 6)`; treat only the former as the exact ledger value, and do not use `Float64`. `UInt256` covers all Solidity `uint256`, while Decimal precision tops out at 76 digits. [VERIFIED: requirements.md:249-250] [CITED: https://clickhouse.com/docs/reference/data-types/int-uint] [CITED: https://clickhouse.com/docs/reference/data-types/decimal] [CITED: https://clickhouse.com/docs/integrations/language-clients/go/data-types]
4. For Phase 1, expose a simple `SELECT` over stored raw events ordered by block and log index. If choosing `ReplacingMergeTree` now for later replay, use a stable log identity and make all logical reads use `FINAL`; background replacement is explicitly eventual. Canonical orphan exclusion and checkpoint advancement remain Phase 2. [CITED: https://clickhouse.com/docs/reference/engines/table-engines/mergetree-family/replacingmergetree] [VERIFIED: .planning/ROADMAP.md:28-37]

### Code Examples

The official Go client exposes these APIs; the following is an implementation sketch with values supplied by validated config. [CITED: https://pkg.go.dev/github.com/ethereum/go-ethereum@v1.17.7/ethclient] [ASSUMED: sketch not compiled in this session]

```go
q := ethereum.FilterQuery{
    FromBlock: new(big.Int).SetUint64(from),
    ToBlock:   new(big.Int).SetUint64(to),
    Addresses: []common.Address{tokenAddress},
    Topics:    [][]common.Hash{eventTopics},
}
logs, err := rpcClient.FilterLogs(ctx, q)
```

Decode the amount only after checking `len(log.Data) == 32` for USDT Issue/Redeem or USDC Transfer and `len(log.Data) == 64` for USDT DestroyedBlackFunds; its amount occupies `log.Data[32:64]`. Require one topic for each USDT event and three for USDC Transfer. A source mismatch is an error, not a zero amount. [VERIFIED: live Mainnet receipt layouts, 2026-10-08]

```go
amount := new(big.Int).SetBytes(log.Data[offset : offset+32])
// Persist amount as UInt256 via the ClickHouse native batch API.
```

ClickHouse's official Go example uses `conn.PrepareBatch(ctx, "INSERT INTO example")`, `batch.Append(*big.Int)`, and `batch.Send()`, then scans `UInt256` into `big.Int`. Use that pattern and verify a row containing the maximum 256-bit value plus a real fixture amount round trips exactly. [CITED: https://clickhouse.com/docs/integrations/language-clients/go/data-types]

## Don't Hand-Roll

| Problem | Use | Why |
|---------|-----|-----|
| Ethereum RPC and log types | `go-ethereum/ethclient`, `ethereum.FilterQuery`, `types.Log` | Typed chain provenance and standard topic filtering. [CITED: https://pkg.go.dev/github.com/ethereum/go-ethereum/ethclient] |
| Keccak topic hashing | go-ethereum `crypto.Keccak256Hash` or pinned verified topic constants | SHA3-256 is not Keccak-256; `web3_sha3` produced the verified topic hashes. [VERIFIED: live Mainnet web3_sha3 probes, 2026-10-08] |
| 256-bit arithmetic | Go `math/big`; ClickHouse `UInt256` | Full on-chain amount range and exact round trip. [CITED: https://clickhouse.com/docs/reference/data-types/int-uint] [CITED: https://clickhouse.com/docs/integrations/language-clients/go/data-types] |
| Config parsing and logging | Go `encoding/json`, `os.Getenv`, `log/slog` | No need to add a YAML or logging dependency for the first executable. [ASSUMED: implementation recommendation] |

## Common Pitfalls

| Pitfall | Check at implementation gate |
|---------|------------------------------|
| Treat USDT zero-address Transfer as issuance | Only its three supply topics may construct a USDT supply row; fixture receipts and source show those paths. [CITED: https://github.com/tethercoin/USDT/blob/main/TetherToken.sol] |
| Count USDC paired `Mint`/`Burn` as extra rows | The mint and burn receipts each have a matching zero-address Transfer; expect one row per receipt. [VERIFIED: Mainnet receipt probes, 2026-10-08] |
| Decode `DestroyedBlackFunds` as a single word or indexed address | Deployed receipt has one topic and 64 data bytes; second word is amount. [VERIFIED: Mainnet receipt probe, 2026-10-08] |
| Use a provider that returns receipts but refuses historical `eth_getLogs` | This was observed: PublicNode returned `-32602 Archive requests require a personal token`; dRPC served the receipt but its historical log query returned code 27; a separate public endpoint returned `-32029 Too Many Requests`. Require a successful exact-block log probe before acceptance. [VERIFIED: live provider probes, 2026-10-08] |
| Lose raw precision in ClickHouse | The source SQL example separates `raw_amount UInt256` from `amount Decimal(38, 6)`; assert exact `UInt256` round trips and never pass float for raw units. [VERIFIED: requirements.md:249-250] [CITED: https://clickhouse.com/docs/reference/data-types/int-uint] |
| Infer an absolute supply from the bounded sample | Phase 1 event rows are deltas only; an anchor and same-block `totalSupply` comparison belong to Phase 3. [VERIFIED: .planning/ROADMAP.md:40-47] |

## Environment Availability

| Dependency | Available here | Implication |
|------------|----------------|-------------|
| Go | `go1.27.1` | Local compiler matches `go.mod`; module proxy timed out on both `go list -m @latest` probes. [VERIFIED: local probes, 2026-10-08] |
| Docker daemon | Docker 29.1.3 and daemon reachable | Can run a pinned ClickHouse container for integration tests. [VERIFIED: local probes, 2026-10-08] |
| Docker Compose V2 plugin | `docker compose version` returned `unknown command` | Phase 4 deployment needs the plugin; Phase 1 can use `docker run` for a ClickHouse smoke test. [VERIFIED: local probes, 2026-10-08] |
| ClickHouse binary/server | Not found on PATH | Start the pinned container for Phase 1 storage tests. [VERIFIED: local command probe, 2026-10-08] |
| Configured RPC URL | `ETH_RPC_URL` absent | Operator supplies a provider with historical log access; public URLs used in this research are probes, not application defaults. [VERIFIED: local environment probe, 2026-10-08] |
| `ctx7`/Context7 MCP | Neither available | Official project docs and pkg.go.dev were used for library API checks. [VERIFIED: tool availability probe, 2026-10-08] |

## Validation Architecture

| Property | Value |
|----------|-------|
| Framework | Go standard `testing` package; no test infrastructure currently exists. [VERIFIED: go.mod:1-3] |
| Quick run | `go test ./...` after packages and dependencies are added. [ASSUMED: planned command] |
| Full gate | `go test ./...` plus bounded live-RPC and ClickHouse integration run. [ASSUMED: planned command] |

| Requirement | Smallest meaningful check |
|-------------|---------------------------|
| CHAIN-01 | Parse config containing the quoted contract metadata and a non-default confirmation depth; refuse missing URL, bad address, and invalid range. [VERIFIED: requirements.md:66-89] [ASSUMED: test design] |
| EVENT-01 | Table test the three decoded USDT receipt logs; live probe `eth_getLogs` at each fixture block through the operator's URL, then inspect three stored rows. [VERIFIED: Mainnet fixtures above] |
| EVENT-02 | Table test both USDC pairs; only the zero-address Transfer is inserted, once per action. [VERIFIED: Mainnet fixtures above] |
| EVENT-03 | ClickHouse insert/read of real fixture amount and `2^256-1`, checking exact decimal strings and all provenance columns; ordinary USDC transfer and synthetic USDT zero-address Transfer produce zero supply rows. [CITED: https://clickhouse.com/docs/reference/data-types/int-uint] [ASSUMED: test design] |

Wave 0 needs a decoder test file and a storage integration test or one executable smoke command; do not add a test framework. Live tests must be opt-in when credentials or server access are unavailable, while unit fixture tests run offline. [ASSUMED: recommended test structure]

## Security Domain

`security_enforcement` is enabled in `.planning/config.json`. [VERIFIED: .planning/config.json:48-48] Apply the relevant ASVS categories to this command-line ingestion path; no user login/session workflow is in Phase 1. [ASSUMED: ASVS applicability mapping]

| ASVS category | Applies | Phase 1 control |
|---------------|---------|-----------------|
| V2 Authentication | RPC/database credentials only | Read secrets from environment and redact URLs in logs. [VERIFIED: AGENTS.md:15-15] [ASSUMED: implementation detail] |
| V3 Session Management | No operator session in this CLI | No session feature to design. [VERIFIED: .planning/ROADMAP.md:16-25] |
| V4 Access Control | Database writer | Use a scoped ClickHouse writer account when deployment is introduced. [ASSUMED] |
| V5 Input Validation | Yes | Validate config, address, block range, topic/data lengths, and RPC response metadata before insertion. [ASSUMED: standard control] |
| V6 Cryptography | Transport and hash topic | Use TLS RPC/database connections and go-ethereum Keccak; do not implement cryptography. [ASSUMED: standard control] |

The most relevant threat is a malicious or faulty RPC/config value causing forged or malformed supply rows; validate the configured address and the log's `Address`, and keep receipt fixtures plus block provenance for later canonical verification. [ASSUMED: threat model] Phase 2 owns durable replay/reorg exclusion. [VERIFIED: .planning/ROADMAP.md:28-37]

## Assumptions Log

| # | Claim | Risk if wrong |
|---|-------|---------------|
| A1 | JSON config and one bounded CLI command are enough for Phase 1. | Operator may require YAML or a different invocation; decoder/storage behavior is unaffected. |
| A2 | Pinned client versions compile and interoperate with ClickHouse 26.8 on this machine. | Go proxy timeout and absent ClickHouse prevented the smoke test; execute before declaring phase complete. |
| A3 | A provider with historical logs for all five fixture blocks is available to the operator. | Historical acceptance blocks until a suitable URL is configured; do not replace live proof with explorer text. |
| A4 | A deployed nonzero USDT zero-address Transfer fixture can be found. | Source and a synthetic regression cover the rule, but the chain-negative acceptance proof remains open. |

## Open Questions

Dispositions recorded 2026-10-08 during phase-plan revision: each question is gated by an executable check in the phase plans rather than answered at research time.

1. Which operator-supplied RPC URL passes `eth_getLogs` for the oldest fixture (`0x15aac45`) and the other four blocks at practical rate limits? Public endpoints behaved differently during this research. [VERIFIED: live provider probes, 2026-10-08]
   - RESOLVED-BY-PLAN: the specific URL is a run-time operator choice per D-01, validated executably rather than researched. Plan 01-01 Task 1 implements the D-02 fail-fast probe (chain ID 1 plus a sample historical `eth_getLogs` before any ingest), plan 01-02 Task 1 runs the env-gated TestLive suite against the five fixture blocks, and ETH_RPC_URL is surfaced via user_setup in both plans. If no qualifying URL is set, live tests skip and the live evidence is collected at end-of-phase verification; assumption A3 stays disclosed until then.
2. Is a nonzero deployed USDT zero-address Transfer available as a negative chain fixture? No such transaction was verified in this session; do not label a guessed hash as evidence. [ASSUMED]
   - RESOLVED-BY-PLAN: the rule is enforced executably by the crafted-fixture decoder regression in plan 01-01 Task 2 (a USDT zero-address Transfer yields no supply row). The deployed on-chain negative fixture itself remains open as assumption A4, disclosed in plan 01-02's assumptions and tracked for end-of-phase verification; no guessed hash is treated as evidence.
3. Which concrete ClickHouse 26.8 image patch passes the `UInt256`/`big.Int` round trip with pinned clickhouse-go? Neither image nor client dependency was pulled here. [ASSUMED]
   - RESOLVED-BY-PLAN: plan 01-01 Task 3 resolves the newest concrete 26.8.x tag at execution and runs the pinned container; the passing storage integration test (exact UInt256 decimal-string round trip including 2^256-1, FINAL-read replay idempotence) is the acceptance, and a failing round trip blocks the phase.

## Sources

- [TetherToken source](https://github.com/tethercoin/USDT/blob/main/TetherToken.sol) and [Circle token design](https://github.com/circlefin/stablecoin-evm/blob/master/doc/tokendesign.md): source-level event semantics. [CITED]
- [Ethereum JSON-RPC](https://ethereum.org/developers/docs/apis/json-rpc/) and [go-ethereum ethclient](https://pkg.go.dev/github.com/ethereum/go-ethereum@v1.17.7/ethclient): bounded log filters and Go APIs. [CITED]
- [ClickHouse UInt types](https://clickhouse.com/docs/reference/data-types/int-uint), [Go data types](https://clickhouse.com/docs/integrations/language-clients/go/data-types), [ReplacingMergeTree](https://clickhouse.com/docs/reference/engines/table-engines/mergetree-family/replacingmergetree): exact amount storage and read semantics. [CITED]
- Mainnet `eth_getTransactionReceipt`, `eth_getLogs`, and `web3_sha3` probes, run 2026-10-08 against public RPC endpoints; fixture transaction hashes and exact results are recorded above. [VERIFIED: live probes]

## Metadata

**Confidence breakdown:** Contract topics/layout and five receipts HIGH (live Mainnet); library APIs MEDIUM (official docs, local dependency install blocked by proxy); ClickHouse integration LOW until container round trip; provider suitability LOW until operator URL passes probes. The GSD `classify-confidence --provider websearch --verified` seam returned `MEDIUM`; the package-legitimacy seam does not support Go. [VERIFIED: local seam probes, 2026-10-08]

**Valid until:** 2026-11-07 for API/version recommendations; transaction fixtures are immutable chain observations, but provider access can change sooner. [ASSUMED: review interval]
