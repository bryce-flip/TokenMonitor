---
schema_version: 1
open_count: 0
waived_count: 0
fixed_count: 1
total_count: 1
last_updated: 2026-10-10T11:32:52.206Z
---

# Broken Windows Ledger

> Cross-phase defect register. With `workflow.windows_enforce` enabled, `/gsd-ship` blocks while `open_count > 0`.
> Waive with `gsd-tools windows waive <id> "<reason>"` (reason required).
> Mark fixed with `gsd-tools windows fixed <id>`.

| id | phase | kind | file | line | description | status | reason | recorded_at | resolved_at |
|----|-------|------|------|------|-------------|--------|--------|-------------|-------------|
| 1 | 02 | unmet-truth | .planning/phases/02-reliable-canonical-indexing/02-VALIDATION.md |  | Backstop truth pending: 15-minute continuous Mainnet run following finalized-head advances (manual operator observation, phase gate) | fixed |  | 2026-10-09T10:22:53.900Z | 2026-10-10T11:32:52.206Z |

````json
[
  {
    "id": 1,
    "kind": "unmet-truth",
    "phase": "02",
    "file": ".planning/phases/02-reliable-canonical-indexing/02-VALIDATION.md",
    "line": null,
    "description": "Backstop truth pending: 15-minute continuous Mainnet run following finalized-head advances (manual operator observation, phase gate)",
    "status": "fixed",
    "reason": "",
    "recorded_at": "2026-10-09T10:22:53.900Z",
    "resolved_at": "2026-10-10T11:32:52.206Z",
    "milestone": "v1.0"
  }
]
````
