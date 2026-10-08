---
phase: 01
review: 01-REVIEW.md
titles: json
findings:
  - id: WR-01
    severity: warning
    disposition: open
    title: "Stored block_hash comes from a by-number header fetch, not the log's own BlockHash — silent provenance mismatch under reorg"
  - id: WR-02
    severity: warning
    disposition: open
    title: "Post-ingest inspection prints the 20 oldest rows in the whole table, not the just-ingested range"
  - id: WR-03
    severity: warning
    disposition: open
    title: "Config validation accepts empty or typo'd symbol — token silently skipped at runtime instead of rejected at startup"
  - id: IN-01
    severity: info
    disposition: open
    title: "Rate-limit detection by substring \"429\" can false-positive on error text"
  - id: IN-02
    severity: info
    disposition: open
    title: "Cancellation during backoff reports the stale throttle error, not ctx.Err()"
  - id: IN-03
    severity: info
    disposition: open
    title: "Probe indexes cfg.Tokens[0] without a guard"
  - id: IN-04
    severity: info
    disposition: open
    title: "Decoder classifies by topic0 first, token second — negative guarantee is empirical, not structural"
  - id: IN-05
    severity: info
    disposition: open
    title: "Migration located by CWD-relative candidate paths"
  - id: IN-06
    severity: info
    disposition: open
    title: "Failed Append leaves the prepared batch un-aborted"
  - id: IN-07
    severity: info
    disposition: open
    title: "Inspect interpolates LIMIT via fmt.Sprintf"
  - id: IN-08
    severity: info
    disposition: open
    title: "rpc.Client exposes no Close; Validate does not reject duplicate contracts"
open: 11
total: 11
recorded: 2026-10-08T11:45:00Z
---

# Phase 01: Code Review Disposition

| Finding | Severity | Disposition | Source |
|---------|----------|-------------|--------|
| WR-01 | warning | open | - |
| WR-02 | warning | open | - |
| WR-03 | warning | open | - |
| IN-01 | info | open | - |
| IN-02 | info | open | - |
| IN-03 | info | open | - |
| IN-04 | info | open | - |
| IN-05 | info | open | - |
| IN-06 | info | open | - |
| IN-07 | info | open | - |
| IN-08 | info | open | - |

Dispositions: `open` (recorded, not yet triaged), `fixed`, `skipped`, `deferred`.

Set `deferred` by hand and put the reason in the Source cell; both are preserved. A `|` in the reason is kept as prose and escaped on the next run.

Re-running the gate keeps every row it can. A row the current review no longer reports is kept and its Source cell flagged, so a finding does not leave this record silently. ONE exception: when a finding id is REUSED by a different finding, the earlier decision cannot keep a row — the id is taken — and it is dropped. A RECORDED decision (anything but `open`) is named on the console when that happens; a row still at `open` is replaced silently, because `open` records no decision to lose.
