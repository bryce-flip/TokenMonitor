---
phase: 02
review: 02-REVIEW.md
titles: json
findings:
  - id: CR-01
    severity: critical
    disposition: fixed
    title: "Rewind re-anchor is invisible to ReadCheckpoint — the recovery path never completes on the real store"
  - id: WR-01
    severity: warning
    disposition: fixed
    title: "isRateLimitErr matches the bare substring \"429\" — block numbers and hashes in provider error text get misrouted into retry backoff"
  - id: WR-02
    severity: warning
    disposition: fixed
    title: "A single transient error in the finalized-tag probe downgrades the whole run to confirmed semantics"
  - id: WR-03
    severity: warning
    disposition: fixed
    title: "Post-destructive failure in Rewind discards the report — the operator cannot see what was deleted"
  - id: IN-01
    severity: info
    disposition: skipped
    title: "README §4 stale claim and duplicated section number"
  - id: IN-02
    severity: info
    disposition: skipped
    title: "EnsureSchema and the default config path are CWD-relative"
  - id: IN-03
    severity: info
    disposition: skipped
    title: "Load silently coerces explicit invalid zero values into defaults"
  - id: IN-04
    severity: info
    disposition: skipped
    title: "Interrupt during backoff is misreported as a throttle failure"
  - id: IN-05
    severity: info
    disposition: skipped
    title: "FetchLogsResilient has no guard against window == 0"
  - id: IN-06
    severity: info
    disposition: skipped
    title: "Idle loop skips checkpoint verification whenever cpHeight >= head — including cpHeight > head"
  - id: IN-07
    severity: info
    disposition: skipped
    title: "Confirmed rewind plan can differ from the executed plan"
  - id: IN-08
    severity: info
    disposition: skipped
    title: "Inspect has no chain predicate"
open: 0
total: 12
recorded: 2026-10-10T10:20:00Z
---

# Phase 02: Code Review Disposition

| Finding | Severity | Disposition | Source |
|---------|----------|-------------|--------|
| CR-01 | critical | fixed | 02-REVIEW-FIX.md (1f50cf9: DeleteCheckpointAbove + re-anchor ordering; red-checked on real ClickHouse — TestRewindReanchorWinsOnRealStore fails with the reviewer's symptom when the fix is disabled) |
| WR-01 | warning | fixed | 02-REVIEW-FIX.md (11dac89: status-token regex, not bare substring) |
| WR-02 | warning | fixed | 02-REVIEW-FIX.md (d6a4775: loud fail on transient tag-probe errors; only method-not-found downgrades, logged) |
| WR-03 | warning | fixed | 02-REVIEW-FIX.md (c96420e: partial rewind report preserved and printed when after-totals fails) |
| IN-01 | info | skipped | 02-REVIEW-FIX.md (info severity — outside critical_warning fix scope) |
| IN-02 | info | skipped | 02-REVIEW-FIX.md (info severity — outside critical_warning fix scope) |
| IN-03 | info | skipped | 02-REVIEW-FIX.md (info severity — outside critical_warning fix scope) |
| IN-04 | info | skipped | 02-REVIEW-FIX.md (info severity — outside critical_warning fix scope) |
| IN-05 | info | skipped | 02-REVIEW-FIX.md (info severity — unreachable via Validate; outside fix scope) |
| IN-06 | info | skipped | 02-REVIEW-FIX.md (info severity — provider-head-regression blind spot noted; outside fix scope) |
| IN-07 | info | skipped | 02-REVIEW-FIX.md (info severity — outside critical_warning fix scope) |
| IN-08 | info | skipped | 02-REVIEW-FIX.md (info severity — outside critical_warning fix scope) |

Dispositions: `open` (recorded, not yet triaged), `fixed`, `skipped`, `deferred`.

Set `deferred` by hand and put the reason in the Source cell; both are preserved. A `|` in the reason is kept as prose and escaped on the next run.

Re-running the gate keeps every row it can. A row the current review no longer reports is kept and its Source cell flagged, so a finding does not leave this record silently. ONE exception: when a finding id is REUSED by a different finding, the earlier decision cannot keep a row — the id is taken — and it is dropped. A RECORDED decision (anything but `open`) is named on the console when that happens; a row still at `open` is replaced silently, because `open` records no decision to lose.
