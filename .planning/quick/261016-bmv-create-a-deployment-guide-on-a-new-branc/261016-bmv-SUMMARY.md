---
quick_id: 261016-bmv
status: complete
date: 2026-10-16
implementation_commit: 65575dd
---

# Deployment documentation

Added `docs/DEPLOYMENT.md`, a Chinese single-host runbook for the implemented
Go indexer and ClickHouse path. Linked it from `README.md` and corrected the
README's outdated statement that automatic range halving was still pending.

The guide covers persistent authenticated storage, external secrets/config,
build/install, systemd with SIGINT shutdown, SQL acceptance checks, exclusive
backfill seed semantics, single-writer operation, backup/upgrade/rollback,
and manually confirmed rewind. It identifies Compose, Grafana, and anchored
supply reconciliation as future work.

## Verification

- All 12 Bash command blocks passed `bash -n`; all 8 relative documentation
  and source links resolve.
- `go test -p 1 ./...` passed with Mainnet, ClickHouse, and devnet environment
  variables unset; external suites skipped as intended.
- `go build -p 1 -trimpath -o /tmp/tokenmonitor-indexer ./cmd/indexer` passed.
- `systemd-analyze verify` passed after substituting the local test executable
  and checkout paths. No service was installed or started on the host.
- An isolated container using `clickhouse/clickhouse-server:26.8.20.9` verified
  the documented authenticated user/database, loopback native port, and SQL.
  `go test -p 1 ./internal/storage -count=1` passed against that container.
  All 25 FINAL event rows persisted across a container restart; the temporary
  container and volume were removed afterward.
- Initial sandbox checks encountered local socket restrictions and temporary
  build storage quota limits. Re-running outside the sandbox, with serial
  compilation and build scratch on the repository disk, passed.
- Mainnet RPC access, full history backfill, and live systemd installation
  were not exercised. Source code, migrations, and dependencies are unchanged.

## Git scope and publication

Branch: `docs/deployment-guide`, based on `origin/main` (`200a161`).
Target: a ready-for-review GitHub PR to `main`, published after these records
are committed. No merge is performed as part of this task.

Only the deployment guide, README, and this quick task's records are included.
Pre-existing uncommitted planning work and the six main-only local planning
commits are preserved and excluded from the PR.
