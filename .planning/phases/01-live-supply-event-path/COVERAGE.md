# API Coverage — Phase 1: Live Supply Event Path

External API in scope: the Ethereum JSON-RPC provider surface used by the indexer (per planning-context note). Decisions below start from full coverage as the default; the matrix is the subtraction record.

| capability | decision | reason |
|---|---|---|
| eth_chainId | INTEGRATE | D-02 fail-fast probe verifies chain ID 1 before any ingest |
| eth_blockNumber | INTEGRATE | D-02 probe: eligible-head bound (head minus confirmation_blocks) for the requested range |
| eth_getLogs | INTEGRATE | Core bounded fetch, filtered by contract address plus token-specific topic0 set |
| eth_getBlockByNumber | INTEGRATE | Block header lookup for canonical hash and timestamp stored as provenance (EVENT-03) |
| eth_getTransactionReceipt | OPT-OUT | Fixture receipts were already verified live during Phase 1 research; the indexer's provenance comes from the log itself plus the block header, so per-run receipt fetches add no information |
| eth_call | OPT-OUT | Contract-state calls (totalSupply) belong to Phase 3 anchoring (SUPP-01/SUPP-02), not the event path |
| eth_subscribe / WebSocket | OPT-OUT | HTTP RPC is the Phase 1-2 transport per requirements.md section 4; continuous head tracking arrives in Phase 2 (SYNC-01) |
| web3_sha3 | OPT-OUT | topic0 constants were verified on-chain during research and are pinned in decoder tests; no runtime hashing need |
