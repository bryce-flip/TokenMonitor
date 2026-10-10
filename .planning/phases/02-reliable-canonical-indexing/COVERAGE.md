# API Coverage — Phase 2: Reliable Canonical Indexing

External API in scope: the Ethereum JSON-RPC provider surface used by the
continuous indexer (same scope as Phase 1, extended by the Phase 2 head
policy). Decisions below start from full coverage as the default; the matrix
is the subtraction record. Format follows
`.planning/phases/01-live-supply-event-path/COVERAGE.md`.

| capability | decision | reason |
|---|---|---|
| eth_getBlockByNumber with the `finalized` block tag | INTEGRATE | D-01 head policy: the finalized tag is the default eligible head (go-ethereum serializes the tag via `HeaderByNumber`; `SupportsHeadTag` probes the capability at run start with a loud confirmed downgrade when missing — 02-02) |
| eth_getBlockByNumber with the `safe` block tag | INTEGRATE | Same consensus-tag family the head policy builds on; carried by the same code path and probed live against the devnet and three public Mainnet RPCs during research (not selected by any default policy — `finalized` is the default, `confirmed` uses latest − depth) |
| eth_chainId | INTEGRATE | Carried from Phase 1; still the fail-fast probe's first assertion, and re-run by the `rewind` subcommand before any destructive step |
| eth_blockNumber | INTEGRATE | `confirmed` head policy resolution: latest minus `confirmation_blocks` (D-01 opt-in policy) |
| eth_getLogs | INTEGRATE | Windowed bounded fetch per token with deterministic sub-range halving under the -32005 result cap and bounded backoff under throttles (D-05, 02-02); never skips a range |
| eth_getBlockByNumber (plain, by number) | INTEGRATE | Checkpoint verification before every window ingest (SYNC-05 detection), window-end hashes anchoring the checkpoint, WR-01 per-event-block provenance, and the rewind ancestor walk + re-anchor (02-03) |
| eth_subscribe / WebSocket | OPT-OUT | HTTP transport is the Phase 1-2 design per requirements.md section 4; the poll loop follows the head without a subscription |
| eth_call | OPT-OUT | Contract-state calls (totalSupply anchoring) belong to Phase 3 (SUPP-01/SUPP-02), not the event path |
| eth_getTransactionReceipt | OPT-OUT | No new information in the event path: provenance comes from the log itself plus the block header (carried from Phase 1) |
| eth_sendTransaction / contract deployment | TEST-ONLY | Devnet fixture deployment (`bind/v2.DeployContract` with the verbatim Mainnet creation bytecode) and the `issue`/`redeem` driver transactions in `internal/indexer/devnet_test.go` — never in the indexer runtime path |
